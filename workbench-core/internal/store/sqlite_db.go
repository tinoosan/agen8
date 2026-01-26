package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
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
}

func sqlitePath(cfg config.Config) string {
	return fsutil.GetSQLitePath(cfg.DataDir)
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
		opened, err := sql.Open("sqlite", path)
		if err != nil {
			return nil, fmt.Errorf("sqlite: open db: %w", err)
		}
		opened.SetMaxOpenConns(1)
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
	if _, err := db.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
		return fmt.Errorf("sqlite: set busy_timeout: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("sqlite: begin migration: %w", err)
	}
	for _, stmt := range sqliteMigrations {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: migrate: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit migration: %w", err)
	}
	return nil
}

func timePtrToString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
