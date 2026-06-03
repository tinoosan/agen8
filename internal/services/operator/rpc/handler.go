package rpc

import (
	"context"
	"fmt"
	"strings"

	operatorapp "github.com/tinoosan/agen8-mcp-server/internal/services/operator/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

type Handler struct {
	svc *operatorapp.Service
}

func NewHandler(svc *operatorapp.Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) CreateEscalation(ctx context.Context, p protocol.EscalationCreateParams) (protocol.EscalationCreateResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return protocol.EscalationCreateResult{}, protoErr(protocol.CodeInvalidParams, "projectId is required")
	}
	if strings.TrimSpace(p.Title) == "" {
		return protocol.EscalationCreateResult{}, protoErr(protocol.CodeInvalidParams, "title is required")
	}
	if strings.TrimSpace(p.Description) == "" {
		return protocol.EscalationCreateResult{}, protoErr(protocol.CodeInvalidParams, "description is required")
	}
	esc, err := h.svc.CreateEscalation(ctx, operatorapp.CreateEscalationParams{
		ProjectID:      p.ProjectID,
		SpaceID:        p.SpaceID,
		TaskRef:        p.TaskRef,
		KeyResultRef:   p.KeyResultRef,
		MissionRef:     p.MissionRef,
		Source:         p.Source,
		MemberID:       p.MemberID,
		Category:       domain.Category(strings.TrimSpace(p.Category)),
		Urgency:        domain.Urgency(strings.TrimSpace(p.Urgency)),
		Title:          p.Title,
		Description:    p.Description,
		Recommendation: p.Recommendation,
		Confidence:     p.Confidence,
		Deadline:       p.Deadline,
		Metadata:       p.Metadata,
	})
	if err != nil {
		return protocol.EscalationCreateResult{}, internalErr("create escalation", err)
	}
	return protocol.EscalationCreateResult{Escalation: escalationToView(esc)}, nil
}

func (h *Handler) GetEscalation(ctx context.Context, p protocol.EscalationGetParams) (protocol.EscalationGetResult, error) {
	id := strings.TrimSpace(p.EscalationID)
	if id == "" {
		return protocol.EscalationGetResult{}, protoErr(protocol.CodeInvalidParams, "escalationId is required")
	}
	esc, err := h.svc.GetEscalation(ctx, domain.EscalationID(id))
	if err != nil {
		return protocol.EscalationGetResult{}, internalErr("get escalation", err)
	}
	return protocol.EscalationGetResult{Escalation: escalationToView(esc)}, nil
}

func (h *Handler) ListEscalations(ctx context.Context, p protocol.EscalationListParams) (protocol.EscalationListResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return protocol.EscalationListResult{}, protoErr(protocol.CodeInvalidParams, "projectId is required")
	}
	filter := domain.EscalationFilter{SpaceID: strings.TrimSpace(p.SpaceID), Limit: p.Limit, Offset: p.Offset}
	for _, status := range p.Status {
		if trimmed := strings.TrimSpace(status); trimmed != "" {
			filter.Status = append(filter.Status, domain.Status(trimmed))
		}
	}
	for _, urgency := range p.Urgency {
		if trimmed := strings.TrimSpace(urgency); trimmed != "" {
			filter.Urgency = append(filter.Urgency, domain.Urgency(trimmed))
		}
	}
	for _, category := range p.Category {
		if trimmed := strings.TrimSpace(category); trimmed != "" {
			filter.Category = append(filter.Category, domain.Category(trimmed))
		}
	}
	escalations, err := h.svc.ListEscalations(ctx, projectID, filter)
	if err != nil {
		return protocol.EscalationListResult{}, internalErr("list escalations", err)
	}
	return protocol.EscalationListResult{Escalations: escalationsToView(escalations)}, nil
}

func (h *Handler) ListPendingEscalations(ctx context.Context, p protocol.EscalationListPendingParams) (protocol.EscalationListPendingResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return protocol.EscalationListPendingResult{}, protoErr(protocol.CodeInvalidParams, "projectId is required")
	}
	escalations, err := h.svc.ListEscalations(ctx, projectID, domain.EscalationFilter{Status: []domain.Status{domain.StatusPending}})
	if err != nil {
		return protocol.EscalationListPendingResult{}, internalErr("list pending escalations", err)
	}
	return protocol.EscalationListPendingResult{Escalations: escalationsToView(escalations)}, nil
}

