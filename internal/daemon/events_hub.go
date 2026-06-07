package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
)

const eventAppendMethod = "event.append"

type eventsHub struct {
	bus    *eventbus.Bus
	logger *slog.Logger

	mu      sync.RWMutex
	clients map[string]map[*eventClient]struct{}
	ready   chan struct{}
	once    sync.Once
}

type eventClient struct {
	ch chan []byte
}

func newEventsHub(bus *eventbus.Bus, logger *slog.Logger) *eventsHub {
	if logger == nil {
		logger = slog.Default()
	}
	return &eventsHub{
		bus:     bus,
		logger:  logger,
		clients: map[string]map[*eventClient]struct{}{},
		ready:   make(chan struct{}),
	}
}

func (h *eventsHub) Run(ctx context.Context) error {
	if h == nil || h.bus == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	messages, err := h.bus.Subscribe(ctx, eventbus.TopicPinLifecycle)
	if err != nil {
		return fmt.Errorf("subscribe pin lifecycle: %w", err)
	}
	h.once.Do(func() { close(h.ready) })
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-messages:
			if !ok {
				return nil
			}
			if err := h.handlePinLifecycle(msg.Payload); err != nil {
				h.logger.Warn("drop pin lifecycle event", "error", err)
				msg.Nack()
				continue
			}
			msg.Ack()
		}
	}
}

func (h *eventsHub) Running() <-chan struct{} {
	if h == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return h.ready
}

func (h *eventsHub) Register(projectID string) (<-chan []byte, func()) {
	client := &eventClient{ch: make(chan []byte, 32)}
	h.mu.Lock()
	if h.clients[projectID] == nil {
		h.clients[projectID] = map[*eventClient]struct{}{}
	}
	h.clients[projectID][client] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unregister := func() {
		once.Do(func() {
			h.mu.Lock()
			if clients := h.clients[projectID]; clients != nil {
				delete(clients, client)
				if len(clients) == 0 {
					delete(h.clients, projectID)
				}
			}
			h.mu.Unlock()
			close(client.ch)
		})
	}
	return client.ch, unregister
}

func (h *eventsHub) handlePinLifecycle(payload []byte) error {
	var event eventbus.PinLifecycleEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("decode pin lifecycle: %w", err)
	}
	if event.ProjectID == "" {
		return fmt.Errorf("pin lifecycle event missing projectId")
	}
	notification, err := encodeEventAppend(event.ProjectID, map[string]any{
		"type":      event.EventType,
		"eventType": event.EventType,
		"projectId": event.ProjectID,
		"nodeRef":   event.NodeRef,
		"nodeType":  event.NodeType,
		"timestamp": event.Timestamp,
	})
	if err != nil {
		return err
	}
	h.publish(event.ProjectID, notification)
	return nil
}

func (h *eventsHub) publish(projectID string, payload []byte) {
	h.mu.RLock()
	clients := h.clients[projectID]
	snapshot := make([]*eventClient, 0, len(clients))
	for client := range clients {
		snapshot = append(snapshot, client)
	}
	h.mu.RUnlock()

	for _, client := range snapshot {
		select {
		case client.ch <- payload:
		default:
			h.logger.Warn("drop realtime event for slow client", "projectId", projectID)
		}
	}
}

func encodeEventAppend(projectID string, event map[string]any) ([]byte, error) {
	notification := map[string]any{
		"jsonrpc":   "2.0",
		"method":    eventAppendMethod,
		"projectId": projectID,
		"event":     event,
		"params": map[string]any{
			"event": event,
		},
	}
	return json.Marshal(notification)
}
