package app

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/eventbus"
	pindomain "github.com/tinoosan/agen8/internal/services/pin/domain"
)

// fakeRepo is an in-memory pin repository keyed by (projectID, nodeRef).
type fakeRepo struct {
	pins map[string]pindomain.Pin
}

func newFakeRepo() *fakeRepo { return &fakeRepo{pins: map[string]pindomain.Pin{}} }

func key(projectID, nodeRef string) string { return projectID + "\x00" + nodeRef }

func (f *fakeRepo) Save(_ context.Context, p pindomain.Pin) error {
	k := key(p.ProjectID, p.NodeRef)
	if existing, ok := f.pins[k]; ok {
		// Mirror the real store: preserve original CreatedAt on re-pin.
		p.CreatedAt = existing.CreatedAt
	}
	f.pins[k] = p
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, projectID, nodeRef string) error {
	k := key(projectID, nodeRef)
	if _, ok := f.pins[k]; !ok {
		return pindomain.ErrNotFound
	}
	delete(f.pins, k)
	return nil
}

func (f *fakeRepo) List(_ context.Context, projectID string) ([]pindomain.Pin, error) {
	out := []pindomain.Pin{}
	for _, p := range f.pins {
		if p.ProjectID == projectID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeRepo) Exists(_ context.Context, projectID, nodeRef string) (bool, error) {
	_, ok := f.pins[key(projectID, nodeRef)]
	return ok, nil
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

type publishedEvent struct {
	topic string
	event any
}

type fakeEvents struct {
	events []publishedEvent
}

func (f *fakeEvents) Publish(topic string, event any) error {
	f.events = append(f.events, publishedEvent{topic: topic, event: event})
	return nil
}

func newServiceForTest(t *testing.T, repo pindomain.Repository, clock Clock) *Service {
	t.Helper()
	svc, err := NewService(Config{Pins: repo, Clock: clock})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestService_PinStampsCreatedAtFromClock(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 7, 9, 0, 0, 0, time.UTC)
	svc := newServiceForTest(t, newFakeRepo(), fixedClock{t: now})

	pin, err := svc.Pin(ctx, "proj-1", "mission-1", "mission")
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	if !pin.CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", pin.CreatedAt, now)
	}
	if pin.NodeType != "mission" {
		t.Fatalf("NodeType = %q, want mission", pin.NodeType)
	}
}

func TestService_PinRequiresProjectAndNode(t *testing.T) {
	ctx := context.Background()
	svc := newServiceForTest(t, newFakeRepo(), fixedClock{t: time.Now()})

	if _, err := svc.Pin(ctx, "  ", "n", ""); err == nil {
		t.Fatalf("expected error for blank projectId")
	}
	if _, err := svc.Pin(ctx, "p", "  ", ""); err == nil {
		t.Fatalf("expected error for blank nodeRef")
	}
}

func TestService_UnpinMissingReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	svc := newServiceForTest(t, newFakeRepo(), fixedClock{t: time.Now()})
	if err := svc.Unpin(ctx, "p", "missing"); err != pindomain.ErrNotFound {
		t.Fatalf("Unpin missing = %v, want ErrNotFound", err)
	}
}

func TestService_PinPublishesLifecycleEvent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 7, 9, 30, 0, 0, time.UTC)
	events := &fakeEvents{}
	svc, err := NewService(Config{Pins: newFakeRepo(), Clock: fixedClock{t: now}, Events: events})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := svc.Pin(ctx, " proj-1 ", " mission-1 ", " mission "); err != nil {
		t.Fatalf("Pin: %v", err)
	}

	want := []publishedEvent{{
		topic: eventbus.TopicPinLifecycle,
		event: eventbus.PinLifecycleEvent{
			ProjectID: "proj-1",
			NodeRef:   "mission-1",
			NodeType:  "mission",
			EventType: eventbus.PinEventAdded,
			Timestamp: now,
		},
	}}
	if !reflect.DeepEqual(events.events, want) {
		t.Fatalf("events=%#v want %#v", events.events, want)
	}
}

func TestService_UnpinPublishesLifecycleEvent(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 7, 9, 45, 0, 0, time.UTC)
	repo := newFakeRepo()
	repo.pins[key("proj-1", "task-1")] = pindomain.Pin{
		ProjectID: "proj-1",
		NodeRef:   "task-1",
		NodeType:  "task",
		CreatedAt: now,
	}
	events := &fakeEvents{}
	svc, err := NewService(Config{Pins: repo, Clock: fixedClock{t: now}, Events: events})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if err := svc.Unpin(ctx, " proj-1 ", " task-1 "); err != nil {
		t.Fatalf("Unpin: %v", err)
	}

	want := []publishedEvent{{
		topic: eventbus.TopicPinLifecycle,
		event: eventbus.PinLifecycleEvent{
			ProjectID: "proj-1",
			NodeRef:   "task-1",
			EventType: eventbus.PinEventRemoved,
			Timestamp: now,
		},
	}}
	if !reflect.DeepEqual(events.events, want) {
		t.Fatalf("events=%#v want %#v", events.events, want)
	}
}

func TestService_ListReturnsProjectPins(t *testing.T) {
	ctx := context.Background()
	svc := newServiceForTest(t, newFakeRepo(), fixedClock{t: time.Now()})

	if _, err := svc.Pin(ctx, "p", "a", "task"); err != nil {
		t.Fatalf("Pin a: %v", err)
	}
	if _, err := svc.Pin(ctx, "p", "b", "task"); err != nil {
		t.Fatalf("Pin b: %v", err)
	}
	if _, err := svc.Pin(ctx, "other", "c", "task"); err != nil {
		t.Fatalf("Pin c: %v", err)
	}

	pins, err := svc.List(ctx, "p")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pins) != 2 {
		t.Fatalf("List returned %d pins, want 2 (project-scoped)", len(pins))
	}
}

func TestService_ListRequiresProject(t *testing.T) {
	ctx := context.Background()
	svc := newServiceForTest(t, newFakeRepo(), fixedClock{t: time.Now()})
	if _, err := svc.List(ctx, "   "); err == nil {
		t.Fatalf("expected error for blank projectId")
	}
}
