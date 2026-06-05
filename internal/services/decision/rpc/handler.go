package rpc

import (
	"context"
	"strings"
	"time"

	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

// Handler adapts the decision application service to RPC protocol types.
type Handler struct {
	svc           *decisionapp.Service
	memberDisplay MemberDisplayLookup
}

// MemberDisplayLookup resolves a member id to its display name. Used to
// populate DecisionView.MemberName so list views never have to render
// the raw member id. Satisfied by *spaceapp.Service.
type MemberDisplayLookup interface {
	DisplayName(ctx context.Context, id member.ID) (string, error)
}

// NewHandler creates an RPC handler wrapping the decision application service.
func NewHandler(svc *decisionapp.Service) *Handler {
	return &Handler{svc: svc}
}

// SetMemberDisplayLookup wires a member-display resolver. Optional —
// when nil the handler returns DecisionView entries without MemberName
// populated, and the UI falls back to MemberID.
func (h *Handler) SetMemberDisplayLookup(lookup MemberDisplayLookup) {
	if h == nil {
		return
	}
	h.memberDisplay = lookup
}

// Create handles decision.create RPC calls.
func (h *Handler) Create(ctx context.Context, p DecisionCreateParams) (DecisionCreateResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return DecisionCreateResult{}, invalidParams("projectId is required")
	}
	if strings.TrimSpace(p.Title) == "" {
		return DecisionCreateResult{}, invalidParams("title is required")
	}
	if strings.TrimSpace(p.Rationale) == "" {
		return DecisionCreateResult{}, invalidParams("rationale is required")
	}

	d := domain.Decision{
		ProjectID:      p.ProjectID,
		Source:         domain.DecisionSource(firstNonEmpty(p.Source, string(domain.DecisionSourceAgent))),
		SourceIdentity: p.SourceIdentity,
		Title:          p.Title,
		Confidence:     p.Confidence,
		TaskRef:        p.TaskRef,
		KeyResultRef:   p.KeyResultRef,
		MissionRef:     p.MissionRef,
		CorrelationRef: p.CorrelationRef,
		InformedByRef:  p.InformedByRef,
		Tags:           p.Tags,
		Metadata:       p.Metadata,
		Log: &domain.LogPayload{
			Rationale:              p.Rationale,
			Context:                p.Context,
			AlternativesRejected:   p.AlternativesRejected,
			InvalidationConditions: append([]string(nil), p.InvalidationConditions...),
			Outcome:                p.Outcome,
		},
	}

	created, err := h.svc.Create(ctx, d)
	if err != nil {
		return DecisionCreateResult{}, internalError("create decision: %v", err)
	}
	return DecisionCreateResult{Decision: h.decisionToView(ctx, created)}, nil
}

// Get handles decision.get RPC calls.
func (h *Handler) Get(ctx context.Context, p DecisionGetParams) (DecisionGetResult, error) {
	id := strings.TrimSpace(p.DecisionID)
	if id == "" {
		return DecisionGetResult{}, invalidParams("decisionId is required")
	}

	d, err := h.svc.Get(ctx, domain.DecisionID(id))
	if err != nil {
		return DecisionGetResult{}, internalError("get decision: %v", err)
	}
	return DecisionGetResult{Decision: h.decisionToView(ctx, d)}, nil
}

// Delete handles decision.delete RPC calls.
func (h *Handler) Delete(ctx context.Context, p DecisionDeleteParams) (DecisionDeleteResult, error) {
	id := strings.TrimSpace(p.DecisionID)
	if id == "" {
		return DecisionDeleteResult{}, invalidParams("decisionId is required")
	}

	if err := h.svc.Delete(ctx, domain.DecisionID(id)); err != nil {
		return DecisionDeleteResult{}, internalError("delete decision: %v", err)
	}
	return DecisionDeleteResult{Deleted: true}, nil
}

// List handles decision.list RPC calls.
func (h *Handler) List(ctx context.Context, p DecisionListParams) (DecisionListResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return DecisionListResult{}, invalidParams("projectId is required")
	}

	filter := decisionFilterFromParams(
		projectID,
		p.Source,
		p.Tags,
		p.Query,
		p.Since,
		p.Until,
		p.Sort,
		p.Limit,
		p.Offset,
	)
	decisions, err := h.svc.List(ctx, filter)
	if err != nil {
		return DecisionListResult{}, internalError("list decisions: %v", err)
	}

	views := make([]DecisionView, len(decisions))
	for i, d := range decisions {
		views[i] = h.decisionToView(ctx, d)
	}
	return DecisionListResult{Decisions: views}, nil
}

