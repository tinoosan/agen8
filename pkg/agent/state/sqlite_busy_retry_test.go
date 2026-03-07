package state

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestWithSQLiteBusyRetry_RetriesBusyErrors(t *testing.T) {
	calls := 0
	out, err := withSQLiteBusyRetry(context.Background(), func() (string, error) {
		calls++
		if calls < 3 {
			return "", fmt.Errorf("database is locked (5) (SQLITE_BUSY)")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("withSQLiteBusyRetry: %v", err)
	}
	if out != "ok" {
		t.Fatalf("result = %q, want ok", out)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3", calls)
	}
}

func TestSQLiteTaskStore_UsesBusyTimeoutEnv(t *testing.T) {
	t.Setenv("AGEN8_SQLITE_BUSY_TIMEOUT_MS", "16000")
	store, err := NewSQLiteTaskStore(filepath.Join(t.TempDir(), "agen8.db"))
	if err != nil {
		t.Fatalf("NewSQLiteTaskStore: %v", err)
	}
	db, err := store.dbConn()
	if err != nil {
		t.Fatalf("dbConn: %v", err)
	}
	var timeoutMS int
	if err := db.QueryRow(`PRAGMA busy_timeout;`).Scan(&timeoutMS); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if timeoutMS != 16000 {
		t.Fatalf("busy_timeout=%d, want 16000", timeoutMS)
	}
}

func TestSQLiteTaskStore_ConcurrentBusyRetry(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agen8.db")
	store, err := NewSQLiteTaskStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	now := time.Now().UTC()
	task := types.Task{
		TaskID:    "t-busy-test",
		SessionID: "s1",
		RunID:     "r1",
		Goal:      "test busy retry",
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Open a second connection to create write-lock contention on the same file.
	db2, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(100)")
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	var wg sync.WaitGroup

	// Repeatedly hold a write lock from db2.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, err := db2.Begin()
			if err != nil {
				return
			}
			_, _ = tx.Exec("UPDATE tasks SET updated_at = ? WHERE task_id = 'nonexistent'", time.Now().Format(time.RFC3339Nano))
			time.Sleep(10 * time.Millisecond)
			_ = tx.Commit()
		}()
	}

	// Concurrent reads/writes from the store should succeed due to busy retry.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			_, _ = store.GetTask(ctx, "t-busy-test")
			_, _ = store.ListTasks(ctx, TaskFilter{RunID: "r1"})
			_, _ = store.CountTasks(ctx, TaskFilter{RunID: "r1"})
			_, _ = store.GetRunStats(ctx, "r1")
		}
	}()

	wg.Wait()

	// Verify the task is still readable after contention.
	got, err := store.GetTask(ctx, "t-busy-test")
	if err != nil {
		t.Fatalf("GetTask after contention: %v", err)
	}
	if got.TaskID != "t-busy-test" {
		t.Fatalf("unexpected task ID: %q", got.TaskID)
	}
}
