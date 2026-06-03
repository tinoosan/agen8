package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func TestBus_PublishAndHandle(t *testing.T) {
	bus := New(nil)
	var received atomic.Int32

	bus.AddHandler("test-handler", TopicMessagePublished, func(msg *message.Message) error {
		var event MessagePublishedEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		if event.Source != "test" {
			t.Errorf("source=%q want test", event.Source)
		}
		received.Add(1)
		msg.Ack()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- bus.Run(ctx) }()

	select {
	case <-bus.Running():
	case err := <-errCh:
		t.Fatalf("Run: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for bus to start")
	}

	if err := bus.Publish(TopicMessagePublished, MessagePublishedEvent{
		Source:      "test",
		PublishedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Wait for delivery.
	deadline := time.After(2 * time.Second)
	for received.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for handler")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	if n := received.Load(); n != 1 {
		t.Fatalf("received=%d want 1", n)
	}
	cancel()
}

func TestBus_PublishSpaceMemberLifecycleEvent(t *testing.T) {
	bus := New(nil)
	received := make(chan SpaceMemberLifecycleEvent, 1)

	bus.AddHandler("space-member-handler", TopicSpaceMemberLifecycle, func(msg *message.Message) error {
		var event SpaceMemberLifecycleEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		received <- event
		msg.Ack()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- bus.Run(ctx) }()
	select {
	case <-bus.Running():
	case err := <-errCh:
		t.Fatalf("Run: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for bus to start")
	}

	publishedAt := time.Now().UTC()
	if err := bus.Publish(TopicSpaceMemberLifecycle, SpaceMemberLifecycleEvent{
		SpaceID:        "space-1",
		MemberID:       "member-1",
		MemberType:     "worker",
		EventType:      SpaceMemberEventRegistered,
		LifecycleState: "active",
		HarnessKind:    "codex",
		Model:          "gpt-5.5",
		Effort:         "high",
		Timestamp:      publishedAt,
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case got := <-received:
		if got.EventType != SpaceMemberEventRegistered || got.MemberID != "member-1" || got.HarnessKind != "codex" {
			t.Fatalf("event=%+v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for member lifecycle event")
	}
	cancel()
}

func TestDeliveryRegistry_Route(t *testing.T) {
	reg := NewDeliveryRegistry()
	ch := make(chan types.Message, 8)
	cancel := reg.Register("space-1", "run-1", "lead", ch)
	defer cancel()

	now := time.Now().UTC()
	msg := types.Message{
		MessageID:          "m1",
		DestinationSpaceID: "space-1",
		AssignedRole:       "lead",
		Body:               "hello",
		CreatedAt:          &now,
	}
	delivered := reg.Route(msg)
	if delivered != 1 {
		t.Fatalf("delivered=%d want 1", delivered)
	}
	select {
	case got := <-ch:
		if got.Body != "hello" {
			t.Fatalf("body=%q want hello", got.Body)
		}
	default:
		t.Fatal("no message received")
	}

	// Unregister and verify no delivery.
	cancel()
	delivered = reg.Route(msg)
	if delivered != 0 {
		t.Fatalf("after cancel: delivered=%d want 0", delivered)
	}
}

func TestDeliveryRegistry_SpaceScopedRoutesToCoordinator(t *testing.T) {
	reg := NewDeliveryRegistry()
	coordCh := make(chan types.Message, 8)
	workerCh := make(chan types.Message, 8)
	cancelCoord := reg.Register("space-1", "run-c", "", coordCh)
	defer cancelCoord()
	cancelWorker := reg.Register("space-1", "run-w", "worker", workerCh)
	defer cancelWorker()

	now := time.Now().UTC()
	msg := types.Message{
		MessageID:          "m2",
		DestinationSpaceID: "space-1",
		AssignedRole:       "",
		Body:               "space-level",
		CreatedAt:          &now,
	}
	delivered := reg.Route(msg)
	if delivered != 1 {
		t.Fatalf("delivered=%d want 1", delivered)
	}
	select {
	case <-coordCh:
		// ok
	default:
		t.Fatal("coordinator should have received the message")
	}
	select {
	case <-workerCh:
		t.Fatal("worker should NOT have received the space-scoped message")
	default:
		// ok
	}
}

func TestDeliveryRegistry_BuffersUntilSubscriberRegisters(t *testing.T) {
	reg := NewDeliveryRegistry()
	now := time.Now().UTC()
	msg := types.Message{
		MessageID:          "m-pending",
		DestinationSpaceID: "space-1",
		AssignedRole:       "lead",
		Body:               "hello later",
		CreatedAt:          &now,
	}
	if delivered := reg.Route(msg); delivered != 0 {
		t.Fatalf("delivered before register=%d want 0", delivered)
	}

	ch := make(chan types.Message, 8)
	cancel := reg.Register("space-1", "run-1", "lead", ch)
	defer cancel()

	select {
	case got := <-ch:
		if got.MessageID != "m-pending" || got.Body != "hello later" {
			t.Fatalf("pending message=%+v", got)
		}
	default:
		t.Fatal("pending message was not delivered on register")
	}
}

func TestDeliveryHandler_CorruptPayloadNacks(t *testing.T) {
	reg := NewDeliveryRegistry()
	handler := NewDeliveryHandler(reg)
	msg := message.NewMessage("bad", []byte("not-json"))

	err := handler(msg)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
	select {
	case <-msg.Nacked():
	default:
		t.Fatal("expected message to be nacked")
	}
	select {
	case <-msg.Acked():
		t.Fatal("did not expect ack on corrupt payload")
	default:
	}
}

func TestPersistHandler_CorruptPayloadNacks(t *testing.T) {
	handler := PersistHandler(func(_ context.Context, _ types.MemberMessage) error { return nil })
	msg := message.NewMessage("bad", []byte("{"))

	err := handler(msg)
	if err == nil {
		t.Fatal("expected unmarshal error")
	}
	select {
	case <-msg.Nacked():
	default:
		t.Fatal("expected message to be nacked")
	}
}

func TestPersistHandler_SkipsLiveOnlyMessages(t *testing.T) {
	calls := 0
	handler := PersistHandler(func(_ context.Context, _ types.MemberMessage) error {
		calls++
		return nil
	})
	payload, err := json.Marshal(MessagePublishedEvent{
		Envelope: types.MemberMessage{
			MessageID: "msg-live",
			IntentID:  "turn.create:live",
			Metadata:  map[string]any{"deliveryMode": DeliveryModeLive},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	msg := message.NewMessage("live", payload)

	if err := handler(msg); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if calls != 0 {
		t.Fatalf("persist calls=%d want 0", calls)
	}
	select {
	case <-msg.Acked():
	default:
		t.Fatal("expected live-only message to be acked")
	}
}

func TestPersistHandler_PersistFailureReturnsErrorAndNacks(t *testing.T) {
	wantErr := errors.New("persist failed")
	handler := PersistHandler(func(_ context.Context, _ types.MemberMessage) error { return wantErr })
	payload, err := json.Marshal(MessagePublishedEvent{
		Envelope: types.MemberMessage{MessageID: "msg-1", IntentID: "test:msg-1"},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := message.NewMessage("persist", payload)

	err = handler(msg)
	if !errors.Is(err, wantErr) {
		t.Fatalf("handler error = %v want %v", err, wantErr)
	}
	select {
	case <-msg.Nacked():
	default:
		t.Fatal("expected message to be nacked")
	}
}

func TestPersistHandler_MissingIntentIDAcksWithError(t *testing.T) {
	called := false
	handler := PersistHandler(func(_ context.Context, _ types.MemberMessage) error {
		called = true
		return nil
	})
	payload, err := json.Marshal(MessagePublishedEvent{
		Envelope: types.MemberMessage{MessageID: "msg-no-intent"},
		Source:   "test.source",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	msg := message.NewMessage("no-intent", payload)

	err = handler(msg)
	if err == nil {
		t.Fatal("expected error for missing IntentID")
	}
	if !strings.Contains(err.Error(), "missing required IntentID") {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Fatal("persist func should not have been called for message with missing IntentID")
	}
	select {
	case <-msg.Acked():
	default:
		t.Fatal("expected message to be acked (not nacked) to prevent infinite retry")
	}
}