// Count handles decision.count RPC calls.
func (h *Handler) Count(ctx context.Context, p DecisionCountParams) (DecisionCountResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return DecisionCountResult{}, invalidParams("projectId is required")
	}

	filter := decisionFilterFromParams(
		projectID,
		p.Source,
		p.Tags,
		p.Query,
		p.Since,
		p.Until,
		"",
		0,
		0,
	)
	count, err := h.svc.Count(ctx, filter)
	if err != nil {
		return DecisionCountResult{}, internalError("count decisions: %v", err)
	}
	return DecisionCountResult{Count: count}, nil
}

// Export handles decision.export RPC calls.
func (h *Handler) Export(ctx context.Context, p DecisionExportParams) (DecisionExportResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return DecisionExportResult{}, invalidParams("projectId is required")
	}

	filter := decisionFilterFromParams(
		projectID,
		p.Source,
		p.Tags,
		p.Query,
		p.Since,
		p.Until,
		p.Sort,
		0,
		0,
	)
	decisions, err := h.svc.Export(ctx, filter)
	if err != nil {
		return DecisionExportResult{}, internalError("export decisions: %v", err)
	}

	views := make([]DecisionView, len(decisions))
	for i, d := range decisions {
		views[i] = h.decisionToView(ctx, d)
	}
	return DecisionExportResult{Decisions: views}, nil
}

// ---------------------------------------------------------------------------
// View helper
// ---------------------------------------------------------------------------

func (h *Handler) decisionToView(ctx context.Context, d domain.Decision) DecisionView {
	view := DecisionView{
		ID:             string(d.ID),
		ProjectID:      d.ProjectID,
		Source:         string(d.Source),
		Kind:           string(d.Kind()),
		MemberID:       d.SourceIdentity,
		MemberName:     h.resolveActorDisplay(ctx, d),
		SourceIdentity: d.SourceIdentity,
		Title:          d.Title,
		Confidence:     d.Confidence,
		TaskRef:        d.TaskRef,
		KeyResultRef:   d.KeyResultRef,
		MissionRef:     d.MissionRef,
		CorrelationRef: d.CorrelationRef,
		InformedByRef:  d.InformedByRef,
		Tags:           d.Tags,
		Metadata:       d.Metadata,
		CreatedAt:      d.CreatedAt,
	}
	if p := d.Log; p != nil {
		view.Rationale = p.Rationale
		view.Context = p.Context
		view.AlternativesRejected = p.AlternativesRejected
		view.InvalidationConditions = append([]string(nil), p.InvalidationConditions...)
		view.Outcome = p.Outcome
	}
	return view
}

func (h *Handler) resolveActorDisplay(ctx context.Context, d domain.Decision) string {
	return h.resolveMemberDisplay(ctx, d.SourceIdentity)
}

// resolveMemberDisplay returns the display name for a member id. Empty
// when the lookup is not wired or the lookup itself fails. The handler
// is read-mostly so the failure mode is "the UI shows the id instead
// of a name" — we don't want the whole list view to error out when the
// member registry is briefly unavailable.
func (h *Handler) resolveMemberDisplay(ctx context.Context, memberID string) string {
	if h == nil || h.memberDisplay == nil {
		return ""
	}
	id := strings.TrimSpace(memberID)
	if id == "" {
		return ""
	}
	name, err := h.memberDisplay.DisplayName(ctx, member.ID(id))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(name)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func decisionFilterFromParams(
	projectID string,
	source string,
	tags []string,
	query string,
	since string,
	until string,
	sort string,
	limit int,
	offset int,
) domain.DecisionFilter {
	filter := domain.DecisionFilter{
		ProjectID: strings.TrimSpace(projectID),
		Tags:      tags,
		Query:     strings.TrimSpace(query),
		SortDesc:  strings.TrimSpace(sort) != "oldest",
		Limit:     limit,
		Offset:    offset,
	}
	if s := strings.TrimSpace(source); s != "" {
		filter.Sources = []domain.DecisionSource{domain.DecisionSource(s)}
	}
	if s := strings.TrimSpace(since); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.Since = &t
		}
	}
	if s := strings.TrimSpace(until); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.Until = &t
		}
	}
	return filter
}
