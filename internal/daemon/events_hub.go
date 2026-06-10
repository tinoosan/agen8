package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/ThreeDotsLabs/watermill/message"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/eventbus"
)

const eventAppendMethod = "event.append"

// notificationBuilder decodes a raw bus payload for one topic into the
// project it belongs to and the event.append notification bytes to fan out.
// An empty projectID means "decoded fine but nothing to route" (the message is
// acked and dropped); a non-nil error means the payload was malformed (nacked).
type notificationBuilder func(payload []byte) (projectID string, notification []byte, err error)

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

// topicSubscriptions maps every domain topic the hub fans out to the browser to
// the decoder for that topic's payload. Pin/task/decision/member publish typed
// structs; mission and KR publish a generic types.EventRecord (carrying the
// project id in Data["projectId"]), so they share buildRecordNotification.
//
// Adding a surface to live SSE is now a one-line change here plus a matching
// `event.type` prefix filter on the frontend hook — see
// docs/architecture/realtime-events.html.
func (h *eventsHub) topicSubscriptions() []struct {
	topic   string
	builder notificationBuilder
} {
	return []struct {
		topic   string
		builder notificationBuilder
	}{
		{eventbus.TopicPinLifecycle, h.buildPinNotification},
		{eventbus.TopicTaskLifecycle, h.buildTaskNotification},
		{eventbus.TopicDecisionLogged, h.buildDecisionNotification},
		{eventbus.TopicSpaceMemberLifecycle, h.buildMemberNotification},
		{eventbus.TopicMissionLifecycle, h.buildRecordNotification},
		{eventbus.TopicKRProgress, h.buildRecordNotification},
	}
}

func (h *eventsHub) Run(ctx context.Context) error {
	if h == nil || h.bus == nil {
		<-ctx.Done()
		return ctx.Err()
	}

	var wg sync.WaitGroup
	for _, sub := range h.topicSubscriptions() {
		messages, err := h.bus.Subscribe(ctx, sub.topic)
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", sub.topic, err)
		}
		wg.Add(1)
		go func(topic string, messages <-chan *message.Message, build notificationBuilder) {
			defer wg.Done()
			h.consume(ctx, topic, messages, build)
		}(sub.topic, messages, sub.builder)
	}

	h.once.Do(func() { close(h.ready) })
	<-ctx.Done()
	wg.Wait()
	return ctx.Err()
}

// consume drains one topic's subscription, translating each message into an
// event.append notification and fanning it out to the project's clients.
func (h *eventsHub) consume(ctx context.Context, topic string, messages <-chan *message.Message, build notificationBuilder) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-messages:
			if !ok {
				return
			}
			projectID, notification, err := build(msg.Payload)
			if err != nil {
				// build() failures are deterministic (decode / missing projectId),
				// so retrying the same payload can never succeed. Ack to drop the
				// poison message — Nack would redeliver it forever on the in-memory
				// GoChannel, flooding the log and pinning a CPU.
				h.logger.Warn("drop realtime event", "topic", topic, "error", err)
				msg.Ack()
				continue
			}
			if projectID != "" && notification != nil {
				h.publish(projectID, notification)
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

func (h *eventsHub) buildPinNotification(payload []byte) (string, []byte, error) {
	var event eventbus.PinLifecycleEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return "", nil, fmt.Errorf("decode pin lifecycle: %w", err)
	}
	if event.ProjectID == "" {
		return "", nil, fmt.Errorf("pin lifecycle event missing projectId")
	}
	notification, err := encodeEventAppend(event.ProjectID, map[string]any{
		"type":      event.EventType,
		"eventType": event.EventType,
		"projectId": event.ProjectID,
		"nodeRef":   event.NodeRef,
		"nodeType":  event.NodeType,
		"timestamp": event.Timestamp,
	})
	return event.ProjectID, notification, err
}

func (h *eventsHub) buildTaskNotification(payload []byte) (string, []byte, error) {
	var event eventbus.TaskLifecycleEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return "", nil, fmt.Errorf("decode task lifecycle: %w", err)
	}
	if event.ProjectID == "" {
		return "", nil, fmt.Errorf("task lifecycle event missing projectId")
	}
	notification, err := encodeEventAppend(event.ProjectID, map[string]any{
		"type":      event.EventType,
		"eventType": event.EventType,
		"projectId": event.ProjectID,
		"taskId":    event.TaskID,
		"status":    event.Status,
		"timestamp": event.Timestamp,
	})
	return event.ProjectID, notification, err
}

func (h *eventsHub) buildDecisionNotification(payload []byte) (string, []byte, error) {
	var event eventbus.DecisionLoggedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return "", nil, fmt.Errorf("decode decision logged: %w", err)
	}
	if event.ProjectID == "" {
		return "", nil, fmt.Errorf("decision logged event missing projectId")
	}
	notification, err := encodeEventAppend(event.ProjectID, map[string]any{
		"type":       "decision.logged",
		"eventType":  "decision.logged",
		"projectId":  event.ProjectID,
		"decisionId": event.DecisionID,
		"title":      event.Title,
		"memberName": event.MemberName,
		"confidence": event.Confidence,
		"timestamp":  event.Timestamp,
	})
	return event.ProjectID, notification, err
}

func (h *eventsHub) buildMemberNotification(payload []byte) (string, []byte, error) {
	var event eventbus.SpaceMemberLifecycleEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return "", nil, fmt.Errorf("decode member lifecycle: %w", err)
	}
	if event.ProjectID == "" {
		// Member events are not always project-scoped; drop quietly.
		return "", nil, nil
	}
	notification, err := encodeEventAppend(event.ProjectID, map[string]any{
		"type":           event.EventType,
		"eventType":      event.EventType,
		"projectId":      event.ProjectID,
		"memberId":       event.MemberID,
		"displayName":    event.DisplayName,
		"harnessKind":    event.HarnessKind,
		"model":          event.Model,
		"lifecycleState": event.LifecycleState,
		"timestamp":      event.Timestamp,
	})
	return event.ProjectID, notification, err
}

// buildRecordNotification decodes the generic types.EventRecord used by mission
// and KR events. The project id lives in Data["projectId"]; the remaining Data
// keys (missionId, keyResultId, status, progressPercent, …) are forwarded so
// consumers and the activity feed can render without a follow-up fetch.
func (h *eventsHub) buildRecordNotification(payload []byte) (string, []byte, error) {
	var event types.EventRecord
	if err := json.Unmarshal(payload, &event); err != nil {
		return "", nil, fmt.Errorf("decode event record: %w", err)
	}
	projectID := event.Data["projectId"]
	if projectID == "" {
		return "", nil, fmt.Errorf("event record missing projectId")
	}
	fields := map[string]any{
		"type":      event.Type,
		"eventType": event.Type,
		"projectId": projectID,
		"timestamp": event.Timestamp,
	}
	for key, value := range event.Data {
		if _, exists := fields[key]; !exists {
			fields[key] = value
		}
	}
	notification, err := encodeEventAppend(projectID, fields)
	return projectID, notification, err
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
