package infra

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	pindomain "github.com/tinoosan/agen8/internal/services/pin/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func TestSQLiteRepository_PinLifecycle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLitePinRepoForTest(t)
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	if err := repo.Save(ctx, pindomain.Pin{
		ProjectID: "proj-1",
		NodeRef:   "mission-1",
		NodeType:  "mission",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	exists, err := repo.Exists(ctx, "proj-1", "mission-1")
	if err != nil || !exists {
		t.Fatalf("Exists = %v, %v; want true, nil", exists, err)
	}

	pins, err := repo.List(ctx, "proj-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pins) != 1 || pins[0].NodeRef != "mission-1" || pins[0].NodeType != "mission" {
		t.Fatalf("listed pins = %+v", pins)
	}
	if !pins[0].CreatedAt.Equal(now) {
		t.Fatalf("CreatedAt = %v, want %v", pins[0].CreatedAt, now)
	}

	if err := repo.Delete(ctx, "proj-1", "mission-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	exists, err = repo.Exists(ctx, "proj-1", "mission-1")
	if err != nil || exists {
		t.Fatalf("Exists after delete = %v, %v; want false, nil", exists, err)
	}
}

func TestSQLiteRepository_RePinPreservesCreatedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLitePinRepoForTest(t)
	first := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	later := first.Add(time.Hour)

	if err := repo.Save(ctx, pindomain.Pin{ProjectID: "p", NodeRef: "n", NodeType: "task", CreatedAt: first}); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	// Re-pin the same node with a newer timestamp and changed type. The
	// composite key collides, so created_at must stay at the original while
	// node_type refreshes.
	if err := repo.Save(ctx, pindomain.Pin{ProjectID: "p", NodeRef: "n", NodeType: "decision", CreatedAt: later}); err != nil {
		t.Fatalf("Save re-pin: %v", err)
	}

	pins, err := repo.List(ctx, "p")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(pins) != 1 {
		t.Fatalf("re-pin created a duplicate: %+v", pins)
	}
	if !pins[0].CreatedAt.Equal(first) {
		t.Fatalf("CreatedAt = %v, want preserved %v", pins[0].CreatedAt, first)
	}
	if pins[0].NodeType != "decision" {
		t.Fatalf("NodeType = %q, want refreshed \"decision\"", pins[0].NodeType)
	}
}

func TestSQLiteRepository_PinsAreProjectScoped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLitePinRepoForTest(t)
	now := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

	// Same node ref pinned in two projects must not collide - the project_id
	// half of the key keeps them distinct.
	if err := repo.Save(ctx, pindomain.Pin{ProjectID: "p1", NodeRef: "shared", CreatedAt: now}); err != nil {
		t.Fatalf("Save p1: %v", err)
	}
	if err := repo.Save(ctx, pindomain.Pin{ProjectID: "p2", NodeRef: "shared", CreatedAt: now}); err != nil {
		t.Fatalf("Save p2: %v", err)
	}

	p1, _ := repo.List(ctx, "p1")
	p2, _ := repo.List(ctx, "p2")
	if len(p1) != 1 || len(p2) != 1 {
		t.Fatalf("project scoping leaked: p1=%+v p2=%+v", p1, p2)
	}
}

func TestSQLiteRepository_DeleteMissingReturnsNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLitePinRepoForTest(t)
	if err := repo.Delete(ctx, "p", "missing"); !errors.Is(err, pindomain.ErrNotFound) {
		t.Fatalf("Delete missing error = %v, want ErrNotFound", err)
	}
}

func TestSQLiteRepository_ValidationRejectsBlankKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := newSQLitePinRepoForTest(t)
	if err := repo.Save(ctx, pindomain.Pin{ProjectID: "", NodeRef: "n"}); err == nil {
		t.Fatalf("expected error for blank projectId")
	}
	if err := repo.Save(ctx, pindomain.Pin{ProjectID: "p", NodeRef: ""}); err == nil {
		t.Fatalf("expected error for blank nodeRef")
	}
}

func TestRepositoryBuilderSelectsSQLiteFromHandle(t *testing.T) {
	t.Parallel()
	repo, err := NewRepository(newSQLiteHandleForTest(t))
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	if _, ok := repo.(*SQLiteRepository); !ok {
		t.Fatalf("NewRepository type = %T, want *SQLiteRepository", repo)
	}
}

func newSQLitePinRepoForTest(t *testing.T) *SQLiteRepository {
	t.Helper()
	repo, err := NewSQLiteRepository(newSQLiteHandleForTest(t))
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	return repo
}

func newSQLiteHandleForTest(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return handle
}
