package domain

import "context"

// NotificationFilter controls which notifications are returned by FindByUser.
type NotificationFilter struct {
	Source    string   // filter by source domain (empty = all)
	ProjectID string   // optional project scope (empty = all projects for the user)
	Severity  Severity // minimum severity (empty = all)
	Unread    *bool    // nil = all, true = unread only, false = read only
	Limit     int
	Offset    int
}

// NotificationRepository persists and queries notifications.
type NotificationRepository interface {
	Save(ctx context.Context, notification Notification) error
	FindByUser(ctx context.Context, userID string, filter NotificationFilter) ([]Notification, error)
	MarkRead(ctx context.Context, id string) error
	MarkAllRead(ctx context.Context, userID string) error
	Dismiss(ctx context.Context, id string) error
	UnreadCount(ctx context.Context, userID string) (int, error)

	// LastByThrottleKey returns the most recent notification matching the throttle
	// composite key (userID, source, trigger, throttleKey). Returns nil, nil if
	// no previous notification exists.
	LastByThrottleKey(ctx context.Context, userID, source, trigger, throttleKey string) (*Notification, error)

	// DismissBySubject auto-dismisses every active notification matching the
	// (userID, source, subject) tuple. Used when a domain entity is resolved
	// (OA closed, ask_user answered/cancelled, escalation acknowledged) so the
	// inbox doesn't accumulate stale rows that no longer need attention.
	// Returns the count of rows updated.
	DismissBySubject(ctx context.Context, userID, source string, subject Subject) (int, error)

	// Prune removes notifications according to the retention policy:
	// - Age-based: older than MaxAgeDays
	// - Count-based: keep newest MaxPerUser per user
	// - Dismissed: prune after 7 days regardless of MaxAgeDays
	// Returns the total number of rows deleted.
	Prune(ctx context.Context, policy RetentionPolicy) (int, error)
}

// NotificationRuleRepository persists and queries notification rules.
type NotificationRuleRepository interface {
	FindByUser(ctx context.Context, userID string) ([]NotificationRule, error)
	FindMatching(ctx context.Context, userID, source, trigger string, severity Severity) ([]NotificationRule, error)
	Save(ctx context.Context, rule NotificationRule) error
	Delete(ctx context.Context, id string) error
}
