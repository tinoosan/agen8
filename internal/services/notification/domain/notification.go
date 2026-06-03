// Package domain defines the notification bounded context's core types.
// All types are domain-pure — no infrastructure imports.
package domain

import "time"

// Severity represents the urgency level of a notification.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// SeverityRank returns a numeric rank for severity comparison.
// Higher rank = more severe. Used for escalation bypass of throttle cooldowns.
func SeverityRank(s Severity) int {
	switch s {
	case SeverityInfo:
		return 0
	case SeverityWarning:
		return 1
	case SeverityCritical:
		return 2
	default:
		return -1
	}
}

// NotificationLink is a deep-link to the relevant surface for a notification.
type NotificationLink struct {
	Surface string `json:"surface"` // "calendar", "board", "transcript"
	URL     string `json:"url"`
}

// Subject is a typed reference to the domain entity a notification is about.
// (Source, Subject) together enable auto-dismiss on resolve: when an OA is
// resolved, the service can dismiss the matching "OA created" notification
// by Subject{Kind:"operator_action", ID:oaID} without relying on a
// stringly-typed throttle key convention shared by every evaluator.
type Subject struct {
	Kind string `json:"kind"` // "decision", "operator_action", "escalation", "human_input", "heartbeat"
	ID   string `json:"id"`   // domain-native id (decisionId, oaId, toolCallId, ...)
}

// IsZero reports whether the subject is unset. Notifications that aren't
// "about" a specific domain entity may legitimately omit Subject.
func (s Subject) IsZero() bool {
	return s.Kind == "" && s.ID == ""
}

// Notification is the aggregate root — a raised operator notification.
//
// Identity model:
//   - UserID is the routing key — the human operator who sees this
//     notification. In the current local-single-user mode this is "local";
//     in the hosted multi-user model it'll be the authenticated user.
//   - ProjectID is the workspace scope — used for filtering ("show me
//     notifications for project X") and never for routing.
//   - Subject is an optional typed pointer to the underlying domain entity
//     (decision, OA, ask_user toolCallId, ...). Used for auto-dismiss on
//     resolve and for deep-linking.
//
// The legacy `ProfileID` field has been removed — it was a misnomer that
// conflated user identity, project scope, and agent persona configuration.
type Notification struct {
	ID          string            `json:"id"`
	UserID      string            `json:"userId"`              // routing key
	ProjectID   string            `json:"projectId,omitempty"` // workspace scope
	Source      string            `json:"source"`              // "heartbeat", "decision", "operator_action", "human_input", ...
	Trigger     string            `json:"trigger"`             // "outcome_critical", "oa_created", ...
	Severity    Severity          `json:"severity"`
	Subject     Subject           `json:"subject,omitempty"`
	Title       string            `json:"title"`
	Body        string            `json:"body"`
	Link        *NotificationLink `json:"link,omitempty"`
	ThrottleKey string            `json:"throttleKey,omitempty"` // optional dedup grouping
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	ReadAt      *time.Time        `json:"readAt,omitempty"`      // nil = unread
	DismissedAt *time.Time        `json:"dismissedAt,omitempty"` // nil = active
}

// IsUnread returns true if the notification has not been read.
func (n Notification) IsUnread() bool {
	return n.ReadAt == nil
}

// IsDismissed returns true if the notification has been dismissed.
func (n Notification) IsDismissed() bool {
	return n.DismissedAt != nil
}

// RetentionPolicy controls notification pruning.
type RetentionPolicy struct {
	MaxAgeDays int // default: 30 — notifications older than this are pruned
	MaxPerUser int // default: 1000 — hard cap per user; oldest pruned first
}

// DefaultRetentionPolicy returns the system-default retention settings.
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		MaxAgeDays: 30,
		MaxPerUser: 1000,
	}
}
