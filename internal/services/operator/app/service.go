package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	decisiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	graphdomain "github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
)

// TaskPort applies task-side blocking effects for operator items.
type TaskPort interface {
	BlockTask(ctx context.Context, taskID string, blockerID string) error
	UnblockTask(ctx context.Context, taskID string, blockerID string) error
	GetTaskKeyResultRef(ctx context.Context, taskRef string) (string, error)
}

// GraphPort persists graph relationships through the graph service boundary.
type GraphPort interface {
	Link(ctx context.Context, req graphdomain.GraphLinkRequest) (graphdomain.GraphEdge, []graphdomain.GraphWarning, error)
}

// EventPublisher publishes operator-domain lifecycle events.
type EventPublisher interface {
	PublishOperatorEvent(ctx context.Context, event any) error
}

// DecisionPort creates operator decisions.
type DecisionPort interface {
	CreateDecision(ctx context.Context, decision decisiondomain.Decision) (decisiondomain.DecisionID, error)
}

// MessagePublisher sends operator lifecycle messages back to agents.
type MessagePublisher interface {
	ResolveEscalation(ctx context.Context, esc domain.Escalation, params ResolveEscalationParams) error
	CompleteAction(ctx context.Context, action domain.OperatorAction) error
	CommentOnAction(ctx context.Context, action domain.OperatorAction, comment domain.Comment) error
	BlockAction(ctx context.Context, action domain.OperatorAction, reason string) error
}

// MissionRefLookup resolves mission references from key results and validates explicit mission refs.
type MissionRefLookup interface {
	GetMissionFromKeyResult(ctx context.Context, krRef string) (string, error)
	ValidateMission(ctx context.Context, missionID string) error
}

type servicePorts struct {
	Tasks       TaskPort
	Graph       GraphPort
	Events      EventPublisher
	Decisions   DecisionPort
	Messages    MessagePublisher
	MissionRefs MissionRefLookup
}

// CreateEscalationParams holds fields needed to create a new escalation.
type CreateEscalationParams struct {
	ProjectID      string
	SpaceID        string
	TaskRef        string
	KeyResultRef   string
	MissionRef     string
	Source         string
	MemberID       string
	Category       domain.Category
	Urgency        domain.Urgency
	Title          string
	Description    string
	Recommendation string
	Confidence     float64
	Deadline       *time.Time
	Metadata       map[string]string
}

// ResolveEscalationParams holds fields needed to resolve an escalation.
// Resolution is a binary verdict (approve | reject); any "use approach X" /
// "ask Bob" / "later" intent goes in ResolutionNote.
type ResolveEscalationParams struct {
	Resolution     domain.Resolution
	ResolutionNote string
	ResolvedBy     string
}

const dedupWindow = 5 * time.Minute

// Service orchestrates both operator-action and escalation lifecycle transitions.
type Service struct {
	actionRepo     domain.ActionRepository
	escalationRepo domain.EscalationRepository
	logger         *slog.Logger
	ports          servicePorts
}

func NewService(
	escRepo domain.EscalationRepository,
	actionRepo domain.ActionRepository,
	tasks TaskPort,
	graph GraphPort,
	events EventPublisher,
	decisions DecisionPort,
	messages MessagePublisher,
	missionRefs MissionRefLookup,
	logger *slog.Logger,
) (*Service, error) {
	switch {
	case escRepo == nil:
		return nil, fmt.Errorf("operator service: escalation repository is required")
	case actionRepo == nil:
		return nil, fmt.Errorf("operator service: action repository is required")
	case logger == nil:
		return nil, fmt.Errorf("operator service: logger is required")
	case tasks == nil:
		return nil, fmt.Errorf("operator service: Tasks is required")
	case graph == nil:
		return nil, fmt.Errorf("operator service: Graph is required")
	case events == nil:
		return nil, fmt.Errorf("operator service: Events is required")
	case decisions == nil:
		return nil, fmt.Errorf("operator service: Decisions is required")
	case messages == nil:
		return nil, fmt.Errorf("operator service: Messages is required")
	case missionRefs == nil:
		return nil, fmt.Errorf("operator service: MissionRefs is required")
	}

	return &Service{
		actionRepo:     actionRepo,
		escalationRepo: escRepo,
		logger:         logger,
		ports: servicePorts{
			Tasks:       tasks,
			Graph:       graph,
			Events:      events,
			Decisions:   decisions,
			Messages:    messages,
			MissionRefs: missionRefs,
		},
	}, nil
}

func deadlineFromHours(hours int) *time.Time {
	if hours <= 0 {
		return nil
	}
	dl := time.Now().UTC().Add(time.Duration(hours) * time.Hour)
	return &dl
}

type operatorRefs struct {
	TaskRef      string
	KeyResultRef string
	MissionRef   string
}

