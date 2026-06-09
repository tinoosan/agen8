package store

import (
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/config"
)

func TestSpaceIndexesCreated(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := GetDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	indexes := []string{
		"idx_spaces_user_id",
		"idx_spaces_project_updated",
	}

	for _, idx := range indexes {
		var name string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='index' AND name=?`,
			idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("index %s not found: %v", idx, err)
		}
	}
}

func TestEventTypeIndexCreated(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := GetDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	var name string
	err = db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_events_run_type'`,
	).Scan(&name)
	if err != nil {
		t.Errorf("index idx_events_run_type not found: %v", err)
	}
}

func TestProjectMemberSchemaCreated(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := GetDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	for _, name := range []string{
		"members",
		"idx_members_project_state",
	} {
		var got string
		err := db.QueryRow(
			`SELECT name FROM sqlite_master WHERE name=?`,
			name,
		).Scan(&got)
		if err != nil {
			t.Errorf("schema object %s not found: %v", name, err)
		}
	}
}

func TestRemovedTablesAreNotCreated(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := GetDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	for _, table := range []string{
		"channels",
		"channel_reads",
		"plans",
		"plan_phases",
		"plan_todos",
		"plan_comments",
		"plan_comment_reads",
		"plan_amendments",
		"plan_reviews",
	} {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("check removed table %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("removed table %s was created", table)
		}
	}
}

func TestHardCutoverRejectsLegacyTaskPlanColumns(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := GetDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE tasks (
		task_id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL DEFAULT '',
		assigned_to TEXT NOT NULL DEFAULT '',
		claimed_by_member_id TEXT NOT NULL DEFAULT '',
		task_kind TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT '',
		key_result_ref TEXT NOT NULL DEFAULT '',
		plan_phase_id TEXT,
		plan_todo_id TEXT,
		task_json TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy tasks table: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	err = validateHardCutoverSchema(tx)
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		t.Fatalf("rollback: %v", rollbackErr)
	}
	if err == nil {
		t.Fatal("expected legacy task plan column error")
	}
	if !strings.Contains(err.Error(), `table "tasks" has legacy column "plan_phase_id"`) {
		t.Fatalf("error=%q", err)
	}
	if !strings.Contains(err.Error(), "startup will not replace the active database automatically") {
		t.Fatalf("error should state that startup preserves active data, got %q", err)
	}
	if strings.Contains(err.Error(), "Remove or archive") || strings.Contains(err.Error(), "Delete agen8.db") {
		t.Fatalf("error should not imply manual deletion as the default path, got %q", err)
	}
}

func TestDecisionMemberNameRepairBackfillsFromMemberRecord(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := GetDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO members (member_id, project_id, user_id, native_session_ref, member_type, lifecycle_state, harness_kind, member_json, registered_at, updated_at)
		VALUES ('member-1', 'project-1', 'local', 'session-1', 'coordinator', 'active', 'codex', '{"displayName":"Codex backend engineer"}', '2026-06-05T15:00:00Z', '2026-06-05T15:00:00Z')`); err != nil {
		t.Fatalf("insert member: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO decisions (id, project_id, source, kind, source_identity, member_name, title, rationale, confidence, created_at)
		VALUES ('dec-1', 'project-1', 'agent', 'log', 'member-1', '', 'Readable actors', 'Keep decision actors readable.', 0.9, '2026-06-05T15:01:00Z')`); err != nil {
		t.Fatalf("insert decision: %v", err)
	}

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := repairDecisionMemberNameColumn(tx); err != nil {
		tx.Rollback()
		t.Fatalf("repairDecisionMemberNameColumn: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var got string
	if err := db.QueryRow(`SELECT member_name FROM decisions WHERE id = 'dec-1'`).Scan(&got); err != nil {
		t.Fatalf("select member_name: %v", err)
	}
	if got != "Codex backend engineer" {
		t.Fatalf("member_name=%q want %q", got, "Codex backend engineer")
	}
}

func TestGetDBPostgresRequiresReachableDatabase(t *testing.T) {
	_, err := GetDB(config.Config{
		DataDir:     t.TempDir(),
		DBDriver:    "postgres",
		DatabaseURL: "postgres://user:pass@127.0.0.1:1/agen8",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "open db") &&
		!strings.Contains(err.Error(), "connection refused") &&
		!strings.Contains(err.Error(), "connect") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPostgresSchemaSQLConvertsSQLiteOnlySyntax(t *testing.T) {
	got := postgresSchemaSQL(`CREATE TABLE events (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		credentials BLOB NOT NULL
	);`)
	for _, forbidden := range []string{
		"AUTOINCREMENT",
		"DATETIME",
		"BOOLEAN NOT NULL DEFAULT 1",
		"BLOB",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("postgres schema contains SQLite-only syntax %q: %s", forbidden, got)
		}
	}
	for _, want := range []string{
		"seq BIGSERIAL PRIMARY KEY",
		"created_at TIMESTAMPTZ NOT NULL",
		"enabled BOOLEAN NOT NULL DEFAULT TRUE",
		"credentials BYTEA NOT NULL",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("postgres schema missing %q: %s", want, got)
		}
	}
}

func TestLoadPostgresSchemaSQLConvertsConsolidatedSchema(t *testing.T) {
	got, err := loadPostgresSchemaSQL()
	if err != nil {
		t.Fatalf("loadPostgresSchemaSQL: %v", err)
	}
	for _, forbidden := range []string{
		"AUTOINCREMENT",
		"DATETIME",
		"BLOB",
		"BOOLEAN NOT NULL DEFAULT 1",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("postgres schema contains SQLite-only syntax %q", forbidden)
		}
	}
}
