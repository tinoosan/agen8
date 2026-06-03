package infra

import (
	"context"

	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
)

// InAppChannel persists notifications for the in-app notification bell.
// Broadcasting to connected clients is handled by Service.raise() —
// the in-app channel only needs to persist to avoid double-broadcasting.
type InAppChannel struct {
	store domain.NotificationRepository
}

var _ domain.NotificationChannel = (*InAppChannel)(nil)

// NewInAppChannel creates a new in-app notification channel.
func NewInAppChannel(store domain.NotificationRepository) *InAppChannel {
	return &InAppChannel{store: store}
}

// Type returns the channel identifier.
func (c *InAppChannel) Type() string { return "in_app" }

// Send persists the notification. Broadcasting is handled at the service level
// to avoid duplicate pushes when multiple channels are active.
func (c *InAppChannel) Send(ctx context.Context, n domain.Notification) error {
	// The notification is already saved by Service.raise() before
	// channel dispatch, so in-app channel is a no-op. This keeps the channel
	// interface consistent while avoiding double-writes.
	return nil
}