func (s *Service) resolveRefs(ctx context.Context, taskRef, keyResultRef, missionRef string) (operatorRefs, error) {
	taskRef = strings.TrimSpace(taskRef)
	keyResultRef = strings.TrimSpace(keyResultRef)
	missionRef = strings.TrimSpace(missionRef)

	resolved := operatorRefs{TaskRef: taskRef, KeyResultRef: keyResultRef, MissionRef: missionRef}
	if resolved.TaskRef != "" {
		taskKR, err := s.ports.Tasks.GetTaskKeyResultRef(ctx, resolved.TaskRef)
		if err != nil {
			return operatorRefs{}, fmt.Errorf("operator: resolve task_ref %q: %w", resolved.TaskRef, err)
		}
		taskKR = strings.TrimSpace(taskKR)
		if taskKR != "" {
			if resolved.KeyResultRef != "" && !strings.EqualFold(resolved.KeyResultRef, taskKR) {
				return operatorRefs{}, fmt.Errorf("operator: task_ref %q links to key_result_ref %q, but %q was provided", resolved.TaskRef, taskKR, resolved.KeyResultRef)
			}
			resolved.KeyResultRef = taskKR
		}
	}

	if resolved.KeyResultRef != "" {
		derivedMissionRef, err := s.ports.MissionRefs.GetMissionFromKeyResult(ctx, resolved.KeyResultRef)
		if err != nil {
			return operatorRefs{}, fmt.Errorf("operator: invalid key_result_ref %q: %w", resolved.KeyResultRef, err)
		}
		derivedMissionRef = strings.TrimSpace(derivedMissionRef)
		if derivedMissionRef == "" {
			return operatorRefs{}, fmt.Errorf("operator: key_result_ref %q is missing mission linkage", resolved.KeyResultRef)
		}
		if resolved.MissionRef != "" && !strings.EqualFold(resolved.MissionRef, derivedMissionRef) {
			return operatorRefs{}, fmt.Errorf("operator: key_result_ref %q belongs to mission_ref %q, but %q was provided", resolved.KeyResultRef, derivedMissionRef, resolved.MissionRef)
		}
		resolved.MissionRef = derivedMissionRef
	} else if resolved.MissionRef != "" {
		if err := s.ports.MissionRefs.ValidateMission(ctx, resolved.MissionRef); err != nil {
			return operatorRefs{}, fmt.Errorf("operator: invalid mission_ref %q: %w", resolved.MissionRef, err)
		}
	}

	return resolved, nil
}

// Create persists a new operator action and applies side effects.
func (s *Service) Create(ctx context.Context, params domain.CreateParams) (domain.OperatorAction, error) {
	if params.ID == "" {
		params.ID = domain.OperatorActionID("oa-" + uuid.NewString())
	}

	oa, err := domain.NewOperatorAction(params, time.Now().UTC())
	if err != nil {
		return domain.OperatorAction{}, fmt.Errorf("create operator action: %w", err)
	}

	if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
		return domain.OperatorAction{}, fmt.Errorf("save operator action: %w", err)
	}

	if oa.Blocking && oa.TaskRef != "" {
		if err := s.ports.Tasks.BlockTask(ctx, oa.TaskRef, string(oa.ID)); err != nil {
			s.logger.Error("failed to block task", "oaID", oa.ID, "taskRef", oa.TaskRef, "error", err)
			return domain.OperatorAction{}, fmt.Errorf("block task %s: %w", oa.TaskRef, err)
		}
	}

	if err := s.linkBlockedBy(ctx, oa.ProjectID, oa.CreatedAt, strings.TrimSpace(oa.TaskRef), strings.TrimSpace(oa.KeyResultRef), strings.TrimSpace(oa.MissionRef), graphdomain.NodeTypeOperatorAction, string(oa.ID)); err != nil {
		return domain.OperatorAction{}, err
	}

	if err := s.publishOALifecycle(ctx, eventbus.OALifecycleEvent{
		ProjectID: oa.ProjectID,
		SpaceID:   oa.SpaceID,
		ActionID:  string(oa.ID),
		TaskRef:   oa.TaskRef,
		EventType: "oa.created",
		OldStatus: "",
		NewStatus: string(oa.Status),
		Title:     oa.Title,
		Urgency:   string(oa.Urgency),
		Category:  string(oa.Category),
		Blocking:  oa.Blocking,
		Timestamp: oa.CreatedAt,
	}); err != nil {
		return oa, fmt.Errorf("publish oa.created: %w", err)
	}

	return oa, nil
}

func (s *Service) Get(ctx context.Context, id domain.OperatorActionID) (domain.OperatorAction, error) {
	oa, err := s.actionRepo.GetAction(ctx, id)
	if err != nil {
		return domain.OperatorAction{}, err
	}
	if oa.Status == domain.OAStatusPending {
		now := time.Now().UTC()
		oldStatus := oa.Status
		if err := oa.Acknowledge(now); err != nil {
			return domain.OperatorAction{}, fmt.Errorf("auto-acknowledge: %w", err)
		}
		if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
			return domain.OperatorAction{}, fmt.Errorf("save auto-acknowledged: %w", err)
		}
		if err := s.publishOALifecycle(ctx, eventbus.OALifecycleEvent{
			ProjectID: oa.ProjectID,
			SpaceID:   oa.SpaceID,
			ActionID:  string(oa.ID),
			TaskRef:   oa.TaskRef,
			EventType: "oa.acknowledged",
			OldStatus: string(oldStatus),
			NewStatus: string(oa.Status),
			Blocking:  oa.Blocking,
			Timestamp: now,
		}); err != nil {
			return oa, fmt.Errorf("publish oa.acknowledged: %w", err)
		}
	}
	return oa, nil
}

