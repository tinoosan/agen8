package infra

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
	implstore "github.com/tinoosan/agen8-mcp-server/internal/store"
)

func newRepoForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	handle, err := implstore.GetDBHandle(context.Background(), config.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("GetDBHandle: %v", err)
	}
	return NewSQLiteRepository(handle)
}

func sampleNotification(id, throttle string, now time.Time) domain.Notification {
	return domain.Notification{
		ID:          id,
		UserID:      "user-1",
		ProjectID:   "proj-1",
		Source:      domain.Source,
		Trigger:     domain.TriggerTaskCompleted,
		Severity:    domain.SeverityInfo,
		SubjectKind: domain.SubjectTask,
		SubjectID:   "task-x",
		Title:       "Task completed",
		Body:        "A task was approved and marked done.",
		LinkSurface: "task",
		LinkURL:     "/project/proj-1/tasks/task-x",
		ThrottleKey: throttle,
		Metadata:    map[string]string{"k": "v"},
		CreatedAt:   now,
	}
}

func TestInsertAndListRoundTrip(t *testing.T) {
	t.Parallel()
	repo := newRepoForTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	if err := repo.Insert(ctx, sampleNotification("ntf-1", "task.completed:task-x", now)); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := repo.ListActive(ctx, "user-1", "proj-1")
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("active = %d, want 1", len(got))
	}
	n := got[0]
	if n.Title != "Task completed" || n.LinkURL != "/project/proj-1/tasks/task-x" {
		t.Fatalf("round-trip lost fields: %#v", n)
	}
	if n.Metadata["k"] != "v" {
		t.Fatalf("metadata not preserved: %#v", n.Metadata)
	}
	if n.ReadAt != nil || n.DismissedAt != nil {
		t.Fatalf("new notification should be unread and undismissed")
	}
}

func TestExistsByThrottleKey(t *testing.T) {
	t.Parallel()
	repo := newRepoForTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	if err := repo.Insert(ctx, sampleNotification("ntf-1", "task.completed:task-x", now)); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	// Even after dismissal, the throttle key must still register as "seen".
	if err := repo.Dismiss(ctx, "user-1", "ntf-1"); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	exists, err := repo.ExistsByThrottleKey(ctx, "user-1", "proj-1", "task.completed:task-x")
	if err != nil {
		t.Fatalf("ExistsByThrottleKey: %v", err)
	}
	if !exists {
		t.Fatalf("throttle key should persist across dismissal")
	}

	missing, _ := repo.ExistsByThrottleKey(ctx, "user-1", "proj-1", "task.completed:other")
	if missing {
		t.Fatalf("unknown throttle key should not exist")
	}
}

func TestDismissRemovesFromActive(t *testing.T) {
	t.Parallel()
	repo := newRepoForTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	_ = repo.Insert(ctx, sampleNotification("ntf-1", "k1", now))
	_ = repo.Insert(ctx, sampleNotification("ntf-2", "k2", now))
	if err := repo.Dismiss(ctx, "user-1", "ntf-1"); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	got, _ := repo.ListActive(ctx, "user-1", "proj-1")
	if len(got) != 1 || got[0].ID != "ntf-2" {
		t.Fatalf("active after dismiss = %#v", got)
	}
}

func TestMarkReadAndMarkAllRead(t *testing.T) {
	t.Parallel()
	repo := newRepoForTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	_ = repo.Insert(ctx, sampleNotification("ntf-1", "k1", now))
	_ = repo.Insert(ctx, sampleNotification("ntf-2", "k2", now))

	if err := repo.MarkRead(ctx, "user-1", "ntf-1"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	got, _ := repo.ListActive(ctx, "user-1", "proj-1")
	read := 0
	for _, n := range got {
		if n.ReadAt != nil {
			read++
		}
	}
	if read != 1 {
		t.Fatalf("read count after MarkRead = %d, want 1", read)
	}

	count, err := repo.MarkAllRead(ctx, "user-1", "proj-1")
	if err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if count != 1 {
		t.Fatalf("MarkAllRead affected = %d, want 1 (ntf-1 already read)", count)
	}
	got, _ = repo.ListActive(ctx, "user-1", "proj-1")
	for _, n := range got {
		if n.ReadAt == nil {
			t.Fatalf("notification %s still unread after MarkAllRead", n.ID)
		}
	}
}

func TestListScopedByUserAndProject(t *testing.T) {
	t.Parallel()
	repo := newRepoForTest(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)

	mine := sampleNotification("ntf-mine", "k1", now)
	other := sampleNotification("ntf-other", "k2", now)
	other.UserID = "user-2"
	otherProject := sampleNotification("ntf-proj", "k3", now)
	otherProject.ProjectID = "proj-2"
	_ = repo.Insert(ctx, mine)
	_ = repo.Insert(ctx, other)
	_ = repo.Insert(ctx, otherProject)

	got, _ := repo.ListActive(ctx, "user-1", "proj-1")
	if len(got) != 1 || got[0].ID != "ntf-mine" {
		t.Fatalf("scoping failed: %#v", got)
	}
}
