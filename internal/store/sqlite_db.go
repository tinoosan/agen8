package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/config"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// currentSchemaVersion is bumped on hard cutover. Version 4 introduces the
// typed identity model on notifications/notification_rules: profile_id was
// dropped (it was a misnomer that conflated user identity, project scope,
// and agent persona) and replaced with user_id + project_id + subject_*.
// Existing v3 DBs are incompatible and must be wiped.
const currentSchemaVersion = 4

// loadMigrationSQL reads all .sql files from the embedded migrations directory,
// sorts them by filename, and returns a slice of SQL strings ready to execute.
func loadMigrationSQL() ([]string, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	var stmts []string
	for _, name := range names {
		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		stmts = append(stmts, string(data))
	}
	return stmts, nil
}

// GetDB returns a shared *sql.DB handle for the given config, running
// migrations if needed. This is safe for read-only metrics queries from
// packages outside of store.
func GetDB(cfg config.Config) (*sql.DB, error) {
	handle, err := getDBHandle(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return handle.DB(), nil
}

func GetDBHandle(ctx context.Context, cfg config.Config) (*storagedb.Handle, error) {
	return getDBHandle(ctx, cfg)
}

// SQLitePathForConfig resolves the SQLite path for the provided config.
// Local and hosted mode both resolve to the same DataDir-backed SQLite file
// (agen8.db). User isolation is enforced via user_id ownership in tables.
func SQLitePathForConfig(cfg config.Config) string {
	return storagedb.SQLitePath(cfg.DataDir)
}

const defaultSQLiteBusyTimeoutMS = 10000

func sqliteBusyTimeoutMS() int {
	return envInt("AGEN8_SQLITE_BUSY_TIMEOUT_MS", defaultSQLiteBusyTimeoutMS)
}

func envInt(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}

func getDBHandle(ctx context.Context, cfg config.Config) (*storagedb.Handle, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	driver := storagedb.Driver(strings.TrimSpace(cfg.DBDriver))
	if driver == "" {
		driver = storagedb.DriverSQLite
	}
	return storagedb.Open(ctx, storagedb.Config{
		Driver:       driver,
		DataDir:      cfg.DataDir,
		DatabaseURL:  cfg.DatabaseURL,
		MigrationKey: "app",
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			switch driver {
			case storagedb.DriverSQLite:
				return migrateSQLite(db)
			case storagedb.DriverPostgres:
				return migratePostgres(ctx, db)
			default:
				return fmt.Errorf("unsupported db driver %q", driver)
			}
		},
	})
}

func getSQLiteDB(cfg config.Config) (*sql.DB, error) {
	cfg.DBDriver = "sqlite"
	cfg.DatabaseURL = ""
	handle, err := getDBHandle(context.Background(), cfg)
	if err != nil {
		return nil, err
	}
	return handle.DB(), nil
}

