package app

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/eventbus"
	"github.com/tinoosan/agen8/internal/services/task/domain"
)

// capturePublisher records every Publish call so tests can assert on the topic
// and payload the task service fans out.
type capturePublisher struct {
	topic string
	event any
	err   error
	calls int
}

func (c *capturePublisher) Publish(topic string, event any) error {
	c.calls++
	c.topic = topic
	c.event = event
	return c.err
}

func newTestService(pub EventPublisher) *Service {
	return &Service{
		clock:  domain.FixedClock{T: time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)},
		events: pub,
		logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestPublishTaskEventMapsActionsToEventTypes(t *testing.T) {
	cases := map[string]string{
		"create":         "task.created",
		"update":         "task.updated",
		"claim":          "task.claimed",
		"assign":         "task.assigned",
		"complete":       "task.submitted",
		"approve_review": "task.completed",
		"retry_review":   "task.retried",
		"fail_review":    "task.failed",
		"block":          "task.blocked",
		"unblock":        "task.unblocked",
		"release":        "task.released",
		"cancel":         "task.canceled",
	}
	for action, wantType := range cases {
		t.Run(action, func(t *testing.T) {
			pub := &capturePublisher{}
			s := newTestService(pub)
			s.publishTaskEvent(action, domain.Task{
				ID:        "task-1",
				ProjectID: "proj-1",
				Status:    domain.TaskStatusPending,
				Title:     "do the thing",
			})
			if pub.calls != 1 {
				t.Fatalf("calls = %d, want 1", pub.calls)
			}
			if pub.topic != eventbus.TopicTaskLifecycle {
				t.Fatalf("topic = %q, want %q", pub.topic, eventbus.TopicTaskLifecycle)
			}
			event, ok := pub.event.(eventbus.TaskLifecycleEvent)
			if !ok {
				t.Fatalf("event type = %T, want TaskLifecycleEvent", pub.event)
			}
			if event.EventType != wantType {
				t.Fatalf("eventType = %q, want %q", event.EventType, wantType)
			}
			if event.ProjectID != "proj-1" || event.TaskID != "task-1" {
				t.Fatalf("event = %+v", event)
			}
			if event.Values["title"] != "do the thing" {
				t.Fatalf("values = %v, want title carried", event.Values)
			}
		})
	}
}

func TestPublishTaskEventNoOpsWhenUnconfigured(t *testing.T) {
	t.Run("nil publisher", func(t *testing.T) {
		s := newTestService(nil)
		// Must not panic with no publisher wired.
		s.publishTaskEvent("create", domain.Task{ID: "t", ProjectID: "p"})
	})

	t.Run("missing project", func(t *testing.T) {
		pub := &capturePublisher{}
		s := newTestService(pub)
		s.publishTaskEvent("create", domain.Task{ID: "t"})
		if pub.calls != 0 {
			t.Fatalf("calls = %d, want 0 for project-less task", pub.calls)
		}
	})

	t.Run("unmapped action", func(t *testing.T) {
		pub := &capturePublisher{}
		s := newTestService(pub)
		s.publishTaskEvent("nonsense", domain.Task{ID: "t", ProjectID: "p"})
		if pub.calls != 0 {
			t.Fatalf("calls = %d, want 0 for unmapped action", pub.calls)
		}
	})
}
