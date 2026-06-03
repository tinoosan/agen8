package protocol

type EmptyResult struct{}

type NotificationsListParams struct {
	UserID    string `json:"userId"`
	Source    string `json:"source,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	Severity  string `json:"severity,omitempty"`
	Unread    *bool  `json:"unread,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type NotificationsListResult struct {
	Notifications []NotificationItem `json:"notifications"`
}

type NotificationsUnreadCountParams struct {
	UserID string `json:"userId"`
}

type NotificationsUnreadCountResult struct {
	Count int `json:"count"`
}

type NotificationsMarkReadParams struct {
	ID string `json:"id"`
}

type NotificationsMarkAllReadParams struct {
	UserID string `json:"userId"`
}

type NotificationsDismissParams struct {
	ID string `json:"id"`
}

type NotificationsRulesListParams struct {
	UserID string `json:"userId"`
}

type NotificationsRulesListResult struct {
	Rules []NotificationRuleItem `json:"rules"`
}

type NotificationsRulesSaveParams struct {
	Rule NotificationRuleItem `json:"rule"`
}

type NotificationsRulesDeleteParams struct {
	ID string `json:"id"`
}

type NotificationsSourcesListResult struct {
	Sources  []string `json:"sources"`
	Channels []string `json:"channels"`
}

type NotificationItem struct {
	ID          string                   `json:"id"`
	UserID      string                   `json:"userId"`
	ProjectID   string                   `json:"projectId,omitempty"`
	Source      string                   `json:"source"`
	Trigger     string                   `json:"trigger"`
	Severity    string                   `json:"severity"`
	Subject     *NotificationSubjectItem `json:"subject,omitempty"`
	Title       string                   `json:"title"`
	Body        string                   `json:"body"`
	Link        *NotificationLinkItem    `json:"link,omitempty"`
	ThrottleKey string                   `json:"throttleKey,omitempty"`
	Metadata    map[string]string        `json:"metadata,omitempty"`
	CreatedAt   string                   `json:"createdAt"`
	ReadAt      *string                  `json:"readAt,omitempty"`
	DismissedAt *string                  `json:"dismissedAt,omitempty"`
}

type NotificationSubjectItem struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type NotificationLinkItem struct {
	Surface string `json:"surface"`
	URL     string `json:"url"`
}

type NotificationRuleItem struct {
	ID              string   `json:"id"`
	UserID          string   `json:"userId"`
	Source          string   `json:"source"`
	Trigger         string   `json:"trigger"`
	MinSeverity     string   `json:"minSeverity"`
	Channels        []string `json:"channels"`
	CooldownMinutes int      `json:"cooldownMinutes"`
	Enabled         bool     `json:"enabled"`
	WebhookURL      string   `json:"webhookUrl,omitempty"`
}
