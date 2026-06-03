// Package rpc adapts the notification application service to RPC protocol types.
package rpc

import (
	"context"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

// Handler adapts the notification application service to protocol RPC methods.
type Handler struct {
	svc *app.Service
}

// NewHandler creates an RPC handler wrapping the notification service.
func NewHandler(svc *app.Service) *Handler {
	return &Handler{svc: svc}
}

// List handles notifications.list RPC calls.
func (h *Handler) List(ctx context.Context, p protocol.NotificationsListParams) (protocol.NotificationsListResult, error) {
	if p.UserID == "" {
		return protocol.NotificationsListResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "userId is required",
		}
	}

	filter := domain.NotificationFilter{
		Source:    p.Source,
		ProjectID: p.ProjectID,
		Severity:  domain.Severity(p.Severity),
		Unread:    p.Unread,
		Limit:     p.Limit,
		Offset:    p.Offset,
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}

	notifications, err := h.svc.List(ctx, p.UserID, filter)
	if err != nil {
		return protocol.NotificationsListResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: err.Error(),
		}
	}

	items := make([]protocol.NotificationItem, len(notifications))
	for i, n := range notifications {
		items[i] = toProtocolNotification(n)
	}

	return protocol.NotificationsListResult{Notifications: items}, nil
}

// UnreadCount handles notifications.unreadCount RPC calls.
func (h *Handler) UnreadCount(ctx context.Context, p protocol.NotificationsUnreadCountParams) (protocol.NotificationsUnreadCountResult, error) {
	if p.UserID == "" {
		return protocol.NotificationsUnreadCountResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "userId is required",
		}
	}

	count, err := h.svc.UnreadCount(ctx, p.UserID)
	if err != nil {
		return protocol.NotificationsUnreadCountResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: err.Error(),
		}
	}

	return protocol.NotificationsUnreadCountResult{Count: count}, nil
}

// MarkRead handles notifications.markRead RPC calls.
func (h *Handler) MarkRead(ctx context.Context, p protocol.NotificationsMarkReadParams) (protocol.EmptyResult, error) {
	if p.ID == "" {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "id is required",
		}
	}

	if err := h.svc.MarkRead(ctx, p.ID); err != nil {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: err.Error(),
		}
	}
	return protocol.EmptyResult{}, nil
}

// MarkAllRead handles notifications.markAllRead RPC calls.
func (h *Handler) MarkAllRead(ctx context.Context, p protocol.NotificationsMarkAllReadParams) (protocol.EmptyResult, error) {
	if p.UserID == "" {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "userId is required",
		}
	}

	if err := h.svc.MarkAllRead(ctx, p.UserID); err != nil {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: err.Error(),
		}
	}
	return protocol.EmptyResult{}, nil
}

// Dismiss handles notifications.dismiss RPC calls.
func (h *Handler) Dismiss(ctx context.Context, p protocol.NotificationsDismissParams) (protocol.EmptyResult, error) {
	if p.ID == "" {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "id is required",
		}
	}

	if err := h.svc.Dismiss(ctx, p.ID); err != nil {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: err.Error(),
		}
	}
	return protocol.EmptyResult{}, nil
}

// RulesList handles notifications.rules.list RPC calls.
func (h *Handler) RulesList(ctx context.Context, p protocol.NotificationsRulesListParams) (protocol.NotificationsRulesListResult, error) {
	if p.UserID == "" {
		return protocol.NotificationsRulesListResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "userId is required",
		}
	}

	rules, err := h.svc.ListRules(ctx, p.UserID)
	if err != nil {
		return protocol.NotificationsRulesListResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: err.Error(),
		}
	}

	items := make([]protocol.NotificationRuleItem, len(rules))
	for i, r := range rules {
		items[i] = toProtocolRule(r)
	}

	return protocol.NotificationsRulesListResult{Rules: items}, nil
}

// RulesSave handles notifications.rules.save RPC calls.
func (h *Handler) RulesSave(ctx context.Context, p protocol.NotificationsRulesSaveParams) (protocol.EmptyResult, error) {
	rule := fromProtocolRule(p.Rule)
	if rule.UserID == "" {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "rule.userId is required",
		}
	}

	if err := h.svc.SaveRule(ctx, rule); err != nil {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: err.Error(),
		}
	}
	return protocol.EmptyResult{}, nil
}

// RulesDelete handles notifications.rules.delete RPC calls.
func (h *Handler) RulesDelete(ctx context.Context, p protocol.NotificationsRulesDeleteParams) (protocol.EmptyResult, error) {
	if p.ID == "" {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInvalidParams, Message: "id is required",
		}
	}

	if err := h.svc.DeleteRule(ctx, p.ID); err != nil {
		return protocol.EmptyResult{}, &protocol.ProtocolError{
			Code: protocol.CodeInternalError, Message: err.Error(),
		}
	}
	return protocol.EmptyResult{}, nil
}

// SourcesList handles notifications.sources.list RPC calls — returns registered
// evaluator sources and channel types for the preferences UI.
func (h *Handler) SourcesList(_ context.Context, _ struct{}) (protocol.NotificationsSourcesListResult, error) {
	return protocol.NotificationsSourcesListResult{
		Sources:  h.svc.RegisteredSources(),
		Channels: h.svc.RegisteredChannelTypes(),
	}, nil
}

// ── Type mapping ────────────────────────────────────────────────────────

func toProtocolNotification(n domain.Notification) protocol.NotificationItem {
	item := protocol.NotificationItem{
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
		CreatedAt:   n.CreatedAt.Format(time.RFC3339),
	}
	if !n.Subject.IsZero() {
		item.Subject = &protocol.NotificationSubjectItem{
			Kind: n.Subject.Kind,
			ID:   n.Subject.ID,
		}
	}
	if n.Link != nil {
		item.Link = &protocol.NotificationLinkItem{
			Surface: n.Link.Surface,
			URL:     n.Link.URL,
		}
	}
	if n.ReadAt != nil {
		s := n.ReadAt.Format(time.RFC3339)
		item.ReadAt = &s
	}
	if n.DismissedAt != nil {
		s := n.DismissedAt.Format(time.RFC3339)
		item.DismissedAt = &s
	}
	return item
}

func toProtocolRule(r domain.NotificationRule) protocol.NotificationRuleItem {
	return protocol.NotificationRuleItem{
		ID:              r.ID,
		UserID:          r.UserID,
		Source:          r.Source,
		Trigger:         r.Trigger,
		MinSeverity:     string(r.MinSeverity),
		Channels:        r.Channels,
		CooldownMinutes: r.CooldownMinutes,
		Enabled:         r.Enabled,
		WebhookURL:      r.WebhookURL,
	}
}

func fromProtocolRule(r protocol.NotificationRuleItem) domain.NotificationRule {
	return domain.NotificationRule{
		ID:              r.ID,
		UserID:          r.UserID,
		Source:          r.Source,
		Trigger:         r.Trigger,
		MinSeverity:     domain.Severity(r.MinSeverity),
		Channels:        r.Channels,
		CooldownMinutes: r.CooldownMinutes,
		Enabled:         r.Enabled,
		WebhookURL:      r.WebhookURL,
	}
}
