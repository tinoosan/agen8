package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

const DeliveryModeLive = "live"

// DeliveryRegistry tracks live subscriber channels keyed by spaceID+role.
// The supervisor registers/unregisters channels as spaces start/stop.
type DeliveryRegistry struct {
	mu      sync.RWMutex
	subs    map[string]*DeliverySubscription
	pending []types.Message
}

// DeliverySubscription represents a live subscriber channel for a space role.
type DeliverySubscription struct {
	SpaceID string
	RunID   string
	Role    string
	Ch      chan types.Message
}

// NewDeliveryRegistry creates an empty delivery registry.
func NewDeliveryRegistry() *DeliveryRegistry {
	return &DeliveryRegistry{
		subs: make(map[string]*DeliverySubscription),
	}
}

func subKey(spaceID, runID, role string) string {
	return fmt.Sprintf("%s:%s:%s", spaceID, runID, role)
}

// Register adds a subscriber channel. Returns a cancel function to unregister.
func (r *DeliveryRegistry) Register(spaceID, runID, role string, ch chan types.Message) func() {
	spaceID = strings.TrimSpace(spaceID)
	runID = strings.TrimSpace(runID)
	role = strings.TrimSpace(role)
	key := subKey(spaceID, runID, role)
	r.mu.Lock()
	sub := &DeliverySubscription{
		SpaceID: spaceID,
		RunID:   runID,
		Role:    role,
		Ch:      ch,
	}
	r.subs[key] = sub
	if len(r.pending) > 0 {
		remaining := r.pending[:0]
		for _, msg := range r.pending {
			destSpace := strings.TrimSpace(string(msg.DestinationSpaceID))
			destRole := strings.TrimSpace(msg.AssignedRole)
			if !matchSubscription(sub, destSpace, destRole) {
				remaining = append(remaining, msg)
				continue
			}
			select {
			case sub.Ch <- msg:
			default:
				remaining = append(remaining, msg)
				slog.Warn("eventbus: subscriber buffer full, pending message retained",
					"spaceID", sub.SpaceID,
					"role", sub.Role,
					"messageID", msg.MessageID,
				)
			}
		}
		r.pending = remaining
	}
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		delete(r.subs, key)
		r.mu.Unlock()
	}
}

// Route delivers a message to all matching subscribers. Routing rules match
// the original broker: destination space+role for targeted messages,
// coordinator (empty-role subscriber) for space-scoped messages without a role.
func (r *DeliveryRegistry) Route(msg types.Message) int {
	destSpace := strings.TrimSpace(string(msg.DestinationSpaceID))
	destRole := strings.TrimSpace(msg.AssignedRole)

	r.mu.Lock()
	defer r.mu.Unlock()

	delivered := 0
	for _, sub := range r.subs {
		if !matchSubscription(sub, destSpace, destRole) {
			continue
		}
		select {
		case sub.Ch <- msg:
			delivered++
		default:
			slog.Warn("eventbus: subscriber buffer full, message dropped",
				"spaceID", sub.SpaceID,
				"role", sub.Role,
				"messageID", msg.MessageID,
			)
		}
	}
	if delivered == 0 {
		r.pending = append(r.pending, msg)
		const maxPendingLiveMessages = 256
		if overflow := len(r.pending) - maxPendingLiveMessages; overflow > 0 {
			r.pending = append([]types.Message(nil), r.pending[overflow:]...)
			slog.Warn("eventbus: live delivery pending buffer overflow, oldest messages dropped",
				"dropped", overflow,
				"spaceID", destSpace,
				"role", destRole,
			)
		}
	}
	return delivered
}

func matchSubscription(sub *DeliverySubscription, destSpace, destRole string) bool {
	if destSpace != "" && sub.SpaceID != destSpace {
		return false
	}
	if destRole != "" {
		if sub.Role != destRole {
			return false
		}
	} else if destSpace != "" {
		// Space-scoped message without a specific role targets only the
		// coordinator (empty-role subscriber).
		if sub.Role != "" {
			return false
		}
	}
	return true
}

// SubscriberCount returns the number of active subscriptions.
func (r *DeliveryRegistry) SubscriberCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.subs)
}

// NewDeliveryHandler returns a Watermill handler that routes messages to
// live subscriber channels via the DeliveryRegistry.
func NewDeliveryHandler(registry *DeliveryRegistry) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var event MessagePublishedEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			slog.Warn("eventbus.delivery: unmarshal failed", "error", err)
			msg.Nack()
			return fmt.Errorf("eventbus.delivery: unmarshal failed: %w", err)
		}
		delivered := registry.Route(event.Message)
		slog.Debug("eventbus.delivery: routed",
			"messageID", event.Message.MessageID,
			"destSpace", event.Message.DestinationSpaceID,
			"destRole", event.Message.AssignedRole,
			"delivered", delivered,
		)
		msg.Ack()
		return nil
	}
}

// PersistHandler returns a Watermill handler that persists published messages
// to the message store (queue table). The persist func should call
// taskService.PublishMessage or equivalent.
func PersistHandler(persist func(ctx context.Context, msg types.MemberMessage) error) message.NoPublishHandlerFunc {
	return func(msg *message.Message) error {
		var event MessagePublishedEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			slog.Warn("eventbus.persist: unmarshal failed", "error", err)
			msg.Nack()
			return fmt.Errorf("eventbus.persist: unmarshal failed: %w", err)
		}
		// Validate required fields before persisting. Missing fields are structural
		// bugs in the publisher — retrying will never fix them, so Ack to avoid
		// infinite retry loops that hang agents and produce dead letters.
		if event.Envelope.IntentID == "" {
			slog.Error("eventbus.persist: message missing IntentID — acking to prevent infinite retry",
				"messageID", event.Envelope.MessageID, "source", event.Source)
			msg.Ack()
			return fmt.Errorf("eventbus.persist: message %s missing required IntentID (source: %s)", event.Envelope.MessageID, event.Source)
		}
		if isLiveOnlyMessage(event.Envelope) {
			msg.Ack()
			return nil
		}
		if err := persist(msg.Context(), event.Envelope); err != nil {
			slog.Error("eventbus.persist: failed", "error", err, "messageID", event.Envelope.MessageID)
			// Nack so Watermill can retry.
			msg.Nack()
			return fmt.Errorf("eventbus.persist: persist failed for %s: %w", event.Envelope.MessageID, err)
		}
		msg.Ack()
		return nil
	}
}

func isLiveOnlyMessage(envelope types.MemberMessage) bool {
	if len(envelope.Metadata) == 0 {
		return false
	}
	raw, ok := envelope.Metadata["deliveryMode"]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(raw)), DeliveryModeLive)
}