func migratePostgres(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("postgres: db is nil")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("postgres: begin migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	);`); err != nil {
		tx.Rollback()
		return fmt.Errorf("postgres: schema_version: %w", err)
	}
	version, err := currentSchema(tx)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("postgres: %w", err)
	}
	if version > 0 && version != currentSchemaVersion {
		tx.Rollback()
		return fmt.Errorf("postgres: incompatible schema version %d; this MVP build requires a fresh schema version %d", version, currentSchemaVersion)
	}
	if version == 0 {
		stmt, err := loadPostgresSchemaSQL()
		if err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			tx.Rollback()
			return fmt.Errorf("postgres: migrate schema: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_version (version, applied_at) VALUES ($1, $2)
			 ON CONFLICT (version) DO UPDATE SET applied_at = EXCLUDED.applied_at`,
			currentSchemaVersion,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("postgres: record schema version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("postgres: commit migration: %w", err)
	}
	return nil
}

func loadPostgresSchemaSQL() (string, error) {
	data, err := migrationsFS.ReadFile("migrations/004_schema.sql")
	if err != nil {
		return "", fmt.Errorf("postgres: read schema snapshot: %w", err)
	}
	return postgresSchemaSQL(string(data)), nil
}

func postgresSchemaSQL(sql string) string {
	sql = strings.ReplaceAll(sql, "INTEGER PRIMARY KEY AUTOINCREMENT", "BIGSERIAL PRIMARY KEY")
	sql = strings.ReplaceAll(sql, "DATETIME", "TIMESTAMPTZ")
	sql = strings.ReplaceAll(sql, "BOOLEAN NOT NULL DEFAULT 1", "BOOLEAN NOT NULL DEFAULT TRUE")
	sql = strings.ReplaceAll(sql, "BLOB", "BYTEA")
	return sql
}

func migrateSQLite(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("sqlite: db is nil")
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		return fmt.Errorf("sqlite: set journal_mode: %w", err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys=ON;`); err != nil {
		return fmt.Errorf("sqlite: enable foreign keys: %w", err)
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA busy_timeout=%d;`, sqliteBusyTimeoutMS())); err != nil {
		return fmt.Errorf("sqlite: set busy_timeout: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: begin migration: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	);`); err != nil {
		tx.Rollback()
		return fmt.Errorf("sqlite: schema_version: %w", err)
	}
	version, err := currentSchema(tx)
	if err != nil {
		tx.Rollback()
		return err
	}
	if version > 0 && version != currentSchemaVersion {
		tx.Rollback()
		return fmt.Errorf("sqlite: incompatible schema version %d; this MVP build requires a fresh schema version %d", version, currentSchemaVersion)
	}
	if version == 0 {
		stmts, err := loadMigrationSQL()
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlite: load migrations: %w", err)
		}
		for _, stmt := range stmts {
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("sqlite: migrate: %w", err)
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_version (version, applied_at) VALUES (?, ?)`,
			currentSchemaVersion,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlite: record schema version: %w", err)
		}
	}

	if err := ensureChannelSchema(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := validateHardCutoverSchema(tx); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit migration: %w", err)
	}

	return nil
}

func ensureChannelSchema(tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("sqlite: ensure channel schema: transaction is nil")
	}
	rows, err := tx.Query(`PRAGMA table_info(channels)`)
	if err != nil {
		return fmt.Errorf("sqlite: pragma channels: %w", err)
	}
	hasLastMessageAt := false
	hasSpaceID := false
	hasMemberLabel := false
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
			rows.Close()
			return fmt.Errorf("sqlite: scan channels pragma: %w", err)
		}
		if name == "last_message_at" {
			hasLastMessageAt = true
		}
		if name == "space_id" {
			hasSpaceID = true
		}
		if name == "member_label" {
			hasMemberLabel = true
		}
	}
	rows.Close()
	if hasSpaceID || !hasMemberLabel {
		memberLabelSelect := `member_label`
		if !hasMemberLabel {
			memberLabelSelect = `title`
		}
		if err := rebuildChannelsCanonical(tx, hasLastMessageAt, memberLabelSelect); err != nil {
			return err
		}
		hasLastMessageAt = true
	}
	if !hasLastMessageAt {
		if _, err := tx.Exec(`ALTER TABLE channels ADD COLUMN last_message_at TEXT`); err != nil {
			return fmt.Errorf("sqlite: add channels.last_message_at: %w", err)
		}
	}
	if _, err := tx.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_space_member_run
		ON channels(space_id, member_label, run_id, member_id)`); err != nil {
		return fmt.Errorf("sqlite: ensure idx_channels_space_member_run: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_channels_space_id
		ON channels(space_id, updated_at DESC)`); err != nil {
		return fmt.Errorf("sqlite: ensure idx_channels_space_id: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_channels_member_id
		ON channels(member_id, updated_at DESC)`); err != nil {
		return fmt.Errorf("sqlite: ensure idx_channels_member_id: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_channels_run_id
		ON channels(run_id) WHERE run_id != ''`); err != nil {
		return fmt.Errorf("sqlite: ensure idx_channels_run_id: %w", err)
	}
	if _, err := tx.Exec(`CREATE TABLE IF NOT EXISTS channel_reads (
		user_id      TEXT NOT NULL,
		channel_id   TEXT NOT NULL,
		last_seen_at TEXT NOT NULL,
		PRIMARY KEY (user_id, channel_id)
	)`); err != nil {
		return fmt.Errorf("sqlite: ensure channel_reads: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_channel_reads_user
		ON channel_reads(user_id)`); err != nil {
		return fmt.Errorf("sqlite: ensure idx_channel_reads_user: %w", err)
	}
	return nil
}

func rebuildChannelsCanonical(tx *sql.Tx, hasLastMessageAt bool, memberLabelSelect string) error {
	lastMessageSelect := `last_message_at`
	if !hasLastMessageAt {
		lastMessageSelect = `NULL`
	}
	statements := []string{
		`DROP INDEX IF EXISTS idx_channels_space_kind_member_run`,
		`DROP INDEX IF EXISTS idx_channels_space_member_run`,
		`DROP INDEX IF EXISTS idx_channels_space_id`,
		`DROP INDEX IF EXISTS idx_channels_member_id`,
		`DROP INDEX IF EXISTS idx_channels_run_id`,
		`CREATE TABLE channels_next (
			channel_id TEXT PRIMARY KEY,
			space_id TEXT NOT NULL,
			project_id TEXT DEFAULT '',
			run_id TEXT DEFAULT '',
			member_id TEXT DEFAULT '',
			member_label TEXT DEFAULT '',
			title TEXT DEFAULT '',
			status TEXT NOT NULL,
			channel_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			last_message_at TEXT
		)`,
		`INSERT INTO channels_next (
			channel_id, space_id, project_id, run_id, member_id, member_label, title, status, channel_json, created_at, updated_at, last_message_at
		)
		SELECT
			channel_id, space_id, project_id, run_id, member_id, ` + memberLabelSelect + `, title, status, channel_json, created_at, updated_at, ` + lastMessageSelect + `
		FROM channels`,
		`DROP TABLE channels`,
		`ALTER TABLE channels_next RENAME TO channels`,
	}
	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite: rebuild channels: %w", err)
		}
	}
	return nil
}

