package lastseen

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func newStoreForTest(t *testing.T) *Store {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	store, err := NewStore(handle)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store
}

func TestStore_GetNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStoreForTest(t)
	_, err := s.Get(ctx, "user-1", "proj-1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on empty store = %v; want ErrNotFound", err)
	}
}

func TestStore_MarkSeenAndGet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStoreForTest(t)
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)

	if err := s.MarkSeen(ctx, "user-1", "proj-1", now); err != nil {
		t.Fatalf("MarkSeen: %v", err)
	}

	got, err := s.Get(ctx, "user-1", "proj-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Equal(now) {
		t.Fatalf("Get = %v; want %v", got, now)
	}
}

func TestStore_MarkSeenUpserts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStoreForTest(t)
	t1 := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(2 * time.Hour)

	if err := s.MarkSeen(ctx, "user-1", "proj-1", t1); err != nil {
		t.Fatalf("MarkSeen t1: %v", err)
	}
	if err := s.MarkSeen(ctx, "user-1", "proj-1", t2); err != nil {
		t.Fatalf("MarkSeen t2: %v", err)
	}

	got, err := s.Get(ctx, "user-1", "proj-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.Equal(t2) {
		t.Fatalf("Get after upsert = %v; want %v", got, t2)
	}
}

func TestStore_IsolatedByUserAndProject(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStoreForTest(t)
	t1 := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Hour)

	if err := s.MarkSeen(ctx, "user-1", "proj-1", t1); err != nil {
		t.Fatalf("MarkSeen user-1/proj-1: %v", err)
	}
	if err := s.MarkSeen(ctx, "user-2", "proj-1", t2); err != nil {
		t.Fatalf("MarkSeen user-2/proj-1: %v", err)
	}

	got1, err := s.Get(ctx, "user-1", "proj-1")
	if err != nil || !got1.Equal(t1) {
		t.Fatalf("Get user-1/proj-1 = %v, %v; want %v, nil", got1, err, t1)
	}
	got2, err := s.Get(ctx, "user-2", "proj-1")
	if err != nil || !got2.Equal(t2) {
		t.Fatalf("Get user-2/proj-1 = %v, %v; want %v, nil", got2, err, t2)
	}
}

func TestStore_RequiresUserIDAndProjectID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s := newStoreForTest(t)
	now := time.Now().UTC()

	if err := s.MarkSeen(ctx, "", "proj-1", now); err == nil {
		t.Fatal("MarkSeen with empty userID: want error, got nil")
	}
	if err := s.MarkSeen(ctx, "user-1", "", now); err == nil {
		t.Fatal("MarkSeen with empty projectID: want error, got nil")
	}
	if _, err := s.Get(ctx, "", "proj-1"); err == nil {
		t.Fatal("Get with empty userID: want error, got nil")
	}
	if _, err := s.Get(ctx, "user-1", ""); err == nil {
		t.Fatal("Get with empty projectID: want error, got nil")
	}
}