func (s *Service) Start(ctx context.Context, id domain.OperatorActionID) (domain.OperatorAction, error) {
	return s.transitionAction(ctx, id, "oa.started", func(oa *domain.OperatorAction) error {
		return oa.Start(time.Now().UTC())
	}, nil)
}

func (s *Service) Complete(ctx context.Context, id domain.OperatorActionID, outcome domain.CompleteOutcome) (domain.OperatorAction, error) {
	return s.transitionAction(ctx, id, "oa.completed", func(oa *domain.OperatorAction) error {
		return oa.Complete(outcome, time.Now().UTC())
	}, func(oa *domain.OperatorAction) error {
		if oa.Status == domain.OAStatusCompleted {
			if err := s.tryUnblockActionTaskOnComplete(ctx, oa); err != nil {
				return err
			}
			s.tryCreateDecisionOnActionComplete(ctx, oa)
			return s.tryPublishCompletionMessage(ctx, oa)
		}
		return nil
	})
}

func (s *Service) Verify(ctx context.Context, id domain.OperatorActionID, accepted bool, feedback string, author string) (domain.OperatorAction, error) {
	oa, err := s.actionRepo.GetAction(ctx, id)
	if err != nil {
		return domain.OperatorAction{}, err
	}

	oldStatus := oa.Status
	now := time.Now().UTC()
	if err := oa.Verify(accepted, feedback, now); err != nil {
		return domain.OperatorAction{}, err
	}
	if !accepted && strings.TrimSpace(author) != "" {
		comment := domain.Comment{Author: strings.TrimSpace(author), Text: "[VERIFICATION] " + strings.TrimSpace(feedback), CreatedAt: now}
		if err := oa.AddComment(comment); err != nil {
			return domain.OperatorAction{}, err
		}
	}
	if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
		return domain.OperatorAction{}, fmt.Errorf("save oa.verified: %w", err)
	}
	if accepted && oa.Status == domain.OAStatusCompleted {
		if err := s.tryUnblockActionTaskOnComplete(ctx, &oa); err != nil {
			return oa, fmt.Errorf("verify unblock task: %w", err)
		}
		s.tryCreateDecisionOnActionComplete(ctx, &oa)
		if err := s.tryPublishCompletionMessage(ctx, &oa); err != nil {
			return oa, fmt.Errorf("verify completion message: %w", err)
		}
	}
	if err := s.publishOALifecycle(ctx, eventbus.OALifecycleEvent{
		ProjectID:     oa.ProjectID,
		SpaceID:       oa.SpaceID,
		ActionID:      string(oa.ID),
		TaskRef:       oa.TaskRef,
		EventType:     "oa.verified",
		OldStatus:     string(oldStatus),
		NewStatus:     string(oa.Status),
		Title:         oa.Title,
		Urgency:       string(oa.Urgency),
		Category:      string(oa.Category),
		OutcomeStatus: string(oa.OutcomeStatus),
		Blocking:      oa.Blocking,
		Timestamp:     now,
	}); err != nil {
		return oa, fmt.Errorf("publish oa.verified: %w", err)
	}
	return oa, nil
}

func (s *Service) Block(ctx context.Context, id domain.OperatorActionID, reason string) (domain.OperatorAction, error) {
	oa, err := s.actionRepo.GetAction(ctx, id)
	if err != nil {
		return domain.OperatorAction{}, err
	}
	oldStatus := oa.Status
	now := time.Now().UTC()
	if err := oa.Block(reason, now); err != nil {
		return domain.OperatorAction{}, err
	}
	if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
		return domain.OperatorAction{}, fmt.Errorf("save oa.blocked: %w", err)
	}
	if err := s.tryPublishBlockedMessage(ctx, &oa, reason); err != nil {
		return oa, fmt.Errorf("after-persist oa.blocked: %w", err)
	}
	if err := s.publishOALifecycle(ctx, eventbus.OALifecycleEvent{
		ProjectID: oa.ProjectID,
		SpaceID:   oa.SpaceID,
		ActionID:  string(oa.ID),
		TaskRef:   oa.TaskRef,
		EventType: "oa.blocked",
		OldStatus: string(oldStatus),
		NewStatus: string(oa.Status),
		Title:     oa.Title,
		Urgency:   string(oa.Urgency),
		Category:  string(oa.Category),
		Blocking:  oa.Blocking,
		Timestamp: now,
	}); err != nil {
		return oa, fmt.Errorf("publish oa.blocked: %w", err)
	}
	return oa, nil
}

func (s *Service) Unblock(ctx context.Context, id domain.OperatorActionID) (domain.OperatorAction, error) {
	return s.transitionAction(ctx, id, "oa.unblocked", func(oa *domain.OperatorAction) error {
		return oa.Unblock(time.Now().UTC())
	}, nil)
}

