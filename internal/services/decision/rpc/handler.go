package rpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/decision/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

// Handler adapts the decision application service to RPC protocol types.
type Handler struct {
	svc           *decisionapp.Service
	memberDisplay MemberDisplayLookup
	userDisplay   UserDisplayLookup
}

// MemberDisplayLookup resolves a member id to its display name. Used to
// populate DecisionView.MemberName so list views never have to render
// the raw member id. Satisfied by *spaceapp.Service.
type MemberDisplayLookup interface {
	DisplayName(ctx context.Context, id member.ID) (string, error)
}

// UserDisplayLookup resolves the authenticated operator to a display name.
// Operator-authored decisions store SourceIdentity as the stable sentinel
// "operator"; the list view needs the actual account display name instead.
type UserDisplayLookup interface {
	CurrentUserDisplayName(ctx context.Context) (string, error)
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

// SetUserDisplayLookup wires the current-user display resolver used for
// operator-authored decisions.
func (h *Handler) SetUserDisplayLookup(lookup UserDisplayLookup) {
	if h == nil {
		return
	}
	h.userDisplay = lookup
}

// Create handles decision.create RPC calls.
func (h *Handler) Create(ctx context.Context, p protocol.DecisionCreateParams) (protocol.DecisionCreateResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return protocol.DecisionCreateResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "projectId is required",
		}
	}
	if strings.TrimSpace(p.Title) == "" {
		return protocol.DecisionCreateResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "title is required",
		}
	}
	kind := domain.DecisionKind(strings.TrimSpace(firstNonEmpty(p.Kind, string(domain.DecisionKindLog))))

	d := domain.Decision{
		ProjectID:         p.ProjectID,
		SpaceID:           p.SpaceID,
		Source:            domain.DecisionSource(p.Source),
		SourceIdentity:    p.SourceIdentity,
		RunID:             p.RunID,
		Title:             p.Title,
		Confidence:        p.Confidence,
		TaskRef:           p.TaskRef,
		KeyResultRef:      p.KeyResultRef,
		MissionRef:        p.MissionRef,
		PlanRef:           p.PlanRef,
		OperatorActionRef: p.OperatorActionRef,
		EscalationRef:     p.EscalationRef,
		CorrelationRef:    p.CorrelationRef,
		InformedByRef:     p.InformedByRef,
		Tags:              p.Tags,
		Metadata:          p.Metadata,
	}
	switch kind {
	case domain.DecisionKindLog:
		if strings.TrimSpace(p.Rationale) == "" {
			return protocol.DecisionCreateResult{}, &protocol.ProtocolError{
				Code: protocol.CodeInvalidParams, Message: "rationale is required",
			}
		}
		d.Log = &domain.LogPayload{
			Rationale:              p.Rationale,
			Context:                p.Context,
			AlternativesRejected:   p.AlternativesRejected,
			InvalidationConditions: append([]string(nil), p.InvalidationConditions...),
			Outcome:                p.Outcome,
		}
	case domain.DecisionKindAskUser:
		d.AskUser = &domain.AskUserPayload{
			Context:   p.Context,
			Questions: p.Questions,
			Answers:   p.Answers,
			Cancelled: p.Cancelled,
		}
	default:
		return protocol.DecisionCreateResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: fmt.Sprintf("unknown decision kind %q (must be 'log' or 'ask_user')", kind),
		}
	}

	created, err := h.svc.Create(ctx, d)
	if err != nil {
		return protocol.DecisionCreateResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: fmt.Sprintf("create decision: %v", err),
		}
	}
	return protocol.DecisionCreateResult{Decision: h.decisionToView(ctx, created)}, nil
}

// Get handles decision.get RPC calls.
func (h *Handler) Get(ctx context.Context, p protocol.DecisionGetParams) (protocol.DecisionGetResult, error) {
	id := strings.TrimSpace(p.DecisionID)
	if id == "" {
		return protocol.DecisionGetResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "decisionId is required",
		}
	}

	d, err := h.svc.Get(ctx, domain.DecisionID(id))
	if err != nil {
		return protocol.DecisionGetResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: fmt.Sprintf("get decision: %v", err),
		}
	}
	return protocol.DecisionGetResult{Decision: h.decisionToView(ctx, d)}, nil
}

