// Package app provides the application service for decision lifecycle management.
//
// What this service is responsible for:
//   - Persisting decisions through the repository.
//   - Auto-creating graph edges from a decision to anything it
//     references (key result, mission, task).
//   - Publishing a "decision.logged" event so the notification evaluator
//     and other subscribers see new decisions.
//   - De-duplicating identical decisions logged within a short window
//     (same project + role + title + task) — guards against agent
//     retry-storms creating dozens of identical entries.
//
// What this service is NOT responsible for:
//   - Validating the decision's shape — that lives on domain.Decision.Validate().
//   - Constructing decisions for callers — Log is a thin mapping wrapper;
//     tools or RPC handlers can also build a Decision directly and call Create.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

// Service orchestrates decision creation with side effects.
type Service struct {
	repo        domain.Repository
	clock       domain.Clock
	logger      *slog.Logger
	links       GraphLinkWriter
	linkDeleter GraphLinkDeleter
	events      EventPublisher
	tasks       TaskKeyResultReader
	missions    KeyResultMissionReader
	members     MemberDisplayLookup
}

// LogRequest is the input shape for Service.Log — a "log" decision
// recorded deliberately by an agent. Tool/RPC layers map their wire
// formats onto this struct.
type LogRequest struct {
	ProjectID              string
	MemberID               string
	Title                  string
	Rationale              string
	AlternativesRejected   string
	InvalidationConditions []string
	Confidence             float64
	TaskRef                string
	KeyResultRef           string
	MissionRef             string
}

// Result is the projected response shape returned by Log.
type Result struct {
	ID                     string
	Kind                   string
	Title                  string
	InvalidationConditions []string
	TaskRef                string
	KeyResultRef           string
	MissionRef             string
	MemberID               string
	MemberName             string
	SourceType             string
}

type GraphLinkWriter interface {
	Link(ctx context.Context, req graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error)
}

type GraphLinkDeleter interface {
	DeleteLinksForNode(ctx context.Context, nodeType string, nodeID string) error
}

type EventPublisher interface {
	Publish(topic string, event any) error
}

type TaskKeyResultReader interface {
	TaskKeyResult(ctx context.Context, taskID string) (string, error)
}

type KeyResultMissionReader interface {
	KeyResultMission(ctx context.Context, keyResultID string) (string, error)
}

type MemberDisplayLookup interface {
	DisplayName(ctx context.Context, id member.ID) (string, error)
}

func NewService(
	repo domain.Repository,
	clock domain.Clock,
	links GraphLinkWriter,
	linkDeleter GraphLinkDeleter,
	events EventPublisher,
	tasks TaskKeyResultReader,
	missions KeyResultMissionReader,
	members MemberDisplayLookup,
	logger *slog.Logger,
) (*Service, error) {
	switch {
	case repo == nil:
		return nil, errors.New("decision service: repository is required")
	case clock == nil:
		return nil, errors.New("decision service: clock is required")
	case links == nil:
		return nil, errors.New("decision service: graph link writer is required")
	case linkDeleter == nil:
		return nil, errors.New("decision service: graph link deleter is required")
	case events == nil:
		return nil, errors.New("decision service: event publisher is required")
	}
	if logger == nil {
		logger = slog.Default().With("service", "decision")
	}
	return &Service{
		repo:        repo,
		clock:       clock,
		logger:      logger,
		links:       links,
		linkDeleter: linkDeleter,
		events:      events,
		tasks:       tasks,
		missions:    missions,
		members:     members,
	}, nil
}

// dedupWindow controls how far back to look when checking for duplicate
// decisions. Same-turn duplicates from agent retries land within seconds;
// 60s catches those without rejecting genuinely-similar decisions logged
// minutes apart.
const dedupWindow = 60 * time.Second

// resolveDecisionRefs walks task → KR → mission, filling in any missing
// upstream refs so the graph edge fan-out below has the full set of
// targets. This means a caller who only supplied TaskRef gets the
// (decision)→(KR) and (decision)→(mission) edges automatically.
func (s *Service) resolveDecisionRefs(ctx context.Context, d *domain.Decision) error {
	if d == nil {
		return errors.New("resolve decision refs: decision is nil")
	}
	if d.KeyResultRef == "" && d.TaskRef != "" && s.tasks != nil {
		kr, err := s.tasks.TaskKeyResult(ctx, d.TaskRef)
		if err != nil {
			return fmt.Errorf("resolve decision task %s key result: %w", d.TaskRef, err)
		}
		d.KeyResultRef = strings.TrimSpace(kr)
	}
	if d.MissionRef == "" && d.KeyResultRef != "" && s.missions != nil {
		m, err := s.missions.KeyResultMission(ctx, d.KeyResultRef)
		if err != nil {
			return fmt.Errorf("resolve decision key result %s mission: %w", d.KeyResultRef, err)
		}
		d.MissionRef = strings.TrimSpace(m)
	}
	return nil
}

