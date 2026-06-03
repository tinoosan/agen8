package domain

import "context"

// NotificationChannel is the OCP extension point for delivery mechanisms.
// Each delivery mechanism (in-app, webhook, email, Slack, PagerDuty)
// implements this interface. Adding a new channel requires no changes to
// the dispatch logic.
type NotificationChannel interface {
	// Type returns the channel identifier (e.g. "in_app", "webhook").
	Type() string

	// Send delivers a notification via this channel.
	Send(ctx context.Context, notification Notification) error
}