func (s *Service) Cancel(ctx context.Context, id domain.OperatorActionID) (domain.OperatorAction, error) {
	return s.transitionAction(ctx, id, "oa.canceled", func(oa *domain.OperatorAction) error {
		return oa.Cancel(time.Now().UTC())
	}, nil)
}

func (s *Service) AddProgressNote(ctx context.Context, id domain.OperatorActionID, text string) (domain.OperatorAction, error) {
	oa, err := s.actionRepo.GetAction(ctx, id)
	if err != nil {
		return domain.OperatorAction{}, err
	}
	note := domain.ProgressNote{Text: text, CreatedAt: time.Now().UTC()}
	if err := oa.AddProgressNote(note); err != nil {
		return domain.OperatorAction{}, err
	}
	if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
		return domain.OperatorAction{}, fmt.Errorf("save progress note: %w", err)
	}
	if err := s.publishOALifecycle(ctx, eventbus.OALifecycleEvent{
		ProjectID: oa.ProjectID,
		SpaceID:   oa.SpaceID,
		ActionID:  string(oa.ID),
		TaskRef:   oa.TaskRef,
		EventType: "oa.progress_noted",
		OldStatus: string(oa.Status),
		NewStatus: string(oa.Status),
		Title:     oa.Title,
		Category:  string(oa.Category),
		Urgency:   string(oa.Urgency),
		Text:      note.Text,
		Blocking:  oa.Blocking,
		Timestamp: note.CreatedAt,
	}); err != nil {
		return oa, fmt.Errorf("publish oa.progress_noted: %w", err)
	}
	return oa, nil
}

func (s *Service) AddComment(ctx context.Context, id domain.OperatorActionID, author string, text string) (domain.OperatorAction, error) {
	oa, err := s.actionRepo.GetAction(ctx, id)
	if err != nil {
		return domain.OperatorAction{}, err
	}
	comment := domain.Comment{Author: author, Text: text, CreatedAt: time.Now().UTC()}
	if err := oa.AddComment(comment); err != nil {
		return domain.OperatorAction{}, err
	}
	if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
		return domain.OperatorAction{}, fmt.Errorf("save comment: %w", err)
	}
	if err := s.publishOALifecycle(ctx, eventbus.OALifecycleEvent{
		ProjectID: oa.ProjectID,
		SpaceID:   oa.SpaceID,
		ActionID:  string(oa.ID),
		TaskRef:   oa.TaskRef,
		EventType: "oa.comment",
		OldStatus: string(oa.Status),
		NewStatus: string(oa.Status),
		Title:     oa.Title,
		Category:  string(oa.Category),
		Urgency:   string(oa.Urgency),
		Author:    comment.Author,
		Text:      comment.Text,
		Blocking:  oa.Blocking,
		Timestamp: comment.CreatedAt,
	}); err != nil {
		return oa, fmt.Errorf("publish oa.comment: %w", err)
	}
	if err := s.tryPublishCommentMessage(ctx, oa, comment); err != nil {
		return oa, fmt.Errorf("publish comment message: %w", err)
	}
	return oa, nil
}

func (s *Service) AddAttachment(ctx context.Context, id domain.OperatorActionID, attachment domain.Attachment) (domain.OperatorAction, error) {
	oa, err := s.actionRepo.GetAction(ctx, id)
	if err != nil {
		return domain.OperatorAction{}, err
	}
	if err := oa.AddAttachment(attachment); err != nil {
		return domain.OperatorAction{}, err
	}
	if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
		return domain.OperatorAction{}, fmt.Errorf("save attachment: %w", err)
	}
	return oa, nil
}

func (s *Service) GetAttachment(ctx context.Context, attachmentID string) (domain.OperatorAction, domain.Attachment, error) {
	oa, err := s.actionRepo.FindActionByAttachmentID(ctx, attachmentID)
	if err != nil {
		return domain.OperatorAction{}, domain.Attachment{}, err
	}
	for _, attachment := range oa.Attachments {
		if strings.TrimSpace(attachment.ID) == strings.TrimSpace(attachmentID) {
			return oa, attachment, nil
		}
	}
	return domain.OperatorAction{}, domain.Attachment{}, fmt.Errorf("attachment not found: %s", attachmentID)
}

func (s *Service) List(ctx context.Context, projectID string, filter domain.ActionFilter) ([]domain.OperatorAction, error) {
	return s.actionRepo.FindActionsByProject(ctx, projectID, filter)
}

func (s *Service) ListPending(ctx context.Context, projectID string) ([]domain.OperatorAction, error) {
	return s.actionRepo.FindPendingActions(ctx, projectID)
}

func (s *Service) CountByStatus(ctx context.Context, projectID string) (map[domain.OAStatus]int, error) {
	return s.actionRepo.CountActionsByStatus(ctx, projectID)
}

