package domain

// NotificationRule is an operator-configured rule that controls which
// notifications are delivered via which channels. Rules follow a simple model:
// source x trigger x minSeverity -> channels. The "*" wildcard matches all.
type NotificationRule struct {
	ID              string   `json:"id"`
	UserID          string   `json:"userId"`
	Source          string   `json:"source"`          // "heartbeat", "task", "*" (all sources)
	Trigger         string   `json:"trigger"`         // "outcome_critical", "circuit_opened", "*"
	MinSeverity     Severity `json:"minSeverity"`     // only fire if severity >= this level
	Channels        []string `json:"channels"`        // ["in_app", "webhook"]
	CooldownMinutes int      `json:"cooldownMinutes"` // default: 30. 0 = no throttling
	Enabled         bool     `json:"enabled"`
	WebhookURL      string   `json:"webhookUrl,omitempty"` // URL for webhook channel delivery (required when "webhook" is in Channels)
}

// Matches returns true if this rule applies to the given notification parameters.
// Wildcard "*" matches any value. Severity must meet the minimum threshold.
func (r NotificationRule) Matches(source, trigger string, severity Severity) bool {
	if !r.Enabled {
		return false
	}
	if r.Source != "*" && r.Source != source {
		return false
	}
	if r.Trigger != "*" && r.Trigger != trigger {
		return false
	}
	return SeverityRank(severity) >= SeverityRank(r.MinSeverity)
}

// DefaultRules returns the default notification rules for a new user.
// These provide sensible defaults that operators can customize later.
func DefaultRules(userID string) []NotificationRule {
	return []NotificationRule{
		{
			UserID:          userID,
			Source:          "heartbeat",
			Trigger:         "outcome_critical",
			MinSeverity:     SeverityCritical,
			Channels:        []string{"in_app"},
			CooldownMinutes: 30,
			Enabled:         true,
		},
		{
			UserID:          userID,
			Source:          "heartbeat",
			Trigger:         "outcome_error",
			MinSeverity:     SeverityCritical,
			Channels:        []string{"in_app"},
			CooldownMinutes: 30,
			Enabled:         true,
		},
		{
			UserID:          userID,
			Source:          "heartbeat",
			Trigger:         "circuit_opened",
			MinSeverity:     SeverityCritical,
			Channels:        []string{"in_app"},
			CooldownMinutes: 30,
			Enabled:         true,
		},
		{
			UserID:          userID,
			Source:          "heartbeat",
			Trigger:         "*",
			MinSeverity:     SeverityWarning,
			Channels:        []string{"in_app"},
			CooldownMinutes: 30,
			Enabled:         true,
		},
		{
			UserID:          userID,
			Source:          "*",
			Trigger:         "*",
			MinSeverity:     SeverityCritical,
			Channels:        []string{"in_app"},
			CooldownMinutes: 30,
			Enabled:         true,
		},
	}
}
