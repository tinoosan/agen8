package store

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
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
		"idx_channels_space_id",
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
		"idx_members_one_active_coordinator",
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

func TestChannelSchemaIsCanonical(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}

	db, err := GetDB(cfg)
	if err != nil {
		t.Fatalf("getSQLiteDB: %v", err)
	}

	columns := tableColumns(t, db, "channels")
	for _, column := range []string{"channel_id", "space_id", "project_id", "run_id", "member_id", "member_label", "status", "channel_json", "last_message_at"} {
		if !columns[column] {
			t.Fatalf("channels.%s missing", column)
		}
	}
}

func tableColumns(t *testing.T, db *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("pragma %s: %v", table, err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notNull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma %s: %v", table, err)
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("pragma rows %s: %v", table, err)
	}
	return columns
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
