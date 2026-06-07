package rpc

// Wire contracts for the notification RPC surface. Field names mirror the
// frontend NotificationItem type (web/src/lib/types.ts) so the client consumes
// these without a translation layer.

// NotificationLinkView is the optional deep-link target.
type NotificationLinkView struct {
	Surface string `json:"surface"`
	URL     string `json:"url,omitempty"`
}

// NotificationView is one inbox row.
type NotificationView struct {
	ID          string                `json:"id"`
	UserID      string                `json:"userId"`
	ProjectID   string                `json:"projectId,omitempty"`
	Source      string                `json:"source"`
	Trigger     string                `json:"trigger"`
	Severity    string                `json:"severity"`
	Title       string                `json:"title"`
	Body        string                `json:"body,omitempty"`
	Link        *NotificationLinkView `json:"link,omitempty"`
	ThrottleKey string                `json:"throttleKey,omitempty"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
	CreatedAt   string                `json:"createdAt"`
	ReadAt      string                `json:"readAt,omitempty"`
	DismissedAt string                `json:"dismissedAt,omitempty"`
}

// NotificationListParams scopes the inbox to a project (user comes from identity).
type NotificationListParams struct {
	ProjectID string `json:"projectId"`
}

// NotificationListResult carries the active inbox plus the unread count for the
// badge, in one round-trip.
type NotificationListResult struct {
	Notifications []NotificationView `json:"notifications"`
	UnreadCount   int                `json:"unreadCount"`
}

// NotificationMarkReadParams marks a single notification read.
type NotificationMarkReadParams struct {
	ID string `json:"id"`
}

// NotificationMarkAllReadParams marks a project's inbox read.
type NotificationMarkAllReadParams struct {
	ProjectID string `json:"projectId"`
}

// NotificationDismissParams removes a notification from the active inbox.
type NotificationDismissParams struct {
	ID string `json:"id"`
}

// NotificationMutationResult is the shared shape for write operations.
type NotificationMutationResult struct {
	OK    bool `json:"ok"`
	Count int  `json:"count,omitempty"`
}