func (h *Handler) ResolveEscalation(ctx context.Context, p protocol.EscalationResolveParams) (protocol.EscalationResolveResult, error) {
	id := strings.TrimSpace(p.EscalationID)
	if id == "" {
		return protocol.EscalationResolveResult{}, protoErr(protocol.CodeInvalidParams, "escalationId is required")
	}
	if strings.TrimSpace(p.Resolution) == "" {
		return protocol.EscalationResolveResult{}, protoErr(protocol.CodeInvalidParams, "resolution is required")
	}
	if strings.TrimSpace(p.ResolvedBy) == "" {
		return protocol.EscalationResolveResult{}, protoErr(protocol.CodeInvalidParams, "resolvedBy is required")
	}
	esc, err := h.svc.ResolveEscalation(ctx, domain.EscalationID(id), operatorapp.ResolveEscalationParams{
		Resolution:     domain.Resolution(strings.TrimSpace(p.Resolution)),
		ResolutionNote: p.ResolutionNote,
		ResolvedBy:     p.ResolvedBy,
	})
	if err != nil {
		return protocol.EscalationResolveResult{}, internalErr("resolve escalation", err)
	}
	return protocol.EscalationResolveResult{Escalation: escalationToView(esc)}, nil
}

func (h *Handler) CancelEscalation(ctx context.Context, p protocol.EscalationCancelParams) (protocol.EscalationCancelResult, error) {
	id := strings.TrimSpace(p.EscalationID)
	if id == "" {
		return protocol.EscalationCancelResult{}, protoErr(protocol.CodeInvalidParams, "escalationId is required")
	}
	esc, err := h.svc.CancelEscalation(ctx, domain.EscalationID(id))
	if err != nil {
		return protocol.EscalationCancelResult{}, internalErr("cancel escalation", err)
	}
	return protocol.EscalationCancelResult{Escalation: escalationToView(esc)}, nil
}

func (h *Handler) CountPendingEscalations(ctx context.Context, p protocol.EscalationCountPendingParams) (protocol.EscalationCountPendingResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return protocol.EscalationCountPendingResult{}, protoErr(protocol.CodeInvalidParams, "projectId is required")
	}
	count, err := h.svc.CountPendingEscalations(ctx, projectID)
	if err != nil {
		return protocol.EscalationCountPendingResult{}, internalErr("count pending escalations", err)
	}
	return protocol.EscalationCountPendingResult{Count: count}, nil
}

func (h *Handler) CreateAction(ctx context.Context, p protocol.OpActionCreateParams) (protocol.OpActionCreateResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return protocol.OpActionCreateResult{}, protoErr(protocol.CodeInvalidParams, "projectId is required")
	}
	if strings.TrimSpace(p.Title) == "" {
		return protocol.OpActionCreateResult{}, protoErr(protocol.CodeInvalidParams, "title is required")
	}
	oa, err := h.svc.Create(ctx, domain.CreateParams{
		ProjectID:            p.ProjectID,
		SpaceID:              p.SpaceID,
		TaskRef:              p.TaskRef,
		KeyResultRef:         p.KeyResultRef,
		MissionRef:           p.MissionRef,
		RunID:                p.RunID,
		Blocking:             p.Blocking,
		Source:               domain.OASource(p.Source),
		MemberID:             p.MemberID,
		EscalationRef:        p.EscalationRef,
		Category:             domain.Category(p.Category),
		Urgency:              domain.Urgency(p.Urgency),
		Title:                p.Title,
		Description:          p.Description,
		RequiresVerification: p.RequiresVerification,
		Deadline:             p.Deadline,
		Metadata:             p.Metadata,
	})
	if err != nil {
		return protocol.OpActionCreateResult{}, internalErr("create opAction", err)
	}
	return protocol.OpActionCreateResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) GetAction(ctx context.Context, p protocol.OpActionGetParams) (protocol.OpActionGetResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionGetResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	oa, err := h.svc.Get(ctx, domain.OperatorActionID(id))
	if err != nil {
		return protocol.OpActionGetResult{}, internalErr("get opAction", err)
	}
	return protocol.OpActionGetResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) ListActions(ctx context.Context, p protocol.OpActionListParams) (protocol.OpActionListResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return protocol.OpActionListResult{}, protoErr(protocol.CodeInvalidParams, "projectId is required")
	}
	filter := domain.ActionFilter{SpaceID: p.SpaceID, Limit: p.Limit, Offset: p.Offset}
	for _, s := range p.Status {
		filter.Status = append(filter.Status, domain.OAStatus(s))
	}
	for _, u := range p.Urgency {
		filter.Urgency = append(filter.Urgency, domain.Urgency(u))
	}
	for _, c := range p.Category {
		filter.Category = append(filter.Category, domain.Category(c))
	}
	list, err := h.svc.List(ctx, p.ProjectID, filter)
	if err != nil {
		return protocol.OpActionListResult{}, internalErr("list opAction", err)
	}
	return protocol.OpActionListResult{OpActions: actionsToView(list)}, nil
}

