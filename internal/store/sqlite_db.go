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

	"github.com/tinoosan/agen8/internal/config"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// currentSchemaVersion is the current MCP-first schema baseline. Incompatible
// local databases must be migrated or explicitly archived by the operator;
// startup refuses to silently replace active user data.
const (
	currentSchemaVersion           = 6
	minimumMigratableSchemaVersion = 5
)

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
// Local SQLite state lives in the configured Agen8 data directory, not in the
// project repository. User isolation is enforced via user_id ownership.
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
	openCfg := storagedb.Config{
		Driver:       storagedb.DriverSQLite,
		DataDir:      cfg.DataDir,
		MigrationKey: "app",
		Migrate: func(_ context.Context, db *sql.DB, _ storagedb.Driver) error {
			return migrateSQLite(db)
		},
	}
	handle, err := storagedb.Open(ctx, openCfg)
	if err == nil {
		return handle, nil
	}
	return nil, err
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
		return rollbackWithError(tx, fmt.Errorf("sqlite: schema_version: %w", err))
	}
	version, err := currentSchema(tx)
	if err != nil {
		return rollbackWithError(tx, err)
	}
	if version > currentSchemaVersion || (version > 0 && version < minimumMigratableSchemaVersion) {
		return rollbackWithError(tx, fmt.Errorf("sqlite: incompatible schema version %d; this build supports preserving migrations from version %d through %d", version, minimumMigratableSchemaVersion, currentSchemaVersion))
	}
	if version == 0 {
		stmts, err := loadMigrationSQL()
		if err != nil {
			return rollbackWithError(tx, fmt.Errorf("sqlite: load migrations: %w", err))
		}
		for _, stmt := range stmts {
			if _, err := tx.Exec(stmt); err != nil {
				return rollbackWithError(tx, fmt.Errorf("sqlite: migrate: %w", err))
			}
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_version (version, applied_at) VALUES (?, ?)`,
			currentSchemaVersion,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return rollbackWithError(tx, fmt.Errorf("sqlite: record schema version: %w", err))
		}
	}
	if err := migrateCredentialOwnership(tx); err != nil {
		return rollbackWithError(tx, err)
	}
	if version > 0 && version < currentSchemaVersion {
		if _, err := tx.Exec(
			`INSERT INTO schema_version (version, applied_at) VALUES (?, ?)`,
			currentSchemaVersion,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return rollbackWithError(tx, fmt.Errorf("sqlite: record migrated schema version: %w", err))
		}
	}

	if err := repairDecisionMemberNameColumn(tx); err != nil {
		return rollbackWithError(tx, err)
	}
	if err := validateHardCutoverSchema(tx); err != nil {
		return rollbackWithError(tx, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit migration: %w", err)
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
		{name: "spaces", columns: []string{"space_id", "user_id", "project_id"}, forbid: []string{"session_id", "run_id", "plan_mode"}},
		{name: "credentials", columns: []string{"credential_id", "user_id", "project_id"}},
		{name: "user_profiles", columns: []string{"user_id", "profile_json"}, forbid: nil},
		{name: "project_spaces", columns: []string{"user_id", "space_id"}, forbid: []string{"primary_session_id"}},
		{name: "project_registry", columns: []string{"user_id", "project_root", "project_id"}, forbid: nil},
		{name: "integration_credentials", columns: []string{"user_id", "project_id", "owner_type", "owner_id"}, forbid: nil},
		{name: "decisions", columns: []string{"id", "project_id", "source_identity", "member_name", "invalidation_conditions_json"}, forbid: nil},
		{name: "agent_space_entries", columns: []string{"entry_id", "event_id", "run_id", "kind", "surface", "created_at"}, forbid: nil},
		{name: "members", columns: []string{"member_id", "project_id", "member_type", "lifecycle_state", "harness_kind", "member_json"}, forbid: []string{"space_id", "role_id", "session_token_hash", "session_token_prefix", "provisioning_mode", "external_session_id"}},
		{name: "tasks", columns: []string{"task_id", "project_id", "assigned_to", "claimed_by_member_id", "task_kind", "status", "key_result_ref", "task_json"}, forbid: []string{"plan_phase_id", "plan_todo_id"}},
	}

	for _, tbl := range required {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, tbl.name).Scan(&exists); err != nil {
			return fmt.Errorf("sqlite: validate schema: check table %s: %w", tbl.name, err)
		}
		if exists == 0 {
			if tbl.name == "tasks" {
				continue
			}
			return hardCutoverSchemaError("missing table %q", tbl.name)
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
				return closeRowsWithError(rows, fmt.Errorf("sqlite: validate schema: scan table info %s: %w", tbl.name, err))
			}
			cols[strings.TrimSpace(name)] = struct{}{}
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("sqlite: validate schema: close table info %s: %w", tbl.name, err)
		}

		for _, want := range tbl.columns {
			if _, ok := cols[want]; !ok {
				return hardCutoverSchemaError("table %q missing column %q", tbl.name, want)
			}
		}
		for _, bad := range tbl.forbid {
			if _, ok := cols[bad]; ok {
				return hardCutoverSchemaError("table %q has legacy column %q", tbl.name, bad)
			}
		}
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
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&exists); err != nil {
			return fmt.Errorf("sqlite: validate schema: check removed table %s: %w", table, err)
		}
		if exists > 0 {
			return hardCutoverSchemaError("removed table %q exists", table)
		}
	}
	return nil
}

func migrateCredentialOwnership(tx *sql.Tx) error {
	if tx == nil {
		return fmt.Errorf("sqlite: migrate credential ownership: transaction is nil")
	}
	rows, err := tx.Query(`PRAGMA table_info(credentials)`)
	if err != nil {
		return fmt.Errorf("sqlite: inspect credential ownership: %w", err)
	}
	columns := map[string]struct{}{}
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return closeRowsWithError(rows, fmt.Errorf("sqlite: scan credential ownership schema: %w", err))
		}
		columns[strings.TrimSpace(name)] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return closeRowsWithError(rows, fmt.Errorf("sqlite: iterate credential ownership schema: %w", err))
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("sqlite: close credential ownership schema: %w", err)
	}
	if _, ok := columns["user_id"]; !ok {
		if _, err := tx.Exec(`ALTER TABLE credentials ADD COLUMN user_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("sqlite: add credential user ownership: %w", err)
		}
	}
	if _, ok := columns["project_id"]; !ok {
		if _, err := tx.Exec(`ALTER TABLE credentials ADD COLUMN project_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("sqlite: add credential project ownership: %w", err)
		}
	}

	var unowned int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM credentials WHERE TRIM(user_id) = ''`).Scan(&unowned); err != nil {
		return fmt.Errorf("sqlite: count unowned credentials: %w", err)
	}
	if unowned > 0 {
		ownerRows, err := tx.Query(`SELECT user_id FROM users ORDER BY created_at ASC, user_id ASC LIMIT 2`)
		if err != nil {
			return fmt.Errorf("sqlite: resolve legacy credential owner: %w", err)
		}
		var owners []string
		for ownerRows.Next() {
			var owner string
			if err := ownerRows.Scan(&owner); err != nil {
				return closeRowsWithError(ownerRows, fmt.Errorf("sqlite: scan legacy credential owner: %w", err))
			}
			owners = append(owners, strings.TrimSpace(owner))
		}
		if err := ownerRows.Err(); err != nil {
			return closeRowsWithError(ownerRows, fmt.Errorf("sqlite: iterate legacy credential owners: %w", err))
		}
		if err := ownerRows.Close(); err != nil {
			return fmt.Errorf("sqlite: close legacy credential owners: %w", err)
		}
		if len(owners) != 1 || owners[0] == "" {
			return fmt.Errorf("sqlite: cannot safely assign %d legacy credentials across %d users; export or remove ambiguous credentials before upgrading", unowned, len(owners))
		}
		if _, err := tx.Exec(`UPDATE credentials SET user_id = ? WHERE TRIM(user_id) = ''`, owners[0]); err != nil {
			return fmt.Errorf("sqlite: assign legacy credential owner: %w", err)
		}
	}
	if _, err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_credentials_user_project ON credentials(user_id, project_id)`); err != nil {
		return fmt.Errorf("sqlite: index credential ownership: %w", err)
	}
	return nil
}

func hardCutoverSchemaError(format string, args ...any) error {
	detail := fmt.Sprintf(format, args...)
	return fmt.Errorf("sqlite: incompatible schema: %s. Apply a preserving migration, or run with an explicitly isolated AGEN8_DATA_DIR / --data-dir for clean checks; startup will not replace the active database automatically", detail)
}

func repairDecisionMemberNameColumn(tx *sql.Tx) error {
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = 'decisions'`).Scan(&exists); err != nil {
		return fmt.Errorf("sqlite: repair decisions member_name: check table: %w", err)
	}
	if exists == 0 {
		return nil
	}
	rows, err := tx.Query(`PRAGMA table_info(decisions)`)
	if err != nil {
		return fmt.Errorf("sqlite: repair decisions member_name: table info: %w", err)
	}
	hasMemberName := false
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return closeRowsWithError(rows, fmt.Errorf("sqlite: repair decisions member_name: scan table info: %w", err))
		}
		if strings.TrimSpace(name) == "member_name" {
			hasMemberName = true
		}
	}
	if err := rows.Err(); err != nil {
		return closeRowsWithError(rows, fmt.Errorf("sqlite: repair decisions member_name: iterate table info: %w", err))
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("sqlite: repair decisions member_name: close table info: %w", err)
	}
	if !hasMemberName {
		if _, err := tx.Exec(`ALTER TABLE decisions ADD COLUMN member_name TEXT DEFAULT ''`); err != nil {
			return fmt.Errorf("sqlite: repair decisions member_name: add column: %w", err)
		}
	}
	if _, err := tx.Exec(`
		UPDATE decisions
		SET member_name = COALESCE((
			SELECT json_extract(members.member_json, '$.displayName')
			FROM members
			WHERE members.member_id = decisions.source_identity
		), '')
		WHERE TRIM(COALESCE(member_name, '')) = ''
		  AND TRIM(COALESCE(source_identity, '')) <> ''
	`); err != nil {
		return fmt.Errorf("sqlite: repair decisions member_name: backfill from members: %w", err)
	}
	return nil
}

func rollbackWithError(tx *sql.Tx, err error) error {
	if tx == nil {
		return err
	}
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		return fmt.Errorf("%w; rollback failed: %v", err, rollbackErr)
	}
	return err
}

func closeRowsWithError(rows *sql.Rows, err error) error {
	if rows == nil {
		return err
	}
	if closeErr := rows.Close(); closeErr != nil {
		return fmt.Errorf("%w; close rows: %v", err, closeErr)
	}
	return err
}

func currentSchema(tx *sql.Tx) (int, error) {
	var version int
	if err := tx.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("sqlite: read schema version: %w", err)
	}
	return version, nil
}
