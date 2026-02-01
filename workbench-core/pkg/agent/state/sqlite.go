package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/tinoosan/workbench-core/pkg/types"
)

type SQLiteStore struct {
	path string

	mu  sync.Mutex
	db  *sql.DB
	once sync.Once
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("sqlite store path is required")
	}
	return &SQLiteStore{path: path}, nil
}

func (s *SQLiteStore) init() error {
	var initErr error
	s.once.Do(func() {
		if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
			initErr = fmt.Errorf("sqlite: create dir: %w", err)
			return
		}
		db, err := sql.Open("sqlite", s.path)
		if err != nil {
			initErr = fmt.Errorf("sqlite: open: %w", err)
			return
		}
		db.SetMaxOpenConns(1)
		if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: set journal_mode: %w", err)
			return
		}
		if _, err := db.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: set busy_timeout: %w", err)
			return
		}
		if _, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS task_state (
				task_id TEXT PRIMARY KEY,
				status TEXT NOT NULL,
				attempts INTEGER NOT NULL,
				lease_until TEXT,
				updated_at TEXT NOT NULL,
				result_json TEXT,
				error TEXT
			);
		`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: create task_state: %w", err)
			return
		}
		if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_state_status ON task_state(status);`); err != nil {
			_ = db.Close()
			initErr = fmt.Errorf("sqlite: create index: %w", err)
			return
		}
		s.db = db
	})
	return initErr
}

func (s *SQLiteStore) dbConn() (*sql.DB, error) {
	if err := s.init(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	db := s.db
	s.mu.Unlock()
	if db == nil {
		return nil, fmt.Errorf("sqlite: db not initialized")
	}
	return db, nil
}

func parseTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (s *SQLiteStore) RecoverExpired(ctx context.Context, now time.Time) error {
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		UPDATE task_state
		SET status = ?, updated_at = ?, error = COALESCE(error, 'lease expired')
		WHERE status = ? AND lease_until IS NOT NULL AND lease_until != '' AND lease_until < ?;
	`, string(StatusFailed), now.UTC().Format(time.RFC3339Nano), string(StatusActive), now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("recover expired: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Claim(ctx context.Context, taskID string, ttl time.Duration) (ClaimResult, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ClaimResult{}, fmt.Errorf("taskID is required")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	db, err := s.dbConn()
	if err != nil {
		return ClaimResult{}, err
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(ttl)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ClaimResult{}, fmt.Errorf("claim begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var status string
	var attempts int
	var leaseRaw string
	row := tx.QueryRowContext(ctx, `SELECT status, attempts, COALESCE(lease_until, '') FROM task_state WHERE task_id = ?`, taskID)
	switch err := row.Scan(&status, &attempts, &leaseRaw); err {
	case sql.ErrNoRows:
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO task_state (task_id, status, attempts, lease_until, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, taskID, string(StatusActive), 1, leaseUntil.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano)); err != nil {
			return ClaimResult{}, fmt.Errorf("claim insert: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return ClaimResult{}, fmt.Errorf("claim commit: %w", err)
		}
		return ClaimResult{Claimed: true, Attempts: 1, LeaseUntil: leaseUntil}, nil
	case nil:
		// continue
	default:
		return ClaimResult{}, fmt.Errorf("claim select: %w", err)
	}

	switch Status(strings.ToLower(strings.TrimSpace(status))) {
	case StatusSucceeded, StatusCanceled, StatusQuarantined:
		return ClaimResult{Claimed: false, Attempts: attempts}, tx.Commit()
	}

	lease := parseTime(leaseRaw)
	if !lease.IsZero() && lease.After(now) {
		return ClaimResult{Claimed: false, Attempts: attempts, LeaseUntil: lease}, tx.Commit()
	}

	attempts++
	if _, err := tx.ExecContext(ctx, `
		UPDATE task_state
		SET status = ?, attempts = ?, lease_until = ?, updated_at = ?
		WHERE task_id = ?
	`, string(StatusActive), attempts, leaseUntil.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), taskID); err != nil {
		return ClaimResult{}, fmt.Errorf("claim update: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return ClaimResult{}, fmt.Errorf("claim commit: %w", err)
	}
	return ClaimResult{Claimed: true, Attempts: attempts, LeaseUntil: leaseUntil}, nil
}

func (s *SQLiteStore) Extend(ctx context.Context, taskID string, ttl time.Duration) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	leaseUntil := now.Add(ttl)
	_, err = db.ExecContext(ctx, `
		UPDATE task_state
		SET lease_until = ?, updated_at = ?
		WHERE task_id = ? AND status = ?
	`, leaseUntil.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), taskID, string(StatusActive))
	if err != nil {
		return fmt.Errorf("extend: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Complete(ctx context.Context, taskID string, result types.TaskResult) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	b, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	status := Status(strings.ToLower(strings.TrimSpace(string(result.Status))))
	switch status {
	case StatusSucceeded, StatusFailed, StatusCanceled:
	default:
		// Fall back to failed if the provided status isn't terminal.
		status = StatusFailed
	}
	_, err = db.ExecContext(ctx, `
		UPDATE task_state
		SET status = ?, lease_until = '', updated_at = ?, result_json = ?, error = ?
		WHERE task_id = ?
	`, string(status), now.Format(time.RFC3339Nano), string(b), strings.TrimSpace(result.Error), taskID)
	if err != nil {
		return fmt.Errorf("complete: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Quarantine(ctx context.Context, taskID string, errMsg string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = db.ExecContext(ctx, `
		UPDATE task_state
		SET status = ?, lease_until = '', updated_at = ?, error = ?
		WHERE task_id = ?
	`, string(StatusQuarantined), now.Format(time.RFC3339Nano), strings.TrimSpace(errMsg), taskID)
	if err != nil {
		return fmt.Errorf("quarantine: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Get(ctx context.Context, taskID string) (Record, bool, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Record{}, false, fmt.Errorf("taskID is required")
	}
	db, err := s.dbConn()
	if err != nil {
		return Record{}, false, err
	}
	var status string
	var attempts int
	var leaseRaw string
	var updatedRaw string
	var resultJSON string
	var errMsg string
	row := db.QueryRowContext(ctx, `
		SELECT status, attempts, COALESCE(lease_until, ''), updated_at, COALESCE(result_json, ''), COALESCE(error, '')
		FROM task_state
		WHERE task_id = ?
	`, taskID)
	if err := row.Scan(&status, &attempts, &leaseRaw, &updatedRaw, &resultJSON, &errMsg); err != nil {
		if err == sql.ErrNoRows {
			return Record{}, false, nil
		}
		return Record{}, false, fmt.Errorf("get: %w", err)
	}
	rec := Record{
		TaskID:     taskID,
		Status:     Status(strings.ToLower(strings.TrimSpace(status))),
		Attempts:   attempts,
		LeaseUntil: parseTime(leaseRaw),
		UpdatedAt:  parseTime(updatedRaw),
		Error:      strings.TrimSpace(errMsg),
	}
	if strings.TrimSpace(resultJSON) != "" {
		var tr types.TaskResult
		if err := json.Unmarshal([]byte(resultJSON), &tr); err == nil {
			rec.Result = &tr
		}
	}
	return rec, true, nil
}

