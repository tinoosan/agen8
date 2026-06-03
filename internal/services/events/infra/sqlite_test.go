package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
	_ "modernc.org/sqlite"
)

func setupTestHandle(t *testing.T) *storagedb.Handle {
	t.Helper()
	ddl := []string{
		`CREATE TABLE IF NOT EXISTS spaces (
			space_id TEXT PRIMARY KEY,
			title TEXT,
			current_goal TEXT,
			space_json TEXT NOT NULL,
			created_at TEXT,
			updated_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS runs (
			run_id TEXT PRIMARY KEY,
			space_id TEXT NOT NULL,
			status TEXT NOT NULL,
			goal TEXT NOT NULL,
			run_json TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT,
			created_at TEXT,
			updated_at TEXT,
			parent_run_id TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			ts TEXT NOT NULL,
			type TEXT NOT NULL,
			message TEXT NOT NULL,
			data_json TEXT,
			event_json TEXT NOT NULL,
			severity TEXT NOT NULL DEFAULT 'info',
			category TEXT NOT NULL DEFAULT 'system',
			origin TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_run_seq ON events(run_id, seq)`,
		`CREATE INDEX IF NOT EXISTS idx_events_run_type ON events(run_id, type)`,
	}
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			for _, stmt := range ddl {
				if _, err := db.ExecContext(ctx, stmt); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return handle
}

func insertTestRun(t *testing.T, db *sql.DB, runID, spaceID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO spaces (space_id, title, space_json) VALUES (?, ?, '{}')`,
		spaceID, "test space",
	)
	if err != nil {
		t.Fatalf("insert space: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO runs (run_id, space_id, status, goal, run_json) VALUES (?, ?, 'running', 'test', '{}')`,
		runID, spaceID,
	)
	if err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func TestSQLiteRepository_AppendAndList(t *testing.T) {
	handle := setupTestHandle(t)
	insertTestRun(t, handle.DB(), "run-1", "space-1")
	repo := NewSQLRepository(handle)

	ctx := context.Background()

	err := repo.Append(ctx, types.EventRecord{
		RunID:     "run-1",
		EventID:   "evt-1",
		Timestamp: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		Type:      "test_event",
		Message:   "hello world",
		Data:      map[string]string{"foo": "bar"},
	})
	if err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	err = repo.Append(ctx, types.EventRecord{
		RunID:     "run-1",
		EventID:   "evt-2",
		Timestamp: time.Date(2026, 3, 1, 0, 0, 1, 0, time.UTC),
		Type:      "second_event",
		Message:   "second message",
	})
	if err != nil {
		t.Fatalf("AppendEvent 2: %v", err)
	}

	events, cursor, err := repo.ListPaginated(ctx, domain.EventFilter{RunID: "run-1", Limit: 100})
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != "test_event" {
		t.Errorf("expected test_event, got %s", events[0].Type)
	}
	if events[1].Type != "second_event" {
		t.Errorf("expected second_event, got %s", events[1].Type)
	}
	if cursor <= 0 {
		t.Errorf("expected positive cursor, got %d", cursor)
	}
}

func TestSQLiteRepository_AppendEvent_NonexistentRun(t *testing.T) {
	handle := setupTestHandle(t)
	repo := NewSQLRepository(handle)

	err := repo.Append(context.Background(), types.EventRecord{
		RunID:     "run-does-not-exist",
		EventID:   "evt-1",
		Timestamp: time.Now(),
		Type:      "test_event",
		Message:   "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestSQLiteRepository_AppendEvent_ValidationErrors(t *testing.T) {
	handle := setupTestHandle(t)
	repo := NewSQLRepository(handle)
	ctx := context.Background()

	tests := []struct {
		name  string
		event types.EventRecord
	}{
		{"empty runID", types.EventRecord{Type: "t", Message: "m"}},
		{"empty type", types.EventRecord{RunID: "run-1", Message: "m"}},
		{"empty message", types.EventRecord{RunID: "run-1", Type: "t"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := repo.Append(ctx, tc.event); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestSQLiteRepository_SpaceEntryUpsertRollback(t *testing.T) {
	handle := setupTestHandle(t)
	insertTestRun(t, handle.DB(), "run-1", "space-1")

	repo := NewSQLRepository(handle, WithSpaceEntryUpsert(func(tx *sql.Tx, dialect storagedb.Dialect, runID string, eventSeq int64, ev types.EventRecord) error {
		return fmt.Errorf("boom")
	}))

	err := repo.Append(context.Background(), types.EventRecord{
		RunID:     "run-1",
		EventID:   "evt-1",
		Timestamp: time.Now(),
		Type:      "test_event",
		Message:   "should roll back",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsStr(err.Error(), "boom") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify event was not persisted.
	events, _, err := repo.ListPaginated(context.Background(), domain.EventFilter{RunID: "run-1", Limit: 100})
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected 0 events after rollback, got %d", len(events))
	}
}

func TestSQLiteRepository_CursorPagination(t *testing.T) {
	handle := setupTestHandle(t)
	insertTestRun(t, handle.DB(), "run-1", "space-1")
	repo := NewSQLRepository(handle)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if err := repo.Append(ctx, types.EventRecord{
			RunID:     "run-1",
			EventID:   types.EventID(fmt.Sprintf("evt-%d", i)),
			Timestamp: time.Now(),
			Type:      "paged",
			Message:   fmt.Sprintf("event %d", i),
		}); err != nil {
			t.Fatalf("AppendEvent %d: %v", i, err)
		}
	}

	b1, cursor, err := repo.ListPaginated(ctx, domain.EventFilter{RunID: "run-1", Limit: 4})
	if err != nil {
		t.Fatalf("batch 1: %v", err)
	}
	if len(b1) != 4 {
		t.Fatalf("expected 4, got %d", len(b1))
	}

	b2, cursor, err := repo.ListPaginated(ctx, domain.EventFilter{RunID: "run-1", Limit: 4, AfterSeq: cursor})
	if err != nil {
		t.Fatalf("batch 2: %v", err)
	}
	if len(b2) != 4 {
		t.Fatalf("expected 4, got %d", len(b2))
	}

	b3, _, err := repo.ListPaginated(ctx, domain.EventFilter{RunID: "run-1", Limit: 4, AfterSeq: cursor})
	if err != nil {
		t.Fatalf("batch 3: %v", err)
	}
	if len(b3) != 2 {
		t.Fatalf("expected 2, got %d", len(b3))
	}

	if total := len(b1) + len(b2) + len(b3); total != 10 {
		t.Fatalf("expected 10 total, got %d", total)
	}
}

func TestSQLiteRepository_TypeFilterAndCount(t *testing.T) {
	handle := setupTestHandle(t)
	insertTestRun(t, handle.DB(), "run-1", "space-1")
	repo := NewSQLRepository(handle)
	ctx := context.Background()

	for i, typ := range []string{"info", "error", "info", "warning"} {
		repo.Append(ctx, types.EventRecord{
			RunID:     "run-1",
			EventID:   types.EventID(fmt.Sprintf("evt-%d", i)),
			Timestamp: time.Now(),
			Type:      typ,
			Message:   fmt.Sprintf("%s %d", typ, i),
		})
	}

	evs, _, err := repo.ListPaginated(ctx, domain.EventFilter{RunID: "run-1", Types: []string{"info"}, Limit: 100})
	if err != nil {
		t.Fatalf("ListPaginated: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("expected 2 info events, got %d", len(evs))
	}

	count, err := repo.Count(ctx, domain.EventFilter{RunID: "run-1"})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 4 {
		t.Fatalf("expected 4, got %d", count)
	}
}

func TestSQLiteRepository_LatestSeq(t *testing.T) {
	handle := setupTestHandle(t)
	insertTestRun(t, handle.DB(), "run-1", "space-1")
	repo := NewSQLRepository(handle)
	ctx := context.Background()

	seq, err := repo.LatestSeq(ctx, "run-1")
	if err != nil {
		t.Fatalf("LatestSeq: %v", err)
	}
	if seq != 0 {
		t.Fatalf("expected 0, got %d", seq)
	}

	for i := 0; i < 3; i++ {
		repo.Append(ctx, types.EventRecord{
			RunID:     "run-1",
			EventID:   types.EventID(fmt.Sprintf("evt-%d", i)),
			Timestamp: time.Now(),
			Type:      "test",
			Message:   fmt.Sprintf("event %d", i),
		})
	}

	seq2, err := repo.LatestSeq(ctx, "run-1")
	if err != nil {
		t.Fatalf("LatestSeq: %v", err)
	}
	if seq2 <= 0 {
		t.Fatalf("expected positive seq, got %d", seq2)
	}
}

func TestSQLiteRepository_Tail(t *testing.T) {
	handle := setupTestHandle(t)
	insertTestRun(t, handle.DB(), "run-1", "space-1")
	repo := NewSQLRepository(handle)
	ctx := context.Background()

	// Insert 2 initial events.
	for i := 0; i < 2; i++ {
		repo.Append(ctx, types.EventRecord{
			RunID:     "run-1",
			EventID:   types.EventID(fmt.Sprintf("evt-%d", i)),
			Timestamp: time.Now(),
			Type:      "initial",
			Message:   fmt.Sprintf("initial %d", i),
		})
	}

	offset, err := repo.LatestSeq(ctx, "run-1")
	if err != nil {
		t.Fatalf("LatestSeq: %v", err)
	}

	// Add 2 more events after the offset.
	repo.Append(ctx, types.EventRecord{RunID: "run-1", EventID: "evt-2", Timestamp: time.Now(), Type: "third", Message: "third"})
	repo.Append(ctx, types.EventRecord{RunID: "run-1", EventID: "evt-3", Timestamp: time.Now(), Type: "fourth", Message: "fourth"})

	tailCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	eventCh, errCh := repo.Tail(tailCtx, "run-1", offset)

	var tailed []domain.TailedEvent
	done := false
	for !done {
		select {
		case te, ok := <-eventCh:
			if !ok {
				done = true
				break
			}
			tailed = append(tailed, te)
			if len(tailed) >= 2 {
				cancel()
			}
		case err := <-errCh:
			if err != nil {
				t.Fatalf("Tail error: %v", err)
			}
		case <-tailCtx.Done():
			done = true
		}
	}

	if len(tailed) != 2 {
		t.Fatalf("expected 2 tailed events, got %d", len(tailed))
	}
	if tailed[0].Event.Type != "third" {
		t.Errorf("expected third, got %s", tailed[0].Event.Type)
	}
	if tailed[1].Event.Type != "fourth" {
		t.Errorf("expected fourth, got %s", tailed[1].Event.Type)
	}
	if tailed[1].NextOffset <= tailed[0].NextOffset {
		t.Errorf("expected increasing offsets")
	}
}

func TestSQLiteRepository_ListPaginated_EmptyRunID(t *testing.T) {
	handle := setupTestHandle(t)
	repo := NewSQLRepository(handle)
	_, _, err := repo.ListPaginated(context.Background(), domain.EventFilter{})
	if err == nil {
		t.Fatal("expected error for empty runID")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestSQLiteRepository_AppendEvent_SeqPassedToSpaceEntryUpsert verifies that the
// sequence number from LastInsertId is correctly passed to the projection upsert
// function.
func TestSQLiteRepository_AppendEvent_SeqPassedToSpaceEntryUpsert(t *testing.T) {
	handle := setupTestHandle(t)
	insertTestRun(t, handle.DB(), "run-1", "space-1")

	var capturedSeq int64
	repo := NewSQLRepository(handle, WithSpaceEntryUpsert(func(_ *sql.Tx, _ storagedb.Dialect, _ string, seq int64, _ types.EventRecord) error {
		capturedSeq = seq
		return nil
	}))

	ctx := context.Background()
	if err := repo.Append(ctx, types.EventRecord{
		RunID:     "run-1",
		EventID:   "evt-seq-1",
		Timestamp: time.Now(),
		Type:      "test_event",
		Message:   "seq test",
	}); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	if capturedSeq == 0 {
		t.Fatal("expected non-zero eventSeq passed to projection upsert, got 0")
	}
}