// Delete handles decision.delete RPC calls.
func (h *Handler) Delete(ctx context.Context, p protocol.DecisionDeleteParams) (protocol.DecisionDeleteResult, error) {
	id := strings.TrimSpace(p.DecisionID)
	if id == "" {
		return protocol.DecisionDeleteResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "decisionId is required",
		}
	}

	if err := h.svc.Delete(ctx, domain.DecisionID(id)); err != nil {
		return protocol.DecisionDeleteResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: fmt.Sprintf("delete decision: %v", err),
		}
	}
	return protocol.DecisionDeleteResult{Deleted: true}, nil
}

// List handles decision.list RPC calls.
func (h *Handler) List(ctx context.Context, p protocol.DecisionListParams) (protocol.DecisionListResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return protocol.DecisionListResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "projectId is required",
		}
	}

	filter := decisionFilterFromParams(
		projectID,
		p.Source,
		p.SpaceID,
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
		return protocol.DecisionListResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: fmt.Sprintf("list decisions: %v", err),
		}
	}

	views := make([]protocol.DecisionView, len(decisions))
	for i, d := range decisions {
		views[i] = h.decisionToView(ctx, d)
	}
	return protocol.DecisionListResult{Decisions: views}, nil
}

// Count handles decision.count RPC calls.
func (h *Handler) Count(ctx context.Context, p protocol.DecisionCountParams) (protocol.DecisionCountResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return protocol.DecisionCountResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "projectId is required",
		}
	}

	filter := decisionFilterFromParams(
		projectID,
		p.Source,
		p.SpaceID,
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
		return protocol.DecisionCountResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: fmt.Sprintf("count decisions: %v", err),
		}
	}
	return protocol.DecisionCountResult{Count: count}, nil
}

// Export handles decision.export RPC calls.
func (h *Handler) Export(ctx context.Context, p protocol.DecisionExportParams) (protocol.DecisionExportResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return protocol.DecisionExportResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "projectId is required",
		}
	}

	filter := decisionFilterFromParams(
		projectID,
		p.Source,
		p.SpaceID,
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
		return protocol.DecisionExportResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: fmt.Sprintf("export decisions: %v", err),
		}
	}

	views := make([]protocol.DecisionView, len(decisions))
	for i, d := range decisions {
		views[i] = h.decisionToView(ctx, d)
	}
	return protocol.DecisionExportResult{Decisions: views}, nil
}

// ---------------------------------------------------------------------------
// View helper
// ---------------------------------------------------------------------------

func (h *Handler) decisionToView(ctx context.Context, d domain.Decision) protocol.DecisionView {
	view := protocol.DecisionView{
		ID:                string(d.ID),
		ProjectID:         d.ProjectID,
		SpaceID:           d.SpaceID,
		Source:            string(d.Source),
		Kind:              string(d.Kind()),
		MemberID:          d.SourceIdentity,
		MemberName:        h.resolveActorDisplay(ctx, d),
		SourceIdentity:    d.SourceIdentity,
		RunID:             d.RunID,
		Title:             d.Title,
		Confidence:        d.Confidence,
		TaskRef:           d.TaskRef,
		KeyResultRef:      d.KeyResultRef,
		MissionRef:        d.MissionRef,
		PlanRef:           d.PlanRef,
		OperatorActionRef: d.OperatorActionRef,
		EscalationRef:     d.EscalationRef,
		CorrelationRef:    d.CorrelationRef,
		InformedByRef:     d.InformedByRef,
		Tags:              d.Tags,
		Metadata:          d.Metadata,
		CreatedAt:         d.CreatedAt,
	}
	if p := d.Log; p != nil {
		view.Rationale = p.Rationale
		view.Context = p.Context
		view.AlternativesRejected = p.AlternativesRejected
		view.InvalidationConditions = append([]string(nil), p.InvalidationConditions...)
		view.Outcome = p.Outcome
	}
	if p := d.AskUser; p != nil {
		view.Context = p.Context
		view.Questions = p.Questions
		view.Answers = p.Answers
		view.Cancelled = p.Cancelled
	}
	return view
}

func (h *Handler) resolveActorDisplay(ctx context.Context, d domain.Decision) string {
	if d.Source == domain.DecisionSourceOperator {
		return h.resolveCurrentUserDisplay(ctx)
	}
	return h.resolveMemberDisplay(ctx, d.SourceIdentity)
}

func (h *Handler) resolveCurrentUserDisplay(ctx context.Context) string {
	if h == nil || h.userDisplay == nil {
		return ""
	}
	name, err := h.userDisplay.CurrentUserDisplayName(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(name)
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
	spaceID string,
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
		SpaceID:   strings.TrimSpace(spaceID),
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
