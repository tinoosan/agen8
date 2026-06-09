package app

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/services/notification/domain"
)

// --- fakes ------------------------------------------------------------------

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fakeTaskSource struct{ tasks []domain.TaskSnapshot }

func (f fakeTaskSource) Tasks(_ context.Context, _ string) ([]domain.TaskSnapshot, error) {
	return f.tasks, nil
}

// memRepo is an in-memory domain.Repository for service tests.
type memRepo struct {
	rows map[string]domain.Notification
	seq  int
}

func newMemRepo() *memRepo { return &memRepo{rows: map[string]domain.Notification{}} }

func (m *memRepo) Insert(_ context.Context, n domain.Notification) error {
	m.rows[n.ID] = n
	return nil
}

func (m *memRepo) ListActive(_ context.Context, userID, projectID string) ([]domain.Notification, error) {
	var out []domain.Notification
	for _, n := range m.rows {
		if n.UserID == userID && n.ProjectID == projectID && n.DismissedAt == nil {
			out = append(out, n)
		}
	}
	return out, nil
}

func (m *memRepo) ExistsByThrottleKey(_ context.Context, userID, projectID, key string) (bool, error) {
	for _, n := range m.rows {
		if n.UserID == userID && n.ProjectID == projectID && n.ThrottleKey == key {
			return true, nil
		}
	}
	return false, nil
}

func (m *memRepo) MarkRead(_ context.Context, userID, id string) error {
	if n, ok := m.rows[id]; ok && n.UserID == userID && n.ReadAt == nil {
		t := time.Now().UTC()
		n.ReadAt = &t
		m.rows[id] = n
	}
	return nil
}

func (m *memRepo) MarkAllRead(_ context.Context, userID, projectID string) (int, error) {
	count := 0
	for id, n := range m.rows {
		if n.UserID == userID && n.ProjectID == projectID && n.ReadAt == nil && n.DismissedAt == nil {
			t := time.Now().UTC()
			n.ReadAt = &t
			m.rows[id] = n
			count++
		}
	}
	return count, nil
}

func (m *memRepo) Dismiss(_ context.Context, userID, id string) error {
	if n, ok := m.rows[id]; ok && n.UserID == userID && n.DismissedAt == nil {
		t := time.Now().UTC()
		n.DismissedAt = &t
		m.rows[id] = n
	}
	return nil
}

// --- helpers ----------------------------------------------------------------

func newServiceForTest(t *testing.T, repo domain.Repository, tasks []domain.TaskSnapshot, now time.Time, cfg domain.DeriveConfig) *Service {
	t.Helper()
	svc, err := NewService(repo, fakeTaskSource{tasks: tasks}, fakeClock{now: now}, cfg, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func ptr(t time.Time) *time.Time { return &t }

// --- tests ------------------------------------------------------------------

func TestSyncCreatesAndIsIdempotent(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cfg := domain.DeriveConfig{EventLookback: time.Hour}
	repo := newMemRepo()
	tasks := []domain.TaskSnapshot{
		{ID: "t1", ProjectID: "proj", Title: "Done", Status: "succeeded", CompletedAt: ptr(now.Add(-5 * time.Minute))},
	}
	svc := newServiceForTest(t, repo, tasks, now, cfg)
	ctx := context.Background()

	res, err := svc.SyncAndList(ctx, "user-1", "proj")
	if err != nil {
		t.Fatalf("SyncAndList: %v", err)
	}
	if len(res.Notifications) != 1 || res.UnreadCount != 1 {
		t.Fatalf("first sync = %d notifs / %d unread, want 1/1", len(res.Notifications), res.UnreadCount)
	}

	// Second sync over the same snapshot must not duplicate.
	res2, _ := svc.SyncAndList(ctx, "user-1", "proj")
	if len(res2.Notifications) != 1 {
		t.Fatalf("second sync produced %d notifs, want 1 (idempotent)", len(res2.Notifications))
	}
}

func TestOneTimeEventNotResurrectedAfterDismiss(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cfg := domain.DeriveConfig{EventLookback: time.Hour}
	repo := newMemRepo()
	tasks := []domain.TaskSnapshot{
		{ID: "t1", ProjectID: "proj", Title: "Done", Status: "succeeded", CompletedAt: ptr(now.Add(-5 * time.Minute))},
	}
	svc := newServiceForTest(t, repo, tasks, now, cfg)
	ctx := context.Background()

	res, _ := svc.SyncAndList(ctx, "user-1", "proj")
	id := res.Notifications[0].ID
	if err := svc.Dismiss(ctx, "user-1", id); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	// The task is still succeeded, but the completed event must stay dismissed.
	res2, _ := svc.SyncAndList(ctx, "user-1", "proj")
	if len(res2.Notifications) != 0 {
		t.Fatalf("dismissed one-time event resurrected: %#v", res2.Notifications)
	}
}

func TestStandingNudgeAutoDismissesWhenConditionClears(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cfg := domain.DeriveConfig{BacklogWarnThreshold: 2}
	repo := newMemRepo()
	ctx := context.Background()

	// Backlog above threshold → nudge present.
	highBacklog := []domain.TaskSnapshot{
		{ID: "a", ProjectID: "proj", Status: "pending", CreatedAt: ptr(now)},
		{ID: "b", ProjectID: "proj", Status: "pending", CreatedAt: ptr(now)},
	}
	svc := newServiceForTest(t, repo, highBacklog, now, cfg)
	res, _ := svc.SyncAndList(ctx, "user-1", "proj")
	if len(res.Notifications) != 1 || res.Notifications[0].Trigger != domain.TriggerBacklogHigh {
		t.Fatalf("expected one backlog nudge, got %#v", res.Notifications)
	}

	// Backlog drains below threshold → nudge auto-dismissed.
	svc2 := newServiceForTest(t, repo, []domain.TaskSnapshot{
		{ID: "a", ProjectID: "proj", Status: "pending", CreatedAt: ptr(now)},
	}, now, cfg)
	res2, _ := svc2.SyncAndList(ctx, "user-1", "proj")
	if len(res2.Notifications) != 0 {
		t.Fatalf("backlog nudge should auto-dismiss when condition clears, got %#v", res2.Notifications)
	}
}

func TestMarkAllReadZeroesUnread(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cfg := domain.DeriveConfig{EventLookback: time.Hour}
	repo := newMemRepo()
	tasks := []domain.TaskSnapshot{
		{ID: "t1", ProjectID: "proj", Title: "A", Status: "succeeded", CompletedAt: ptr(now.Add(-5 * time.Minute))},
		{ID: "t2", ProjectID: "proj", Title: "B", Status: "in_review", UpdatedAt: ptr(now.Add(-5 * time.Minute))},
	}
	svc := newServiceForTest(t, repo, tasks, now, cfg)
	ctx := context.Background()

	res, _ := svc.SyncAndList(ctx, "user-1", "proj")
	if res.UnreadCount != 2 {
		t.Fatalf("unread = %d, want 2", res.UnreadCount)
	}
	if _, err := svc.MarkAllRead(ctx, "user-1", "proj"); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	res2, _ := svc.SyncAndList(ctx, "user-1", "proj")
	if res2.UnreadCount != 0 {
		t.Fatalf("unread after MarkAllRead = %d, want 0", res2.UnreadCount)
	}
	if len(res2.Notifications) != 2 {
		t.Fatalf("read notifications should still be listed, got %d", len(res2.Notifications))
	}
}