func (h *Handler) ListPendingActions(ctx context.Context, p protocol.OpActionListPendingParams) (protocol.OpActionListPendingResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return protocol.OpActionListPendingResult{}, protoErr(protocol.CodeInvalidParams, "projectId is required")
	}
	list, err := h.svc.ListPending(ctx, p.ProjectID)
	if err != nil {
		return protocol.OpActionListPendingResult{}, internalErr("list pending opAction", err)
	}
	return protocol.OpActionListPendingResult{OpActions: actionsToView(list)}, nil
}

func (h *Handler) AcknowledgeAction(ctx context.Context, p protocol.OpActionAcknowledgeParams) (protocol.OpActionAcknowledgeResult, error) {
	result, err := h.GetAction(ctx, protocol.OpActionGetParams(p))
	if err != nil {
		return protocol.OpActionAcknowledgeResult{}, err
	}
	return protocol.OpActionAcknowledgeResult(result), nil
}

func (h *Handler) StartAction(ctx context.Context, p protocol.OpActionStartParams) (protocol.OpActionStartResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionStartResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	oa, err := h.svc.Start(ctx, domain.OperatorActionID(id))
	if err != nil {
		return protocol.OpActionStartResult{}, internalErr("start opAction", err)
	}
	return protocol.OpActionStartResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) CompleteAction(ctx context.Context, p protocol.OpActionCompleteParams) (protocol.OpActionCompleteResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionCompleteResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	oa, err := h.svc.Complete(ctx, domain.OperatorActionID(id), domain.CompleteOutcome{OutcomeStatus: domain.OutcomeStatus(p.OutcomeStatus), OutcomeSummary: p.OutcomeSummary, OutcomePairs: p.OutcomePairs})
	if err != nil {
		return protocol.OpActionCompleteResult{}, internalErr("complete opAction", err)
	}
	return protocol.OpActionCompleteResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) VerifyAction(ctx context.Context, p protocol.OpActionVerifyParams) (protocol.OpActionVerifyResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionVerifyResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	oa, err := h.svc.Verify(ctx, domain.OperatorActionID(id), p.Accepted, p.Feedback, p.Author)
	if err != nil {
		return protocol.OpActionVerifyResult{}, internalErr("verify opAction", err)
	}
	return protocol.OpActionVerifyResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) BlockAction(ctx context.Context, p protocol.OpActionBlockParams) (protocol.OpActionBlockResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionBlockResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	if strings.TrimSpace(p.Reason) == "" {
		return protocol.OpActionBlockResult{}, protoErr(protocol.CodeInvalidParams, "reason is required")
	}
	oa, err := h.svc.Block(ctx, domain.OperatorActionID(id), p.Reason)
	if err != nil {
		return protocol.OpActionBlockResult{}, internalErr("block opAction", err)
	}
	return protocol.OpActionBlockResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) UnblockAction(ctx context.Context, p protocol.OpActionUnblockParams) (protocol.OpActionUnblockResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionUnblockResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	oa, err := h.svc.Unblock(ctx, domain.OperatorActionID(id))
	if err != nil {
		return protocol.OpActionUnblockResult{}, internalErr("unblock opAction", err)
	}
	return protocol.OpActionUnblockResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) CancelAction(ctx context.Context, p protocol.OpActionCancelParams) (protocol.OpActionCancelResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionCancelResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	oa, err := h.svc.Cancel(ctx, domain.OperatorActionID(id))
	if err != nil {
		return protocol.OpActionCancelResult{}, internalErr("cancel opAction", err)
	}
	return protocol.OpActionCancelResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) AddNote(ctx context.Context, p protocol.OpActionAddNoteParams) (protocol.OpActionAddNoteResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionAddNoteResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	if strings.TrimSpace(p.Text) == "" {
		return protocol.OpActionAddNoteResult{}, protoErr(protocol.CodeInvalidParams, "text is required")
	}
	oa, err := h.svc.AddProgressNote(ctx, domain.OperatorActionID(id), p.Text)
	if err != nil {
		return protocol.OpActionAddNoteResult{}, internalErr("add note", err)
	}
	return protocol.OpActionAddNoteResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) AddComment(ctx context.Context, p protocol.OpActionAddCommentParams) (protocol.OpActionAddCommentResult, error) {
	id := strings.TrimSpace(p.ActionID)
	if id == "" {
		return protocol.OpActionAddCommentResult{}, protoErr(protocol.CodeInvalidParams, "actionId is required")
	}
	if strings.TrimSpace(p.Author) == "" {
		return protocol.OpActionAddCommentResult{}, protoErr(protocol.CodeInvalidParams, "author is required")
	}
	if strings.TrimSpace(p.Text) == "" {
		return protocol.OpActionAddCommentResult{}, protoErr(protocol.CodeInvalidParams, "text is required")
	}
	oa, err := h.svc.AddComment(ctx, domain.OperatorActionID(id), p.Author, p.Text)
	if err != nil {
		return protocol.OpActionAddCommentResult{}, internalErr("add comment", err)
	}
	return protocol.OpActionAddCommentResult{OpAction: actionToView(oa)}, nil
}

