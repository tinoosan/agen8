package infra

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	harnessrun "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain/run"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var _ harnessrun.Repository = (*SQLiteRunRepository)(nil)
var _ harnessrun.Repository = (*PostgresRunRepository)(nil)

type SQLiteRunRepository struct {
	*runSQLRepository
}

type PostgresRunRepository struct {
	*runSQLRepository
}

type runSQLRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
	name    string
}

type runScannable interface {
	Scan(dest ...any) error
}

func NewSQLiteRunRepository(handle *storagedb.Handle) (*SQLiteRunRepository, error) {
	if handle == nil || handle.DB() == nil || handle.Dialect() == nil {
		return nil, fmt.Errorf("harness sqlite run repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("harness sqlite run repository: storage driver must be sqlite, got %q", handle.Driver())
	}
	if err := MigrateRunSchema(context.Background(), handle.DB()); err != nil {
		return nil, err
	}
	return &SQLiteRunRepository{runSQLRepository: &runSQLRepository{db: handle.DB(), dialect: handle.Dialect(), name: "sqlite"}}, nil
}

func NewPostgresRunRepository(handle *storagedb.Handle) (*PostgresRunRepository, error) {
	if handle == nil || handle.DB() == nil || handle.Dialect() == nil {
		return nil, fmt.Errorf("harness postgres run repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("harness postgres run repository: storage driver must be postgres, got %q", handle.Driver())
	}
	if err := MigrateRunSchema(context.Background(), handle.DB()); err != nil {
		return nil, err
	}
	return &PostgresRunRepository{runSQLRepository: &runSQLRepository{db: handle.DB(), dialect: handle.Dialect(), name: "postgres"}}, nil
}

func NewRunRepository(handle *storagedb.Handle) (harnessrun.Repository, error) {
	if handle == nil {
		return nil, fmt.Errorf("harness run repository: db handle is required")
	}
	switch handle.Driver() {
	case storagedb.DriverSQLite:
		return NewSQLiteRunRepository(handle)
	case storagedb.DriverPostgres:
		return NewPostgresRunRepository(handle)
	default:
		return nil, fmt.Errorf("harness run repository: unsupported storage driver %q", handle.Driver())
	}
}

func MigrateRunSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("harness run migration: db is nil")
	}
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS harness_runs (
			run_id             TEXT PRIMARY KEY,
			project_id         TEXT NOT NULL,
			space_id           TEXT NOT NULL,
			channel_id         TEXT NOT NULL,
			member_id          TEXT NOT NULL,
			session_id         TEXT NOT NULL,
			harness_kind       TEXT NOT NULL,
			native_session_ref TEXT NOT NULL DEFAULT '',
			turn_id            TEXT NOT NULL,
			native_turn_id     TEXT NOT NULL DEFAULT '',
			status             TEXT NOT NULL,
			stop_requested_by  TEXT NOT NULL DEFAULT '',
			stop_requested_at  TEXT,
			started_at         TEXT NOT NULL,
			completed_at       TEXT,
			error              TEXT NOT NULL DEFAULT ''
		)`)
	if err != nil {
		return fmt.Errorf("harness run migration: create table: %w", err)
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_harness_runs_project_status ON harness_runs (project_id, status, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_harness_runs_space_started ON harness_runs (space_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_harness_runs_channel_started ON harness_runs (channel_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_harness_runs_member_started ON harness_runs (member_id, started_at)`,
		`CREATE INDEX IF NOT EXISTS idx_harness_runs_session_status ON harness_runs (session_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_harness_runs_turn_id ON harness_runs (turn_id)`,
		`CREATE INDEX IF NOT EXISTS idx_harness_runs_native_turn_id ON harness_runs (native_turn_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_harness_runs_one_active_session ON harness_runs (session_id) WHERE status IN ('running', 'stop_requested')`,
	} {
		if _, err := db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("harness run migration: index: %w", err)
		}
	}
	return nil
}

