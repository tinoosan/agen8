package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
)

// decodeNotification unwraps the event.append envelope the hub fans out so tests
// can assert on the inner event the browser consumes.
func decodeNotification(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Method string         `json:"method"`
		Event  map[string]any `json:"event"`
		Params struct {
			Event map[string]any `json:"event"`
		} `json:"params"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("unmarshal notification: %v", err)
	}
	if envelope.Method != eventAppendMethod {
		t.Fatalf("method = %q, want %q", envelope.Method, eventAppendMethod)
	}
	// The same event must appear both top-level and under params.event so old
	// and new consumers both see it.
	if envelope.Params.Event["type"] != envelope.Event["type"] {
		t.Fatalf("params.event.type %v != event.type %v", envelope.Params.Event["type"], envelope.Event["type"])
	}
	return envelope.Event
}

func TestBuildNotificationsCarryTypeAndProject(t *testing.T) {
	h := newEventsHub(nil, nil)

	t.Run("pin", func(t *testing.T) {
		payload, _ := json.Marshal(eventbus.PinLifecycleEvent{ProjectID: "proj-1", NodeRef: "mission-1", NodeType: "mission", EventType: eventbus.PinEventAdded})
		project, notif, err := h.buildPinNotification(payload)
		if err != nil || project != "proj-1" {
			t.Fatalf("project=%q err=%v", project, err)
		}
		event := decodeNotification(t, notif)
		if event["type"] != "pin.added" || event["nodeRef"] != "mission-1" {
			t.Fatalf("event = %v", event)
		}
	})

	t.Run("task", func(t *testing.T) {
		payload, _ := json.Marshal(eventbus.TaskLifecycleEvent{ProjectID: "proj-1", TaskID: "task-9", EventType: "task.created", Status: "queued"})
		project, notif, err := h.buildTaskNotification(payload)
		if err != nil || project != "proj-1" {
			t.Fatalf("project=%q err=%v", project, err)
		}
		event := decodeNotification(t, notif)
		if event["type"] != "task.created" || event["taskId"] != "task-9" {
			t.Fatalf("event = %v", event)
		}
	})

	t.Run("decision", func(t *testing.T) {
		payload, _ := json.Marshal(eventbus.DecisionLoggedEvent{ProjectID: "proj-1", DecisionID: "dec-3", Title: "pick SSE", MemberName: "Nova"})
		project, notif, err := h.buildDecisionNotification(payload)
		if err != nil || project != "proj-1" {
			t.Fatalf("project=%q err=%v", project, err)
		}
		event := decodeNotification(t, notif)
		if event["type"] != "decision.logged" || event["decisionId"] != "dec-3" {
			t.Fatalf("event = %v", event)
		}
	})

	t.Run("member", func(t *testing.T) {
		payload, _ := json.Marshal(eventbus.SpaceMemberLifecycleEvent{ProjectID: "proj-1", MemberID: "member-1", EventType: eventbus.SpaceMemberEventRegistered, HarnessKind: "claude-cli", Model: "opus"})
		project, notif, err := h.buildMemberNotification(payload)
		if err != nil || project != "proj-1" {
			t.Fatalf("project=%q err=%v", project, err)
		}
		event := decodeNotification(t, notif)
		if event["type"] != "space.member.registered" || event["harnessKind"] != "claude-cli" {
			t.Fatalf("event = %v", event)
		}
	})

	t.Run("mission record carries projectId from Data", func(t *testing.T) {
		payload, _ := json.Marshal(types.EventRecord{
			Type: "mission.activated",
			Data: map[string]string{"projectId": "proj-1", "missionId": "mission-7", "status": "active"},
		})
		project, notif, err := h.buildRecordNotification(payload)
		if err != nil || project != "proj-1" {
			t.Fatalf("project=%q err=%v", project, err)
		}
		event := decodeNotification(t, notif)
		if event["type"] != "mission.activated" || event["missionId"] != "mission-7" {
			t.Fatalf("event = %v", event)
		}
	})

	t.Run("malformed payload errors", func(t *testing.T) {
		if _, _, err := h.buildTaskNotification([]byte("{not json")); err == nil {
			t.Fatal("expected decode error")
		}
	})

	t.Run("member without project drops quietly", func(t *testing.T) {
		payload, _ := json.Marshal(eventbus.SpaceMemberLifecycleEvent{MemberID: "member-1", EventType: eventbus.SpaceMemberEventRegistered})
		project, notif, err := h.buildMemberNotification(payload)
		if err != nil || project != "" || notif != nil {
			t.Fatalf("expected quiet drop, got project=%q notif=%v err=%v", project, notif, err)
		}
	})
}

// TestHubFansOutMultipleTopics drives a real event bus end to end: a decision
// and a mission event published on two different topics both reach a single
// registered project client as event.append notifications.
func TestHubFansOutMultipleTopics(t *testing.T) {
	bus := eventbus.New(nil)
	t.Cleanup(func() { _ = bus.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := newEventsHub(bus, nil)
	go func() { _ = hub.Run(ctx) }()

	select {
	case <-hub.Running():
	case <-time.After(2 * time.Second):
		t.Fatal("hub did not become ready")
	}

	events, unregister := hub.Register("proj-1")
	defer unregister()

	if err := bus.Publish(eventbus.TopicDecisionLogged, eventbus.DecisionLoggedEvent{ProjectID: "proj-1", DecisionID: "dec-1", Title: "x"}); err != nil {
		t.Fatalf("publish decision: %v", err)
	}
	if err := bus.Publish(eventbus.TopicMissionLifecycle, types.EventRecord{Type: "mission.activated", Data: map[string]string{"projectId": "proj-1", "missionId": "mission-1"}}); err != nil {
		t.Fatalf("publish mission: %v", err)
	}

	got := map[string]bool{}
	for len(got) < 2 {
		select {
		case payload := <-events:
			event := decodeNotification(t, payload)
			got[event["type"].(string)] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for events, got %v", got)
		}
	}
	if !got["decision.logged"] || !got["mission.activated"] {
		t.Fatalf("missing events, got %v", got)
	}
}

// TestHubDoesNotLeakAcrossProjects asserts per-project fan-out isolation.
func TestHubDoesNotLeakAcrossProjects(t *testing.T) {
	bus := eventbus.New(nil)
	t.Cleanup(func() { _ = bus.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hub := newEventsHub(bus, nil)
	go func() { _ = hub.Run(ctx) }()
	<-hub.Running()

	otherEvents, unregister := hub.Register("proj-2")
	defer unregister()

	if err := bus.Publish(eventbus.TopicDecisionLogged, eventbus.DecisionLoggedEvent{ProjectID: "proj-1", DecisionID: "dec-1"}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case payload := <-otherEvents:
		t.Fatalf("proj-2 client received a proj-1 event: %s", payload)
	case <-time.After(300 * time.Millisecond):
		// expected: no cross-project delivery
	}
}