func (s *Service) transitionAction(ctx context.Context, id domain.OperatorActionID, eventType string, mutate func(*domain.OperatorAction) error, afterPersist func(*domain.OperatorAction) error) (domain.OperatorAction, error) {
	oa, err := s.actionRepo.GetAction(ctx, id)
	if err != nil {
		return domain.OperatorAction{}, err
	}
	oldStatus := oa.Status
	if err := mutate(&oa); err != nil {
		return domain.OperatorAction{}, err
	}
	if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
		return domain.OperatorAction{}, fmt.Errorf("save %s: %w", eventType, err)
	}
	if afterPersist != nil {
		if err := afterPersist(&oa); err != nil {
			return oa, fmt.Errorf("after-persist %s: %w", eventType, err)
		}
	}
	if err := s.publishOALifecycle(ctx, eventbus.OALifecycleEvent{
		ProjectID:     oa.ProjectID,
		SpaceID:       oa.SpaceID,
		ActionID:      string(oa.ID),
		TaskRef:       oa.TaskRef,
		EventType:     eventType,
		OldStatus:     string(oldStatus),
		NewStatus:     string(oa.Status),
		Title:         oa.Title,
		Urgency:       string(oa.Urgency),
		Category:      string(oa.Category),
		OutcomeStatus: string(oa.OutcomeStatus),
		Blocking:      oa.Blocking,
		Timestamp:     time.Now().UTC(),
	}); err != nil {
		return oa, fmt.Errorf("publish %s: %w", eventType, err)
	}
	return oa, nil
}

func (s *Service) tryCreateDecisionOnActionComplete(ctx context.Context, oa *domain.OperatorAction) {
	decision := decisiondomain.Decision{
		ProjectID:         oa.ProjectID,
		SpaceID:           oa.SpaceID,
		Source:            decisiondomain.DecisionSourceOperator,
		Title:             fmt.Sprintf("Completed: %s", oa.Title),
		Confidence:        1.0,
		TaskRef:           oa.TaskRef,
		KeyResultRef:      oa.KeyResultRef,
		OperatorActionRef: string(oa.ID),
		Log:               &decisiondomain.LogPayload{Rationale: oa.OutcomeSummary},
	}
	decisionID, err := s.ports.Decisions.CreateDecision(ctx, decision)
	if err != nil {
		s.logger.Error("failed to create decision on OA completion", "oaID", oa.ID, "error", err)
		return
	}
	confidence := 1.0
	_, _, err = s.ports.Graph.Link(ctx, graphdomain.GraphLinkRequest{
		ProjectID:  oa.ProjectID,
		SourceType: graphdomain.NodeTypeOperatorAction,
		SourceID:   string(oa.ID),
		TargetType: graphdomain.NodeTypeDecision,
		TargetID:   string(decisionID),
		EdgeType:   graphdomain.EdgeTypeCompletedBy,
		Confidence: &confidence,
		Rationale:  "operator action completed by decision",
		Origin:     "reference",
		CreatedBy:  "operator_service",
	})
	if err != nil {
		s.logger.Error("failed to create completed_by link", "oaID", oa.ID, "error", err)
	}
}

func (s *Service) tryUnblockActionTaskOnComplete(ctx context.Context, oa *domain.OperatorAction) error {
	if oa == nil || !oa.Blocking || strings.TrimSpace(oa.TaskRef) == "" {
		return nil
	}
	if err := s.ports.Tasks.UnblockTask(ctx, strings.TrimSpace(oa.TaskRef), string(oa.ID)); err != nil {
		return fmt.Errorf("unblock task %s for OA %s: %w", oa.TaskRef, oa.ID, err)
	}
	return nil
}

func (s *Service) tryPublishCompletionMessage(ctx context.Context, oa *domain.OperatorAction) error {
	if err := s.ports.Messages.CompleteAction(ctx, *oa); err != nil {
		return fmt.Errorf("publish completion message for OA %s: %w", oa.ID, err)
	}
	return nil
}

func (s *Service) tryPublishCommentMessage(ctx context.Context, oa domain.OperatorAction, comment domain.Comment) error {
	if !strings.EqualFold(strings.TrimSpace(comment.Author), "operator") {
		return nil
	}
	if err := s.ports.Messages.CommentOnAction(ctx, oa, comment); err != nil {
		return fmt.Errorf("publish comment message for OA %s: %w", oa.ID, err)
	}
	return nil
}

func (s *Service) tryPublishBlockedMessage(ctx context.Context, oa *domain.OperatorAction, reason string) error {
	if err := s.ports.Messages.BlockAction(ctx, *oa, reason); err != nil {
		return fmt.Errorf("publish blocked message for OA %s: %w", oa.ID, err)
	}
	return nil
}