// Create persists a new decision and applies side effects in a fixed
// order: validate → resolve refs → dedup → save → emit links → emit event.
//
// Dedup behavior: if an identical decision (same project + source role +
// title + task) was created within dedupWindow, Create returns the input
// without writing anything. This is intentional — we don't want a single
// agent retry to spawn duplicate audit entries.
//
// Side-effect ordering matters: the graph edges reference the saved
// decision's ID, so save has to happen first; the event payload should
// reflect the persisted state, so it goes last.
func (s *Service) Create(ctx context.Context, d domain.Decision) (domain.Decision, error) {
	if d.ID == "" {
		d.ID = domain.DecisionID("dec-" + uuid.NewString())
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = s.clock.Now().UTC()
	}
	if strings.TrimSpace(d.MemberName) == "" {
		d.MemberName = s.resolveMemberDisplay(ctx, d.SourceIdentity)
	}

	if err := d.Validate(); err != nil {
		return domain.Decision{}, fmt.Errorf("validate decision: %w", err)
	}
	if err := s.resolveDecisionRefs(ctx, &d); err != nil {
		return domain.Decision{}, err
	}

	// Reject duplicates within the window. Returning d (with the caller's
	// generated ID) rather than the existing row is intentional: the
	// caller's contract is "you tried to log this, we treated it as a
	// no-op"; they don't need the existing row's ID.
	since := d.CreatedAt.Add(-dedupWindow)
	exists, err := s.repo.DecisionExistsByFingerprint(ctx, d.ProjectID, d.SourceIdentity, d.Title, d.TaskRef, since)
	if err != nil {
		return domain.Decision{}, fmt.Errorf("dedup check: %w", err)
	}
	if exists {
		s.logger.Info("decision dedup: identical decision exists within window",
			"title", d.Title, "sourceIdentity", d.SourceIdentity, "taskRef", d.TaskRef)
		return d, nil
	}

	if err := s.repo.CreateDecision(ctx, d); err != nil {
		return domain.Decision{}, fmt.Errorf("create decision: %w", err)
	}

	if err := s.emitContextLinks(ctx, d); err != nil {
		return domain.Decision{}, err
	}

	// Publish decision.logged so subscribers (notification evaluator,
	// future projections) see the new decision. Publish failure is part
	// of the create operation because missing wake/notification events
	// hide product-visible state transitions.
	if err := s.events.Publish(eventbus.TopicDecisionLogged, eventbus.DecisionLoggedEvent{
		ProjectID:    d.ProjectID,
		DecisionID:   string(d.ID),
		Source:       string(d.Source),
		MemberID:     d.SourceIdentity,
		MemberName:   d.MemberName,
		Title:        d.Title,
		Confidence:   d.Confidence,
		KeyResultRef: d.KeyResultRef,
		MissionRef:   d.MissionRef,
		TaskRef:      d.TaskRef,
		Timestamp:    d.CreatedAt,
	}); err != nil {
		return domain.Decision{}, fmt.Errorf("publish decision logged event: %w", err)
	}

	return d, nil
}

// resolveMemberDisplay looks up the member's display name. Returns
// empty string when the lookup func is nil or the lookup itself
// fails — callers populate the resolved name into Result.MemberName
// so downstream consumers don't have to do the work.
func (s *Service) resolveMemberDisplay(ctx context.Context, memberID string) string {
	if s.members == nil {
		return ""
	}
	id := strings.TrimSpace(memberID)
	if id == "" {
		return ""
	}
	name, err := s.members.DisplayName(ctx, member.ID(id))
	if err != nil {
		s.logger.Warn("decision: resolve member display name", "memberId", id, "error", err)
		return ""
	}
	return strings.TrimSpace(name)
}

// emitContextLinks creates the (decision) → (X) edges for every non-empty
// FK ref on the decision. This is data-driven on purpose: each entry in
// the table maps "if this ref is set, emit an edge to this target with
// this edge type." Adding a new target type means adding one row.
func (s *Service) emitContextLinks(ctx context.Context, d domain.Decision) error {
	specs := []struct {
		targetID, targetType, edge string
	}{
		{d.KeyResultRef, graphdomain.NodeTypeKeyResult, graphdomain.EdgeTypeServes},
		{d.MissionRef, graphdomain.NodeTypeMission, graphdomain.EdgeTypeServes},
		{d.TaskRef, graphdomain.NodeTypeTask, graphdomain.EdgeTypeMadeDuring},
	}
	for _, spec := range specs {
		if strings.TrimSpace(spec.targetID) == "" {
			continue
		}
		confidence := 1.0
		_, _, err := s.links.Link(ctx, graphdomain.GraphLinkRequest{
			ProjectID:  d.ProjectID,
			SourceType: graphdomain.NodeTypeDecision,
			SourceID:   string(d.ID),
			TargetType: spec.targetType,
			TargetID:   spec.targetID,
			EdgeType:   spec.edge,
			Confidence: &confidence,
			Rationale:  "Decision references this work item.",
			Origin:     "reference",
			CreatedBy:  "decision_service",
		})
		if err != nil {
			return fmt.Errorf("create %s link for decision %s: %w", spec.targetType, d.ID, err)
		}
	}
	return nil
}

