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

	// Vector memory (SQLite-backed). Embeddings are stored as raw float32 little-endian blobs.
	`CREATE TABLE IF NOT EXISTS memories (
		memory_id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		filename TEXT NOT NULL,
		source_file TEXT,
		chunk_index INTEGER DEFAULT 0,
		content TEXT NOT NULL,
		created_at TEXT NOT NULL,
		indexed_at TEXT
	);`,
	`CREATE INDEX IF NOT EXISTS idx_memories_created_at ON memories(created_at);`,
	`CREATE INDEX IF NOT EXISTS idx_memories_source_file ON memories(source_file);`,
	`CREATE INDEX IF NOT EXISTS idx_memories_indexed_at ON memories(indexed_at);`,
	`CREATE TABLE IF NOT EXISTS memory_embeddings (
		memory_id TEXT PRIMARY KEY,
		dim INTEGER NOT NULL,
		embedding BLOB NOT NULL
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
	if err := ensureMemorySchema(db); err != nil {
		return err
	}
	return nil
}

func ensureMemorySchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("sqlite: db is nil")
	}
	cols, err := sqliteTableColumns(db, "memories")
	if err != nil {
		return fmt.Errorf("sqlite: memories schema: %w", err)
	}
	if !cols["source_file"] {
		if _, err := db.Exec(`ALTER TABLE memories ADD COLUMN source_file TEXT;`); err != nil {
			return fmt.Errorf("sqlite: add memories.source_file: %w", err)
		}
	}
	if !cols["chunk_index"] {
		if _, err := db.Exec(`ALTER TABLE memories ADD COLUMN chunk_index INTEGER DEFAULT 0;`); err != nil {
			return fmt.Errorf("sqlite: add memories.chunk_index: %w", err)
		}
	}
	if !cols["indexed_at"] {
		if _, err := db.Exec(`ALTER TABLE memories ADD COLUMN indexed_at TEXT;`); err != nil {
			return fmt.Errorf("sqlite: add memories.indexed_at: %w", err)
		}
	}
	if _, err := db.Exec(`UPDATE memories SET source_file = filename WHERE source_file IS NULL;`); err != nil {
		return fmt.Errorf("sqlite: backfill memories.source_file: %w", err)
	}
	if _, err := db.Exec(`UPDATE memories SET indexed_at = created_at WHERE indexed_at IS NULL;`); err != nil {
		return fmt.Errorf("sqlite: backfill memories.indexed_at: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_source_file ON memories(source_file);`); err != nil {
		return fmt.Errorf("sqlite: index memories.source_file: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_memories_indexed_at ON memories(indexed_at);`); err != nil {
		return fmt.Errorf("sqlite: index memories.indexed_at: %w", err)
	}
	return nil
}

func sqliteTableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `);`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		out[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func timePtrToString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