// CreateEscalation builds a new escalation, persists it, and applies side effects.
func (s *Service) CreateEscalation(ctx context.Context, params CreateEscalationParams) (domain.Escalation, error) {
	if strings.TrimSpace(params.TaskRef) != "" {
		since := time.Now().UTC().Add(-dedupWindow)
		existing, found, err := s.escalationRepo.FindPendingEscalationDuplicate(ctx, params.SpaceID, params.TaskRef, params.Category, params.Urgency, since)
		if err != nil {
			return domain.Escalation{}, fmt.Errorf("dedup check: %w", err)
		}
		if found {
			s.logger.Info("escalation dedup: returning existing pending escalation", "existingID", existing.ID, "spaceID", params.SpaceID, "taskRef", params.TaskRef, "category", params.Category, "urgency", params.Urgency)
			return existing, nil
		}
	}

	now := time.Now().UTC()
	esc := domain.Escalation{
		ID:             domain.EscalationID("esc-" + uuid.NewString()),
		ProjectID:      params.ProjectID,
		SpaceID:        params.SpaceID,
		TaskRef:        params.TaskRef,
		KeyResultRef:   params.KeyResultRef,
		MissionRef:     params.MissionRef,
		Source:         domain.Source(strings.TrimSpace(params.Source)),
		MemberID:       params.MemberID,
		Category:       params.Category,
		Urgency:        params.Urgency,
		Title:          params.Title,
		Description:    params.Description,
		Recommendation: params.Recommendation,
		Confidence:     params.Confidence,
		Status:         domain.StatusPending,
		Deadline:       params.Deadline,
		Metadata:       params.Metadata,
		CreatedAt:      now,
	}
	if err := esc.Validate(); err != nil {
		return domain.Escalation{}, fmt.Errorf("validate escalation: %w", err)
	}
	if err := s.escalationRepo.SaveEscalation(ctx, esc); err != nil {
		return domain.Escalation{}, fmt.Errorf("save escalation: %w", err)
	}
	if strings.TrimSpace(esc.TaskRef) != "" {
		if err := s.ports.Tasks.BlockTask(ctx, esc.TaskRef, string(esc.ID)); err != nil {
			return domain.Escalation{}, fmt.Errorf("block task %s: %w", esc.TaskRef, err)
		}
	}
	if err := s.linkBlockedBy(ctx, esc.ProjectID, now, strings.TrimSpace(esc.TaskRef), strings.TrimSpace(esc.KeyResultRef), strings.TrimSpace(esc.MissionRef), graphdomain.NodeTypeEscalation, string(esc.ID)); err != nil {
		return domain.Escalation{}, err
	}
	if err := s.publishEscalationLifecycle(ctx, eventbus.EscalationLifecycleEvent{
		ProjectID:    esc.ProjectID,
		SpaceID:      esc.SpaceID,
		EscalationID: string(esc.ID),
		TaskRef:      esc.TaskRef,
		EventType:    "escalation.created",
		OldStatus:    "",
		NewStatus:    string(esc.Status),
		Title:        esc.Title,
		Urgency:      string(esc.Urgency),
		Category:     string(esc.Category),
		Timestamp:    now,
	}); err != nil {
		return esc, fmt.Errorf("publish escalation.created: %w", err)
	}
	return esc, nil
}

func (s *Service) ResolveEscalation(ctx context.Context, id domain.EscalationID, params ResolveEscalationParams) (domain.Escalation, error) {
	if strings.TrimSpace(params.ResolvedBy) == "" {
		return domain.Escalation{}, fmt.Errorf("resolvedBy is required")
	}
	if err := domain.ValidateResolution(params.Resolution); err != nil {
		return domain.Escalation{}, err
	}
	esc, err := s.escalationRepo.GetEscalation(ctx, id)
	if err != nil {
		return domain.Escalation{}, fmt.Errorf("get escalation %s: %w", id, err)
	}
	if err := esc.CanTransitionTo(domain.StatusResolved); err != nil {
		return domain.Escalation{}, err
	}
	now := time.Now().UTC()
	oldStatus := esc.Status
	esc.Status = domain.StatusResolved
	esc.Resolution = params.Resolution
	esc.ResolutionNote = params.ResolutionNote
	esc.ResolvedBy = params.ResolvedBy
	esc.ResolvedAt = &now
	if err := s.escalationRepo.SaveEscalation(ctx, esc); err != nil {
		return domain.Escalation{}, fmt.Errorf("save resolved escalation %s: %w", id, err)
	}
	// Resolution is binary now (approve | reject); both unblock the task.
	// "Defer" semantics are now expressed by simply leaving the escalation
	// pending rather than by a special resolution value.
	if err := s.tryUnblockEscalationTask(ctx, &esc); err != nil {
		return domain.Escalation{}, err
	}
	decision := decisiondomain.Decision{
		ID:             decisiondomain.DecisionID("dec-" + uuid.NewString()),
		ProjectID:      esc.ProjectID,
		SpaceID:        esc.SpaceID,
		Source:         decisiondomain.DecisionSourceOperator,
		SourceIdentity: params.ResolvedBy,
		Title:          fmt.Sprintf("Resolved: %s", esc.Title),
		Confidence:     1.0,
		TaskRef:        esc.TaskRef,
		KeyResultRef:   esc.KeyResultRef,
		EscalationRef:  string(esc.ID),
		CreatedAt:      now,
		Log: &decisiondomain.LogPayload{
			Rationale: fmt.Sprintf("Resolution: %s. %s", params.Resolution, strings.TrimSpace(params.ResolutionNote)),
		},
	}
	decisionID, err := s.ports.Decisions.CreateDecision(ctx, decision)
	if err != nil {
		return domain.Escalation{}, fmt.Errorf("create decision for escalation %s: %w", id, err)
	}
	confidence := 1.0
	if _, _, err := s.ports.Graph.Link(ctx, graphdomain.GraphLinkRequest{
		ProjectID:  esc.ProjectID,
		SourceType: graphdomain.NodeTypeEscalation,
		SourceID:   string(esc.ID),
		TargetType: graphdomain.NodeTypeDecision,
		TargetID:   string(decisionID),
		EdgeType:   graphdomain.EdgeTypeResolvedBy,
		Confidence: &confidence,
		Rationale:  "escalation resolved by decision",
		Origin:     "reference",
		CreatedBy:  "operator_service",
	}); err != nil {
		return domain.Escalation{}, fmt.Errorf("create resolved_by link for escalation %s: %w", esc.ID, err)
	}
	if err := s.publishEscalationLifecycle(ctx, eventbus.EscalationLifecycleEvent{ProjectID: esc.ProjectID, SpaceID: esc.SpaceID, EscalationID: string(esc.ID), TaskRef: esc.TaskRef, EventType: "escalation.resolved", OldStatus: string(oldStatus), NewStatus: string(esc.Status), Resolution: string(params.Resolution), ResolvedBy: params.ResolvedBy, Title: esc.Title, Urgency: string(esc.Urgency), Category: string(esc.Category), Timestamp: now}); err != nil {
		return esc, fmt.Errorf("publish escalation.resolved: %w", err)
	}
	if err := s.ports.Messages.ResolveEscalation(ctx, esc, params); err != nil {
		return esc, fmt.Errorf("resolve notify for escalation %s: %w", esc.ID, err)
	}
	return esc, nil
}

