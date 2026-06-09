package rpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/internal/caller"
	notificationapp "github.com/tinoosan/agen8/internal/services/notification/app"
	"github.com/tinoosan/agen8/internal/services/notification/domain"
)

// JSON-RPC error codes mirrored from the transport layer so this package stays
// a leaf relative to internal/rpc (which reads RPCCode() when serializing).
const (
	codeInvalidParams  = -32602
	codeInvalidRequest = -32600
	codeInternalError  = -32603
)

type rpcError struct {
	code    int
	message string
}

func (e *rpcError) Error() string { return e.message }
func (e *rpcError) RPCCode() int  { return e.code }

func invalidParams(message string) *rpcError {
	return &rpcError{code: codeInvalidParams, message: message}
}
func invalidRequest(message string) *rpcError {
	return &rpcError{code: codeInvalidRequest, message: message}
}
func internalError(format string, args ...any) *rpcError {
	return &rpcError{code: codeInternalError, message: fmt.Sprintf(format, args...)}
}

// Handler adapts the notification application service to RPC protocol types.
type Handler struct {
	svc      *notificationapp.Service
	resolver caller.Resolver
}

// NewHandler wraps the notification service. The caller is resolved from
// context (the internal/rpc identity wrapper injects it), giving us the user id
// that scopes every inbox operation.
func NewHandler(svc *notificationapp.Service) *Handler {
	return &Handler{svc: svc, resolver: caller.ContextResolver{}}
}

func (h *Handler) userID(ctx context.Context) (string, error) {
	c, err := h.resolver.ResolveCaller(ctx)
	if err != nil {
		return "", invalidRequest("notification: caller identity is required")
	}
	userID := strings.TrimSpace(c.UserID)
	if userID == "" {
		return "", invalidRequest("notification: caller must be a user")
	}
	return userID, nil
}

// List handles notification.list — derives, reconciles, and returns the inbox.
func (h *Handler) List(ctx context.Context, p NotificationListParams) (NotificationListResult, error) {
	userID, err := h.userID(ctx)
	if err != nil {
		return NotificationListResult{}, err
	}
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return NotificationListResult{}, invalidParams("projectId is required")
	}

	res, err := h.svc.SyncAndList(ctx, userID, projectID)
	if err != nil {
		return NotificationListResult{}, internalError("list notifications: %v", err)
	}

	views := make([]NotificationView, len(res.Notifications))
	for i, n := range res.Notifications {
		views[i] = notificationToView(n)
	}
	return NotificationListResult{Notifications: views, UnreadCount: res.UnreadCount}, nil
}

// MarkRead handles notification.markRead.
func (h *Handler) MarkRead(ctx context.Context, p NotificationMarkReadParams) (NotificationMutationResult, error) {
	userID, err := h.userID(ctx)
	if err != nil {
		return NotificationMutationResult{}, err
	}
	if strings.TrimSpace(p.ID) == "" {
		return NotificationMutationResult{}, invalidParams("id is required")
	}
	if err := h.svc.MarkRead(ctx, userID, p.ID); err != nil {
		return NotificationMutationResult{}, internalError("mark read: %v", err)
	}
	return NotificationMutationResult{OK: true}, nil
}

// MarkAllRead handles notification.markAllRead.
func (h *Handler) MarkAllRead(ctx context.Context, p NotificationMarkAllReadParams) (NotificationMutationResult, error) {
	userID, err := h.userID(ctx)
	if err != nil {
		return NotificationMutationResult{}, err
	}
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return NotificationMutationResult{}, invalidParams("projectId is required")
	}
	count, err := h.svc.MarkAllRead(ctx, userID, projectID)
	if err != nil {
		return NotificationMutationResult{}, internalError("mark all read: %v", err)
	}
	return NotificationMutationResult{OK: true, Count: count}, nil
}

// Dismiss handles notification.dismiss.
func (h *Handler) Dismiss(ctx context.Context, p NotificationDismissParams) (NotificationMutationResult, error) {
	userID, err := h.userID(ctx)
	if err != nil {
		return NotificationMutationResult{}, err
	}
	if strings.TrimSpace(p.ID) == "" {
		return NotificationMutationResult{}, invalidParams("id is required")
	}
	if err := h.svc.Dismiss(ctx, userID, p.ID); err != nil {
		return NotificationMutationResult{}, internalError("dismiss: %v", err)
	}
	return NotificationMutationResult{OK: true}, nil
}

func notificationToView(n domain.Notification) NotificationView {
	view := NotificationView{
		ID:          n.ID,
		UserID:      n.UserID,
		ProjectID:   n.ProjectID,
		Source:      n.Source,
		Trigger:     n.Trigger,
		Severity:    string(n.Severity),
		Title:       n.Title,
		Body:        n.Body,
		ThrottleKey: n.ThrottleKey,
		Metadata:    n.Metadata,
		CreatedAt:   n.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if strings.TrimSpace(n.LinkSurface) != "" || strings.TrimSpace(n.LinkURL) != "" {
		view.Link = &NotificationLinkView{Surface: n.LinkSurface, URL: n.LinkURL}
	}
	if n.ReadAt != nil {
		view.ReadAt = n.ReadAt.UTC().Format(time.RFC3339Nano)
	}
	if n.DismissedAt != nil {
		view.DismissedAt = n.DismissedAt.UTC().Format(time.RFC3339Nano)
	}
	return view
}