func (h *Handler) CountActionStatus(ctx context.Context, p protocol.OpActionCountStatusParams) (protocol.OpActionCountStatusResult, error) {
	if strings.TrimSpace(p.ProjectID) == "" {
		return protocol.OpActionCountStatusResult{}, protoErr(protocol.CodeInvalidParams, "projectId is required")
	}
	counts, err := h.svc.CountByStatus(ctx, p.ProjectID)
	if err != nil {
		return protocol.OpActionCountStatusResult{}, internalErr("count opAction status", err)
	}
	out := make(map[string]int, len(counts))
	for key, value := range counts {
		out[string(key)] = value
	}
	return protocol.OpActionCountStatusResult{Counts: out}, nil
}

func escalationToView(esc domain.Escalation) protocol.EscalationView {
	return protocol.EscalationView{
		ID: string(esc.ID), ProjectID: esc.ProjectID, SpaceID: esc.SpaceID, TaskRef: esc.TaskRef,
		KeyResultRef: esc.KeyResultRef, MissionRef: esc.MissionRef,
		Source: string(esc.Source), MemberID: esc.MemberID, Category: string(esc.Category),
		Urgency: string(esc.Urgency), Title: esc.Title, Description: esc.Description,
		Recommendation: esc.Recommendation, Confidence: esc.Confidence, Status: string(esc.Status),
		Resolution: string(esc.Resolution), ResolutionNote: esc.ResolutionNote,
		Deadline: esc.Deadline, EscalatedAt: esc.EscalatedAt, OriginalUrgency: string(esc.OriginalUrgency),
		Metadata: esc.Metadata, CreatedAt: esc.CreatedAt, ResolvedAt: esc.ResolvedAt, ResolvedBy: esc.ResolvedBy,
	}
}

func escalationsToView(in []domain.Escalation) []protocol.EscalationView {
	out := make([]protocol.EscalationView, len(in))
	for i := range in {
		out[i] = escalationToView(in[i])
	}
	return out
}

func actionToView(oa domain.OperatorAction) protocol.OpActionView {
	v := protocol.OpActionView{
		ID: string(oa.ID), ProjectID: oa.ProjectID, SpaceID: oa.SpaceID, TaskRef: oa.TaskRef,
		KeyResultRef: oa.KeyResultRef, MissionRef: oa.MissionRef, RunID: oa.RunID, Blocking: oa.Blocking,
		Source: string(oa.Source), MemberID: oa.MemberID, EscalationRef: oa.EscalationRef,
		Category: string(oa.Category), Urgency: string(oa.Urgency), Title: oa.Title, Description: oa.Description,
		RequiresVerification: oa.RequiresVerification, Status: string(oa.Status), OutcomeStatus: string(oa.OutcomeStatus),
		OutcomeSummary: oa.OutcomeSummary, OutcomePairs: oa.OutcomePairs, Deadline: oa.Deadline,
		Metadata: oa.Metadata, CreatedAt: oa.CreatedAt, AcknowledgedAt: oa.AcknowledgedAt, StartedAt: oa.StartedAt,
		CompletedAt: oa.CompletedAt, VerifiedAt: oa.VerifiedAt,
	}
	for _, attachment := range oa.Attachments {
		v.Attachments = append(v.Attachments, protocol.OpActionAttachmentView{ID: attachment.ID, Kind: attachment.Kind, Filename: attachment.Filename, ContentType: attachment.ContentType, SizeBytes: attachment.SizeBytes, URL: attachment.URL, Label: attachment.Label, CreatedAt: attachment.CreatedAt})
	}
	for _, note := range oa.ProgressNotes {
		v.ProgressNotes = append(v.ProgressNotes, protocol.OpActionNoteView{Text: note.Text, CreatedAt: note.CreatedAt})
	}
	for _, comment := range oa.Comments {
		v.Comments = append(v.Comments, protocol.OpActionCommentView{Author: comment.Author, Text: comment.Text, CreatedAt: comment.CreatedAt})
	}
	return v
}

func actionsToView(in []domain.OperatorAction) []protocol.OpActionView {
	out := make([]protocol.OpActionView, len(in))
	for i := range in {
		out[i] = actionToView(in[i])
	}
	return out
}

func protoErr(code int, msg string) error {
	return &protocol.ProtocolError{Code: code, Message: msg}
}

func internalErr(action string, err error) error {
	return &protocol.ProtocolError{Code: protocol.CodeInternalError, Message: fmt.Sprintf("%s: %v", action, err)}
}