func (s *Service) CancelEscalation(ctx context.Context, id domain.EscalationID) (domain.Escalation, error) {
	esc, err := s.escalationRepo.GetEscalation(ctx, id)
	if err != nil {
		return domain.Escalation{}, fmt.Errorf("get escalation %s: %w", id, err)
	}
	if err := esc.CanTransitionTo(domain.StatusCanceled); err != nil {
		return domain.Escalation{}, err
	}
	now := time.Now().UTC()
	oldStatus := esc.Status
	esc.Status = domain.StatusCanceled
	if err := s.escalationRepo.SaveEscalation(ctx, esc); err != nil {
		return domain.Escalation{}, fmt.Errorf("save canceled escalation %s: %w", id, err)
	}
	if err := s.tryUnblockEscalationTask(ctx, &esc); err != nil {
		return domain.Escalation{}, err
	}
	if err := s.publishEscalationLifecycle(ctx, eventbus.EscalationLifecycleEvent{ProjectID: esc.ProjectID, SpaceID: esc.SpaceID, EscalationID: string(esc.ID), TaskRef: esc.TaskRef, EventType: "escalation.canceled", OldStatus: string(oldStatus), NewStatus: string(esc.Status), Title: esc.Title, Urgency: string(esc.Urgency), Category: string(esc.Category), Timestamp: now}); err != nil {
		return esc, fmt.Errorf("publish escalation.canceled: %w", err)
	}
	return esc, nil
}

func (s *Service) GetEscalation(ctx context.Context, id domain.EscalationID) (domain.Escalation, error) {
	return s.escalationRepo.GetEscalation(ctx, id)
}

func (s *Service) ListEscalations(ctx context.Context, projectID string, filter domain.EscalationFilter) ([]domain.Escalation, error) {
	return s.escalationRepo.FindEscalationsByProject(ctx, projectID, filter)
}

func (s *Service) CountPendingEscalations(ctx context.Context, projectID string) (int, error) {
	return s.escalationRepo.CountPendingEscalations(ctx, projectID)
}

func (s *Service) EscalateOverdue(ctx context.Context) int {
	now := time.Now().UTC()
	overdue, err := s.escalationRepo.FindExpiredPendingEscalations(ctx, now)
	if err != nil {
		s.logger.Error("deadline loop: failed to find expired pending", "error", err)
		return 0
	}
	failCount := 0
	for _, esc := range overdue {
		if esc.Urgency == domain.UrgencyCritical {
			continue
		}
		newUrgency, err := domain.EscalateUrgency(esc.Urgency)
		if err != nil {
			s.logger.Error("deadline loop: escalate urgency failed", "escalationID", esc.ID, "urgency", esc.Urgency, "error", err)
			failCount++
			continue
		}
		originalUrgency := esc.OriginalUrgency
		if originalUrgency == "" {
			originalUrgency = esc.Urgency
		}
		if err := s.escalationRepo.EscalateEscalationUrgency(ctx, esc.ID, newUrgency, originalUrgency, now); err != nil {
			s.logger.Error("deadline loop: persist escalation failed", "escalationID", esc.ID, "error", err)
			failCount++
			continue
		}
		if err := s.publishEscalationLifecycle(ctx, eventbus.EscalationLifecycleEvent{ProjectID: esc.ProjectID, SpaceID: esc.SpaceID, EscalationID: string(esc.ID), TaskRef: esc.TaskRef, EventType: "escalation.escalated", OldStatus: string(esc.Status), NewStatus: string(esc.Status), Title: esc.Title, Urgency: string(newUrgency), PreviousUrgency: string(esc.Urgency), NewUrgency: string(newUrgency), Category: string(esc.Category), Timestamp: now}); err != nil {
			s.logger.Error("deadline loop: publish event failed", "escalationID", esc.ID, "error", err)
			failCount++
			continue
		}
		s.logger.Info("deadline loop: escalated urgency", "escalationID", esc.ID, "from", esc.Urgency, "to", newUrgency)
	}
	return failCount
}