func validateHardCutoverSchema(tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("sqlite: validate schema: transaction is nil")
	}

	type requiredTable struct {
		name    string
		columns []string
		forbid  []string
	}
	required := []requiredTable{
		{name: "space_runtimes", columns: []string{"space_id", "space_runtime_json"}, forbid: []string{"session_id", "session_json"}},
		{name: "runs", columns: []string{"space_id", "run_id", "run_json"}, forbid: []string{"session_id"}},
		{name: "history", columns: []string{"space_id"}, forbid: []string{"session_id"}},
		{name: "spaces", columns: []string{"space_id", "user_id", "project_id", "plan_mode"}, forbid: []string{"session_id", "run_id"}},
		{name: "channels", columns: []string{"channel_id", "space_id", "member_id", "member_label", "status", "channel_json"}, forbid: []string{"kind"}},
		{name: "user_profiles", columns: []string{"user_id", "profile_json"}, forbid: nil},
		{name: "project_spaces", columns: []string{"user_id", "space_id"}, forbid: []string{"primary_session_id"}},
		{name: "project_registry", columns: []string{"user_id", "project_root", "project_id"}, forbid: nil},
		{name: "integration_credentials", columns: []string{"user_id", "project_id", "owner_type", "owner_id"}, forbid: nil},
		{name: "decisions", columns: []string{"id", "project_id", "invalidation_conditions_json"}, forbid: nil},
		{name: "agent_space_entries", columns: []string{"entry_id", "event_id", "run_id", "kind", "surface", "created_at"}, forbid: nil},
		{name: "members", columns: []string{"member_id", "project_id", "member_type", "lifecycle_state", "harness_kind", "member_json"}, forbid: []string{"space_id", "role_id", "session_token_hash", "session_token_prefix", "provisioning_mode", "external_session_id"}},
		{name: "plans", columns: []string{"id", "space_id", "mission_id", "kr_refs_json"}, forbid: nil},
	}

	for _, tbl := range required {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, tbl.name).Scan(&exists); err != nil {
			return fmt.Errorf("sqlite: validate schema: check table %s: %w", tbl.name, err)
		}
		if exists == 0 {
			return fmt.Errorf("sqlite: incompatible schema: missing table %q (hard cutover). Delete agen8.db and retry", tbl.name)
		}
		rows, err := tx.Query(`PRAGMA table_info(` + tbl.name + `)`)
		if err != nil {
			return fmt.Errorf("sqlite: validate schema: table info %s: %w", tbl.name, err)
		}
		cols := map[string]struct{}{}
		for rows.Next() {
			var cid int
			var name, typ string
			var notnull int
			var dflt sql.NullString
			var pk int
			if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
				rows.Close()
				return fmt.Errorf("sqlite: validate schema: scan table info %s: %w", tbl.name, err)
			}
			cols[strings.TrimSpace(name)] = struct{}{}
		}
		rows.Close()

		for _, want := range tbl.columns {
			if _, ok := cols[want]; !ok {
				return fmt.Errorf("sqlite: incompatible schema: table %q missing column %q (hard cutover). Delete agen8.db and retry", tbl.name, want)
			}
		}
		for _, bad := range tbl.forbid {
			if _, ok := cols[bad]; ok {
				return fmt.Errorf("sqlite: incompatible schema: table %q has legacy column %q (hard cutover). Delete agen8.db and retry", tbl.name, bad)
			}
		}
	}
	return nil
}

func currentSchema(tx *sql.Tx) (int, error) {
	var version int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("sqlite: read schema version: %w", err)
	}
	return version, nil
}
