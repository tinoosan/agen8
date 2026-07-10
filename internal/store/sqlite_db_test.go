package store

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/config"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
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

func TestCredentialOwnershipMigrationAssignsSingleExistingUser(t *testing.T) {
	db := legacyCredentialDatabase(t, []string{"user-a"})
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := migrateCredentialOwnership(tx); err != nil {
		t.Fatalf("migrateCredentialOwnership: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var userID, projectID string
	if err := db.QueryRow(`SELECT user_id, project_id FROM credentials WHERE credential_id = 'cred-legacy'`).Scan(&userID, &projectID); err != nil {
		t.Fatalf("read migrated credential: %v", err)
	}
	if userID != "user-a" || projectID != "" {
		t.Fatalf("ownership=(%q,%q) want (user-a, empty)", userID, projectID)
	}
}

func TestCredentialOwnershipMigrationRejectsAmbiguousUsers(t *testing.T) {
	db := legacyCredentialDatabase(t, []string{"user-a", "user-b"})
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	err = migrateCredentialOwnership(tx)
	if err == nil {
		t.Fatal("expected ambiguous ownership migration to fail")
	}
	if !strings.Contains(err.Error(), "cannot safely assign") {
		t.Fatalf("error=%q", err)
	}
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		t.Fatalf("rollback: %v", rollbackErr)
	}
	if hasTableColumn(t, db, "credentials", "user_id") {
		t.Fatal("failed migration left user_id behind after rollback")
	}
}

func TestMigrateSQLiteUpgradesVersionFiveCredentialOwnership(t *testing.T) {
	db, err := sql.Open("sqlite", storagedb.SQLiteDSN(storagedb.SQLitePath(t.TempDir())))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrateSQLite(db); err != nil {
		t.Fatalf("create current schema: %v", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (user_id TEXT PRIMARY KEY, created_at TEXT NOT NULL);
		INSERT INTO users (user_id, created_at) VALUES ('user-a', '2026-01-01T00:00:00Z');
		DROP INDEX idx_credentials_user_project;
		ALTER TABLE credentials DROP COLUMN project_id;
		ALTER TABLE credentials DROP COLUMN user_id;
		INSERT INTO credentials (credential_id, kind, label, status, fields_json, created_at, updated_at)
		VALUES ('cred-v5', 'api_key', 'Preserved', 'active', '[]', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
		DELETE FROM schema_version;
		INSERT INTO schema_version (version, applied_at) VALUES (5, '2026-01-01T00:00:00Z');
	`); err != nil {
		t.Fatalf("prepare version 5 schema: %v", err)
	}

	if err := migrateSQLite(db); err != nil {
		t.Fatalf("migrate version 5: %v", err)
	}
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != currentSchemaVersion {
		t.Fatalf("schema version=%d want %d", version, currentSchemaVersion)
	}
	var userID, projectID, label string
	if err := db.QueryRow(`SELECT user_id, project_id, label FROM credentials WHERE credential_id = 'cred-v5'`).Scan(&userID, &projectID, &label); err != nil {
		t.Fatalf("read migrated credential: %v", err)
	}
	if userID != "user-a" || projectID != "" || label != "Preserved" {
		t.Fatalf("migrated credential=(%q,%q,%q)", userID, projectID, label)
	}
}

func hasTableColumn(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("table info %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info %s: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info %s: %v", table, err)
	}
	return false
}

func legacyCredentialDatabase(t *testing.T, userIDs []string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", storagedb.SQLiteDSN(storagedb.SQLitePath(t.TempDir())))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`
		CREATE TABLE users (user_id TEXT PRIMARY KEY, created_at TEXT NOT NULL);
		CREATE TABLE credentials (
			credential_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			label TEXT NOT NULL,
			status TEXT NOT NULL,
			fields_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO credentials VALUES ('cred-legacy', 'api_key', 'Legacy', 'active', '[]', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
	`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	for _, userID := range userIDs {
		if _, err := db.Exec(`INSERT INTO users (user_id, created_at) VALUES (?, ?)`, userID, "2026-01-01T00:00:00Z"); err != nil {
			t.Fatalf("insert user %s: %v", userID, err)
		}
	}
	return db
}
