package infra

import (
	"context"
	"database/sql"
	"testing"

	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

func setupToolUsageHandle(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			_, err := db.ExecContext(ctx, `
				CREATE TABLE agent_space_entries (
					entry_id TEXT PRIMARY KEY,
					run_id TEXT NOT NULL,
					kind TEXT NOT NULL,
					title TEXT NOT NULL DEFAULT '',
					created_at TEXT,
					completed_at TEXT
				);
			`)
			return err
		},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return handle
}

func setupToolUsageHandleWithoutSchema(t *testing.T) *storagedb.Handle {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return handle
}

func TestToolUsageQuerier_QueryError_ReturnsError(t *testing.T) {
	handle := setupToolUsageHandleWithoutSchema(t)
	// Don't create table — query will fail
	q := NewSQLToolUsageQuerier(handle)

	_, err := q.QueryByRun(context.Background(), "run-1", 20)
	if err == nil {
		t.Fatalf("expected error when table doesn't exist, got nil")
	}
}

func TestToolUsageQuerier_ValidQuery_ReturnsData(t *testing.T) {
	handle := setupToolUsageHandle(t)
	db := handle.DB()

	if _, err := db.Exec(`INSERT INTO agent_space_entries (entry_id, run_id, kind, title, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"entry-1", "run-1", "tool_call", "shell_exec", "2026-03-24T10:00:00Z", "2026-03-24T10:00:01Z"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_space_entries (entry_id, run_id, kind, title, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"entry-2", "run-1", "tool_call", "shell_exec", "2026-03-24T10:01:00Z", "2026-03-24T10:01:02Z"); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO agent_space_entries (entry_id, run_id, kind, title, created_at, completed_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"entry-3", "run-1", "tool_call", "read_file", "2026-03-24T10:02:00Z", "2026-03-24T10:02:00Z"); err != nil {
		t.Fatalf("insert: %v", err)
	}

	q := NewSQLToolUsageQuerier(handle)
	result, err := q.QueryByRun(context.Background(), "run-1", 20)
	if err != nil {
		t.Fatalf("QueryByRun: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 tool types, got %d", len(result))
	}
	// shell_exec has 2 calls, should be first (sorted by count DESC)
	if result[0].Tool != "shell_exec" || result[0].Count != 2 {
		t.Errorf("result[0]: got %+v, want shell_exec with count 2", result[0])
	}
}
