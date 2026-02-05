package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
)

var (
	sqliteMu       sync.Mutex
	sqliteDBs      = map[string]*sql.DB{}
	sqliteMigrated = map[string]bool{}
)

const currentSchemaVersion = 5

var sqliteMigrations = []string{
	`CREATE TABLE IF NOT EXISTS sessions (
		session_id TEXT PRIMARY KEY,
		title TEXT,
		current_run_id TEXT,
		current_goal TEXT,
		runs_json TEXT,
		session_json TEXT NOT NULL,
		created_at TEXT,
		updated_at TEXT
	);`,
	// Session indexes for pagination and sorting.
	`CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at DESC);`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at DESC);`,
	`CREATE TABLE IF NOT EXISTS runs (
		run_id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		status TEXT NOT NULL,
		goal TEXT NOT NULL,
		run_json TEXT NOT NULL,
		started_at TEXT,
		finished_at TEXT,
		created_at TEXT,
		updated_at TEXT
	);`,
	`CREATE INDEX IF NOT EXISTS idx_runs_session_id ON runs(session_id);`,
	`CREATE TABLE IF NOT EXISTS events (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		ts TEXT NOT NULL,
		type TEXT NOT NULL,
		message TEXT NOT NULL,
		data_json TEXT,
		event_json TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_events_run_seq ON events(run_id, seq);`,
	// Event type index for filtered queries.
	`CREATE INDEX IF NOT EXISTS idx_events_run_type ON events(run_id, type);`,
	`CREATE TABLE IF NOT EXISTS history (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		ts TEXT NOT NULL,
		origin TEXT NOT NULL,
		kind TEXT NOT NULL,
		message TEXT NOT NULL,
		model TEXT,
		data_json TEXT,
		line_json TEXT NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_history_session_seq ON history(session_id, seq);`,
	`CREATE TABLE IF NOT EXISTS constructor_state (
		run_id TEXT PRIMARY KEY,
		updated_at TEXT,
		state_json TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS constructor_manifest (
		run_id TEXT PRIMARY KEY,
		updated_at TEXT,
		manifest_json TEXT NOT NULL
	);`,
	`CREATE TABLE IF NOT EXISTS activities (
		run_id TEXT NOT NULL,
		activity_id TEXT PRIMARY KEY,
		seq INTEGER NOT NULL,
		kind TEXT NOT NULL,
		title TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at TEXT NOT NULL,
		finished_at TEXT,
		meta_json TEXT
	);`,
	`CREATE INDEX IF NOT EXISTS idx_activities_run_seq ON activities(run_id, seq);`,
	`CREATE INDEX IF NOT EXISTS idx_activities_run_id ON activities(run_id);`,
}

func sqlitePath(cfg config.Config) string {
	return fsutil.GetSQLitePath(cfg.DataDir)
}

const (
	defaultSQLiteMaxOpenConns  = 25
	defaultSQLiteMaxIdleConns  = 25
	defaultSQLiteConnMaxLife   = 5 * time.Minute
	defaultSQLiteBusyTimeoutMS = 10000
)

func sqliteMaxOpenConns() int {
	return envInt("WORKBENCH_SQLITE_MAX_OPEN_CONNS", defaultSQLiteMaxOpenConns)
}

func sqliteMaxIdleConns() int {
	return envInt("WORKBENCH_SQLITE_MAX_IDLE_CONNS", defaultSQLiteMaxIdleConns)
}

func sqliteConnMaxLifetime() time.Duration {
	return envDuration("WORKBENCH_SQLITE_CONN_MAX_LIFETIME", defaultSQLiteConnMaxLife)
}

func sqliteBusyTimeoutMS() int {
	return envInt("WORKBENCH_SQLITE_BUSY_TIMEOUT_MS", defaultSQLiteBusyTimeoutMS)
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

func envDuration(name string, def time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func sqliteDSN(path string) string {
	// Use a file URI so per-connection PRAGMAs apply across the connection pool.
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMS()))
	u := url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}
	return u.String()
}

func getSQLiteDB(cfg config.Config) (*sql.DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	path := sqlitePath(cfg)
	sqliteMu.Lock()
	db := sqliteDBs[path]
	migrated := sqliteMigrated[path]
	sqliteMu.Unlock()

	if db == nil {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("sqlite: create data dir: %w", err)
		}
		opened, err := sql.Open("sqlite", sqliteDSN(path))
		if err != nil {
			return nil, fmt.Errorf("sqlite: open db: %w", err)
		}
		maxOpen := sqliteMaxOpenConns()
		maxIdle := sqliteMaxIdleConns()
		if maxIdle > maxOpen {
			maxIdle = maxOpen
		}
		opened.SetMaxOpenConns(maxOpen)
		opened.SetMaxIdleConns(maxIdle)
		opened.SetConnMaxLifetime(sqliteConnMaxLifetime())
		sqliteMu.Lock()
		sqliteDBs[path] = opened
		sqliteMu.Unlock()
		db = opened
	}

	if !migrated {
		if err := migrateSQLite(db); err != nil {
			return nil, err
		}
		sqliteMu.Lock()
		sqliteMigrated[path] = true
		sqliteMu.Unlock()
	}

	return db, nil
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
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: schema_version: %w", err)
	}
	version, err := currentSchema(tx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	if version < currentSchemaVersion {
		for _, stmt := range sqliteMigrations {
			if _, err := tx.Exec(stmt); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("sqlite: migrate: %w", err)
			}
		}
		if err := ensureTasksFinishedColumn(tx); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_version (version, applied_at) VALUES (?, ?)`,
			currentSchemaVersion,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: record schema version: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit migration: %w", err)
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

func ensureTasksFinishedColumn(tx *sql.Tx) error {
	rows, err := tx.Query(`PRAGMA table_info(tasks)`)
	if err != nil {
		return fmt.Errorf("sqlite: tasks table info: %w", err)
	}
	defer rows.Close()

	hasCompleted := false
	hasFinished := false
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("sqlite: scan tasks column: %w", err)
		}
		switch name {
		case "finished_at":
			hasFinished = true
		case "completed_at":
			hasCompleted = true
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: read tasks columns: %w", err)
	}
	if !hasCompleted || hasFinished {
		return nil
	}
	if _, err := tx.Exec(`ALTER TABLE tasks RENAME COLUMN completed_at TO finished_at`); err != nil {
		return fmt.Errorf("sqlite: rename tasks column: %w", err)
	}
	return nil
}