func (s *Service) ArchiveCompleted(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	completed, err := s.actionRepo.FindActionsByProject(ctx, "", domain.ActionFilter{Status: []domain.OAStatus{domain.OAStatusCompleted, domain.OAStatusCanceled}})
	if err != nil {
		s.logger.Error("retention: failed to list completed OAs", "error", err)
		return
	}
	archived := 0
	for _, oa := range completed {
		if oa.CompletedAt == nil || !oa.CompletedAt.Before(cutoff) {
			continue
		}
		if oa.Metadata == nil {
			oa.Metadata = map[string]string{}
		}
		if oa.Metadata["archivedAt"] != "" {
			continue
		}
		oa.Metadata["archivedAt"] = time.Now().UTC().Format(time.RFC3339)
		if err := s.actionRepo.SaveAction(ctx, oa); err != nil {
			s.logger.Error("retention: failed to archive OA", "oaID", oa.ID, "error", err)
			continue
		}
		archived++
	}
	if archived > 0 {
		slog.Info("retention: archived operator actions", "count", archived, "retentionDays", retentionDays)
	}
}

func (s *Service) ArchiveResolved(ctx context.Context, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	resolved, err := s.escalationRepo.FindEscalationsByProject(ctx, "", domain.EscalationFilter{Status: []domain.Status{domain.StatusResolved, domain.StatusCanceled}})
	if err != nil {
		s.logger.Error("retention: failed to list resolved escalations", "error", err)
		return
	}
	archived := 0
	for _, esc := range resolved {
		if esc.ResolvedAt == nil || !esc.ResolvedAt.Before(cutoff) {
			continue
		}
		if esc.Metadata == nil {
			esc.Metadata = map[string]string{}
		}
		if esc.Metadata["archivedAt"] != "" {
			continue
		}
		esc.Metadata["archivedAt"] = time.Now().UTC().Format(time.RFC3339)
		if err := s.escalationRepo.SaveEscalation(ctx, esc); err != nil {
			s.logger.Error("retention: failed to archive escalation", "escalationID", esc.ID, "error", err)
			continue
		}
		archived++
	}
	if archived > 0 {
		slog.Info("retention: archived escalations", "count", archived, "retentionDays", retentionDays)
	}
}

func (s *Service) publishOALifecycle(ctx context.Context, event eventbus.OALifecycleEvent) error {
	if err := s.ports.Events.PublishOperatorEvent(ctx, event); err != nil {
		return fmt.Errorf("publish oa lifecycle: %w", err)
	}
	return nil
}

func (s *Service) publishEscalationLifecycle(ctx context.Context, event eventbus.EscalationLifecycleEvent) error {
	if err := s.ports.Events.PublishOperatorEvent(ctx, event); err != nil {
		return fmt.Errorf("publish escalation lifecycle: %w", err)
	}
	return nil
}

func (s *Service) tryUnblockEscalationTask(ctx context.Context, esc *domain.Escalation) error {
	if strings.TrimSpace(esc.TaskRef) == "" {
		return nil
	}
	if err := s.ports.Tasks.UnblockTask(ctx, esc.TaskRef, string(esc.ID)); err != nil {
		return fmt.Errorf("unblock task %s: %w", esc.TaskRef, err)
	}
	return nil
}

func (s *Service) linkBlockedBy(ctx context.Context, projectID string, createdAt time.Time, taskRef, keyResultRef, missionRef, targetType, targetID string) error {
	var sourceType, sourceID string
	switch {
	case taskRef != "":
		sourceType, sourceID = graphdomain.NodeTypeTask, taskRef
	case keyResultRef != "":
		sourceType, sourceID = graphdomain.NodeTypeKeyResult, keyResultRef
	case missionRef != "":
		sourceType, sourceID = graphdomain.NodeTypeMission, missionRef
	default:
		return nil
	}
	confidence := 1.0
	if _, _, err := s.ports.Graph.Link(ctx, graphdomain.GraphLinkRequest{
		ProjectID:  projectID,
		SourceType: sourceType,
		SourceID:   sourceID,
		TargetType: targetType,
		TargetID:   targetID,
		EdgeType:   graphdomain.EdgeTypeBlockedBy,
		Confidence: &confidence,
		Rationale:  fmt.Sprintf("%s blocked by %s", sourceType, targetType),
		Origin:     "reference",
		CreatedBy:  "operator_service",
	}); err != nil {
		return fmt.Errorf("create blocked_by link for %s %s: %w", targetType, targetID, err)
	}
	return nil
}