// Log records a deliberate "log" decision. This is the agent path —
// the agent decided X, here's why, here's what alternatives were
// rejected, here's what would invalidate this. Validate() rejects
// blank rationale or title, so we don't trim at this boundary; we
// trust callers to pass meaningful values.
func (s *Service) Log(ctx context.Context, req LogRequest) (Result, error) {
	d, err := domain.NewLog(domain.NewLogInput{
		ProjectID:              req.ProjectID,
		MemberID:               req.MemberID,
		MemberName:             s.resolveMemberDisplay(ctx, req.MemberID),
		Title:                  req.Title,
		Rationale:              req.Rationale,
		Context:                "",
		AlternativesRejected:   req.AlternativesRejected,
		InvalidationConditions: req.InvalidationConditions,
		Confidence:             req.Confidence,
		TaskRef:                req.TaskRef,
		KeyResultRef:           req.KeyResultRef,
		MissionRef:             req.MissionRef,
	}, s.clock.Now())
	if err != nil {
		return Result{}, err
	}
	created, err := s.Create(ctx, d)
	if err != nil {
		return Result{}, err
	}
	out := buildResult(created)
	out.MemberName = created.MemberName
	if strings.TrimSpace(out.MemberName) == "" {
		out.MemberName = s.resolveMemberDisplay(ctx, created.SourceIdentity)
	}
	if created.Log != nil {
		out.InvalidationConditions = append([]string(nil), created.Log.InvalidationConditions...)
	}
	return out, nil
}

// buildResult projects common fields from a persisted decision into the
// Result shape.
func buildResult(d domain.Decision) Result {
	return Result{
		ID:           string(d.ID),
		Kind:         string(d.Kind()),
		Title:        d.Title,
		TaskRef:      d.TaskRef,
		KeyResultRef: d.KeyResultRef,
		MissionRef:   d.MissionRef,
		MemberID:     d.SourceIdentity,
		SourceType:   string(d.Source),
	}
}

// Get retrieves a decision by ID. Pass-through to the repository.
func (s *Service) Get(ctx context.Context, id domain.DecisionID) (domain.Decision, error) {
	return s.repo.GetDecision(ctx, id)
}

// Delete removes a decision and the context links pointing at it.
//
// We delete links first then the row — if link deletion fails, the
// decision is preserved so the caller can retry. Deleting the row
// first would orphan the links if the second step failed.
func (s *Service) Delete(ctx context.Context, id domain.DecisionID) error {
	trimmed := strings.TrimSpace(string(id))
	if trimmed == "" {
		return errors.New("decision id is required")
	}
	if _, err := s.repo.GetDecision(ctx, domain.DecisionID(trimmed)); err != nil {
		return fmt.Errorf("get decision before delete: %w", err)
	}
	if err := s.linkDeleter.DeleteLinksForNode(ctx, graphdomain.NodeTypeDecision, trimmed); err != nil {
		return fmt.Errorf("delete decision links: %w", err)
	}
	if err := s.repo.DeleteDecision(ctx, domain.DecisionID(trimmed)); err != nil {
		return fmt.Errorf("delete decision: %w", err)
	}
	return nil
}

func (s *Service) List(ctx context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	filter.ProjectID = strings.TrimSpace(filter.ProjectID)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.ProjectID == "" {
		return nil, fmt.Errorf("projectId is required")
	}
	if filter.Limit < 0 {
		return nil, fmt.Errorf("decision list limit must be non-negative")
	}
	if filter.Offset < 0 {
		return nil, fmt.Errorf("decision list offset must be non-negative")
	}
	return s.repo.ListDecisions(ctx, filter)
}

func (s *Service) ListByKeyResult(ctx context.Context, keyResultRef string) ([]domain.Decision, error) {
	keyResultRef = strings.TrimSpace(keyResultRef)
	if keyResultRef == "" {
		return nil, fmt.Errorf("keyResultRef is required")
	}
	return s.repo.ListDecisionsByKeyResult(ctx, keyResultRef)
}

func (s *Service) Count(ctx context.Context, filter domain.DecisionFilter) (int, error) {
	filter.ProjectID = strings.TrimSpace(filter.ProjectID)
	if filter.ProjectID == "" {
		return 0, fmt.Errorf("projectId is required")
	}
	if filter.Limit < 0 {
		return 0, fmt.Errorf("decision count limit must be non-negative")
	}
	if filter.Offset < 0 {
		return 0, fmt.Errorf("decision count offset must be non-negative")
	}
	return s.repo.CountDecisions(ctx, filter)
}

func (s *Service) Export(ctx context.Context, filter domain.DecisionFilter) ([]domain.Decision, error) {
	filter.ProjectID = strings.TrimSpace(filter.ProjectID)
	if filter.ProjectID == "" {
		return nil, fmt.Errorf("projectId is required")
	}
	if filter.Limit < 0 {
		return nil, fmt.Errorf("decision export limit must be non-negative")
	}
	if filter.Offset < 0 {
		return nil, fmt.Errorf("decision export offset must be non-negative")
	}
	return s.repo.ExportDecisions(ctx, filter)
}