func (r *runSQLRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

func (r *runSQLRepository) Save(ctx context.Context, item harnessrun.Run) error {
	if err := item.Validate(); err != nil {
		return err
	}
	var stopRequestedAt *string
	if item.StopRequestedAt != nil {
		s := item.StopRequestedAt.UTC().Format(time.RFC3339Nano)
		stopRequestedAt = &s
	}
	var completedAt *string
	if item.CompletedAt != nil {
		s := item.CompletedAt.UTC().Format(time.RFC3339Nano)
		completedAt = &s
	}
	_, err := r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO harness_runs (
			run_id, project_id, space_id, channel_id, member_id, session_id,
			harness_kind, native_session_ref, turn_id, native_turn_id, status,
			stop_requested_by, stop_requested_at, started_at, completed_at, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (run_id) DO UPDATE SET
			project_id = excluded.project_id,
			space_id = excluded.space_id,
			channel_id = excluded.channel_id,
			member_id = excluded.member_id,
			session_id = excluded.session_id,
			harness_kind = excluded.harness_kind,
			native_session_ref = excluded.native_session_ref,
			turn_id = excluded.turn_id,
			native_turn_id = excluded.native_turn_id,
			status = excluded.status,
			stop_requested_by = excluded.stop_requested_by,
			stop_requested_at = excluded.stop_requested_at,
			started_at = excluded.started_at,
			completed_at = excluded.completed_at,
			error = excluded.error`),
		item.ID,
		item.ProjectID,
		item.SpaceID,
		item.ChannelID,
		item.MemberID,
		item.SessionID,
		item.HarnessKind,
		item.NativeSessionRef,
		item.TurnID,
		item.NativeTurnID,
		string(item.Status),
		item.StopRequestedBy,
		stopRequestedAt,
		item.StartedAt.UTC().Format(time.RFC3339Nano),
		completedAt,
		item.Error,
	)
	if err != nil {
		return fmt.Errorf("save harness run %s: %w", item.ID, err)
	}
	return nil
}

func (r *runSQLRepository) Get(ctx context.Context, id string) (*harnessrun.Run, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("run id is required")
	}
	row := r.db.QueryRowContext(ctx, r.rebind(runSelectSQL()+` WHERE run_id = ?`), id)
	return scanRun(row)
}

func (r *runSQLRepository) GetActiveBySession(ctx context.Context, sessionID string) (*harnessrun.Run, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	row := r.db.QueryRowContext(ctx, r.rebind(runSelectSQL()+` WHERE session_id = ? AND status IN ('running', 'stop_requested') ORDER BY started_at DESC LIMIT 1`), sessionID)
	return scanRun(row)
}

func (r *runSQLRepository) GetByTurnID(ctx context.Context, turnID string) (*harnessrun.Run, error) {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return nil, fmt.Errorf("turn id is required")
	}
	row := r.db.QueryRowContext(ctx, r.rebind(runSelectSQL()+` WHERE turn_id = ? OR native_turn_id = ? ORDER BY started_at DESC LIMIT 1`), turnID, turnID)
	return scanRun(row)
}

func (r *runSQLRepository) List(ctx context.Context, filter harnessrun.Filter) ([]harnessrun.Run, error) {
	clauses := []string{"1=1"}
	args := []any{}
	if value := strings.TrimSpace(filter.ProjectID); value != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.SpaceID); value != "" {
		clauses = append(clauses, "space_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ChannelID); value != "" {
		clauses = append(clauses, "channel_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.MemberID); value != "" {
		clauses = append(clauses, "member_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.SessionID); value != "" {
		clauses = append(clauses, "session_id = ?")
		args = append(args, value)
	}
	if len(filter.Status) > 0 {
		placeholders := make([]string, 0, len(filter.Status))
		for _, status := range filter.Status {
			if !harnessrun.ValidStatus(status) {
				return nil, fmt.Errorf("invalid run status %q", status)
			}
			placeholders = append(placeholders, "?")
			args = append(args, string(status))
		}
		clauses = append(clauses, "status IN ("+strings.Join(placeholders, ", ")+")")
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, r.rebind(runSelectSQL()+` WHERE `+strings.Join(clauses, " AND ")+` ORDER BY started_at DESC, run_id DESC LIMIT ?`), args...)
	if err != nil {
		return nil, fmt.Errorf("list harness runs: %w", err)
	}
	defer rows.Close()
	var out []harnessrun.Run
	for rows.Next() {
		item, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list harness runs rows: %w", err)
	}
	return out, nil
}

func (r *runSQLRepository) MarkRuntimeLost(ctx context.Context) ([]harnessrun.Run, error) {
	rows, err := r.List(ctx, harnessrun.Filter{
		Status: []harnessrun.Status{harnessrun.StatusRunning, harnessrun.StatusStopRequested},
		Limit:  200,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	completedAt := time.Now().UTC()
	now := completedAt.Format(time.RFC3339Nano)
	_, err = r.db.ExecContext(ctx, r.rebind(`
		UPDATE harness_runs
		SET status = 'failed', completed_at = ?, error = 'runtime_lost'
		WHERE status IN ('running', 'stop_requested')`), now)
	if err != nil {
		return nil, fmt.Errorf("mark harness runs runtime lost: %w", err)
	}
	for i := range rows {
		rows[i].Status = harnessrun.StatusFailed
		rows[i].CompletedAt = &completedAt
		rows[i].Error = "runtime_lost"
	}
	return rows, nil
}

func runSelectSQL() string {
	return `SELECT run_id, project_id, space_id, channel_id, member_id, session_id,
		harness_kind, native_session_ref, turn_id, native_turn_id, status,
		stop_requested_by, stop_requested_at, started_at, completed_at, error
		FROM harness_runs`
}

func scanRun(row runScannable) (*harnessrun.Run, error) {
	var item harnessrun.Run
	var status string
	var stopRequestedAt sql.NullString
	var startedAt string
	var completedAt sql.NullString
	if err := row.Scan(
		&item.ID,
		&item.ProjectID,
		&item.SpaceID,
		&item.ChannelID,
		&item.MemberID,
		&item.SessionID,
		&item.HarnessKind,
		&item.NativeSessionRef,
		&item.TurnID,
		&item.NativeTurnID,
		&status,
		&item.StopRequestedBy,
		&stopRequestedAt,
		&startedAt,
		&completedAt,
		&item.Error,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	item.Status = harnessrun.Status(status)
	parsedStartedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(startedAt))
	if err != nil {
		return nil, fmt.Errorf("parse harness run started_at %q: %w", startedAt, err)
	}
	item.StartedAt = parsedStartedAt.UTC()
	if stopRequestedAt.Valid && strings.TrimSpace(stopRequestedAt.String) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(stopRequestedAt.String))
		if err != nil {
			return nil, fmt.Errorf("parse harness run stop_requested_at %q: %w", stopRequestedAt.String, err)
		}
		parsed = parsed.UTC()
		item.StopRequestedAt = &parsed
	}
	if completedAt.Valid && strings.TrimSpace(completedAt.String) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(completedAt.String))
		if err != nil {
			return nil, fmt.Errorf("parse harness run completed_at %q: %w", completedAt.String, err)
		}
		parsed = parsed.UTC()
		item.CompletedAt = &parsed
	}
	if err := item.Validate(); err != nil {
		return nil, fmt.Errorf("scan harness run %s: %w", item.ID, err)
	}
	return &item, nil
}
