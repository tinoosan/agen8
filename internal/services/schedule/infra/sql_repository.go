package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	schedule "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type SQLRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func newSQLRepository(db *sql.DB, dialect storagedb.Dialect) *SQLRepository {
	return &SQLRepository{db: db, dialect: dialect}
}

func (r *SQLRepository) Get(ctx context.Context, id schedule.EntryID) (schedule.Entry, error) {
	id = schedule.EntryID(strings.TrimSpace(string(id)))
	if id == "" {
		return schedule.Entry{}, fmt.Errorf("schedule entry id is required")
	}
	var raw []byte
	err := r.db.QueryRowContext(ctx, r.rebind(`SELECT entry_json FROM schedule_entries WHERE entry_id = ?`), string(id)).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return schedule.Entry{}, schedule.ErrNotFound
		}
		return schedule.Entry{}, fmt.Errorf("get schedule entry %s: %w", id, err)
	}
	entry, err := unmarshalEntry(raw)
	if err != nil {
		return schedule.Entry{}, fmt.Errorf("unmarshal schedule entry %s: %w", id, err)
	}
	return entry, nil
}

func (r *SQLRepository) List(ctx context.Context, filter schedule.Filter) ([]schedule.Entry, error) {
	where, args := entryWhere(filter)
	query := "SELECT entry_json FROM schedule_entries" + where + " ORDER BY next_run_at ASC, created_at DESC, entry_id ASC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list schedule entries: %w", err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (r *SQLRepository) ListDue(ctx context.Context, now time.Time, limit int) ([]schedule.Entry, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive")
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT entry_json
		FROM schedule_entries
		WHERE status = ? AND next_run_at IS NOT NULL AND next_run_at <= ?
		ORDER BY next_run_at ASC, entry_id ASC
		LIMIT ?
	`), string(schedule.EntryStatusActive), timeString(now), limit)
	if err != nil {
		return nil, fmt.Errorf("list due schedule entries: %w", err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (r *SQLRepository) ListRuns(ctx context.Context, filter schedule.RunFilter) ([]schedule.Run, error) {
	where, args := runWhere(filter)
	query := "SELECT run_json FROM schedule_runs" + where + " ORDER BY due_at DESC, started_at DESC, run_id ASC"
	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list schedule runs: %w", err)
	}
	defer rows.Close()
	return scanRuns(rows)
}

func (r *SQLRepository) Create(ctx context.Context, entry schedule.Entry) error {
	return r.saveEntry(ctx, entry, true)
}

func (r *SQLRepository) Update(ctx context.Context, entry schedule.Entry) error {
	return r.saveEntry(ctx, entry, false)
}

func (r *SQLRepository) ClaimDue(ctx context.Context, run schedule.Run) (schedule.Run, bool, error) {
	if err := run.Validate(); err != nil {
		return schedule.Run{}, false, err
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return schedule.Run{}, false, fmt.Errorf("marshal schedule run: %w", err)
	}
	result, err := r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO schedule_runs (
			run_id, entry_id, space_id, due_at, started_at, finished_at, status,
			target_kind, target_object_id, error, run_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(entry_id, due_at) DO NOTHING
	`),
		string(run.ID),
		string(run.EntryID),
		string(run.SpaceID),
		timeString(run.DueAt),
		timeString(run.StartedAt),
		optionalTimeString(run.FinishedAt),
		string(run.Status),
		string(run.TargetKind),
		run.TargetObjectID,
		run.Error,
		string(payload),
	)
	if err != nil {
		return schedule.Run{}, false, fmt.Errorf("claim schedule run %s: %w", run.ID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return schedule.Run{}, false, fmt.Errorf("inspect schedule run claim result: %w", err)
	}
	if affected == 0 {
		existing, err := r.getRunByEntryDue(ctx, run.EntryID, run.DueAt)
		if err != nil {
			return schedule.Run{}, false, err
		}
		return existing, false, nil
	}
	return run, true, nil
}

func (r *SQLRepository) UpdateRun(ctx context.Context, run schedule.Run) error {
	if err := run.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("marshal schedule run: %w", err)
	}
	result, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE schedule_runs SET
			finished_at = ?,
			status = ?,
			target_kind = ?,
			target_object_id = ?,
			error = ?,
			run_json = ?
		WHERE run_id = ?
	`),
		optionalTimeString(run.FinishedAt),
		string(run.Status),
		string(run.TargetKind),
		run.TargetObjectID,
		run.Error,
		string(payload),
		string(run.ID),
	)
	if err != nil {
		return fmt.Errorf("update schedule run %s: %w", run.ID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect schedule run update result: %w", err)
	}
	if affected == 0 {
		return schedule.ErrRunNotFound
	}
	return nil
}

func (r *SQLRepository) saveEntry(ctx context.Context, entry schedule.Entry, create bool) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal schedule entry: %w", err)
	}
	if create {
		_, err = r.db.ExecContext(ctx, r.rebind(`
			INSERT INTO schedule_entries (
				entry_id, space_id, status, target_kind, next_run_at, expires_at,
				created_at, updated_at, dedupe_key, entry_json
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`),
			string(entry.ID),
			string(entry.SpaceID),
			string(entry.Status),
			string(entry.Target.Kind),
			optionalTimeString(entry.NextRunAt),
			optionalTimeString(entry.ExpiresAt),
			timeString(entry.CreatedAt),
			timeString(entry.UpdatedAt),
			entry.DedupeKey,
			string(payload),
		)
		if err != nil {
			return fmt.Errorf("create schedule entry %s: %w", entry.ID, err)
		}
		return nil
	}
	result, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE schedule_entries SET
			space_id = ?,
			status = ?,
			target_kind = ?,
			next_run_at = ?,
			expires_at = ?,
			updated_at = ?,
			dedupe_key = ?,
			entry_json = ?
		WHERE entry_id = ?
	`),
		string(entry.SpaceID),
		string(entry.Status),
		string(entry.Target.Kind),
		optionalTimeString(entry.NextRunAt),
		optionalTimeString(entry.ExpiresAt),
		timeString(entry.UpdatedAt),
		entry.DedupeKey,
		string(payload),
		string(entry.ID),
	)
	if err != nil {
		return fmt.Errorf("update schedule entry %s: %w", entry.ID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect schedule entry update result: %w", err)
	}
	if affected == 0 {
		return schedule.ErrNotFound
	}
	return nil
}

func (r *SQLRepository) getRunByEntryDue(ctx context.Context, entryID schedule.EntryID, dueAt time.Time) (schedule.Run, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT run_json
		FROM schedule_runs
		WHERE entry_id = ? AND due_at = ?
	`), string(entryID), timeString(dueAt)).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return schedule.Run{}, schedule.ErrRunNotFound
		}
		return schedule.Run{}, fmt.Errorf("get schedule run for entry %s at %s: %w", entryID, dueAt.UTC().Format(time.RFC3339Nano), err)
	}
	run, err := unmarshalRun(raw)
	if err != nil {
		return schedule.Run{}, fmt.Errorf("unmarshal schedule run for entry %s: %w", entryID, err)
	}
	return run, nil
}

func (r *SQLRepository) ensureSchema(ctx context.Context, postgres bool) error {
	jsonType := "TEXT"
	if postgres {
		jsonType = "JSONB"
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS schedule_entries (
			entry_id TEXT PRIMARY KEY,
			space_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT '',
			target_kind TEXT NOT NULL DEFAULT '',
			next_run_at TEXT,
			expires_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			dedupe_key TEXT NOT NULL DEFAULT '',
			entry_json ` + jsonType + ` NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_entries_space_status ON schedule_entries(space_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_entries_next_run ON schedule_entries(status, next_run_at)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_entries_dedupe ON schedule_entries(space_id, dedupe_key, status)`,
		`CREATE TABLE IF NOT EXISTS schedule_runs (
			run_id TEXT PRIMARY KEY,
			entry_id TEXT NOT NULL,
			space_id TEXT NOT NULL DEFAULT '',
			due_at TEXT NOT NULL,
			started_at TEXT NOT NULL,
			finished_at TEXT,
			status TEXT NOT NULL DEFAULT '',
			target_kind TEXT NOT NULL DEFAULT '',
			target_object_id TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			run_json ` + jsonType + ` NOT NULL,
			UNIQUE(entry_id, due_at)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_runs_entry_due ON schedule_runs(entry_id, due_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_runs_space_started ON schedule_runs(space_id, started_at DESC)`,
	} {
		if _, err := r.db.ExecContext(ctx, r.rebind(stmt)); err != nil {
			return fmt.Errorf("ensure schedule schema: %w", err)
		}
	}
	return nil
}

func (r *SQLRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

func entryWhere(filter schedule.Filter) (string, []any) {
	var clauses []string
	var args []any
	if strings.TrimSpace(string(filter.SpaceID)) != "" {
		clauses = append(clauses, "space_id = ?")
		args = append(args, strings.TrimSpace(string(filter.SpaceID)))
	}
	if strings.TrimSpace(string(filter.Status)) != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, strings.TrimSpace(string(filter.Status)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func runWhere(filter schedule.RunFilter) (string, []any) {
	var clauses []string
	var args []any
	if strings.TrimSpace(string(filter.EntryID)) != "" {
		clauses = append(clauses, "entry_id = ?")
		args = append(args, strings.TrimSpace(string(filter.EntryID)))
	}
	if strings.TrimSpace(string(filter.SpaceID)) != "" {
		clauses = append(clauses, "space_id = ?")
		args = append(args, strings.TrimSpace(string(filter.SpaceID)))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanEntries(rows *sql.Rows) ([]schedule.Entry, error) {
	var out []schedule.Entry
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan schedule entry: %w", err)
		}
		entry, err := unmarshalEntry(raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshal schedule entry: %w", err)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedule entries: %w", err)
	}
	return out, nil
}

func scanRuns(rows *sql.Rows) ([]schedule.Run, error) {
	var out []schedule.Run
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan schedule run: %w", err)
		}
		run, err := unmarshalRun(raw)
		if err != nil {
			return nil, fmt.Errorf("unmarshal schedule run: %w", err)
		}
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schedule runs: %w", err)
	}
	return out, nil
}

func unmarshalEntry(raw []byte) (schedule.Entry, error) {
	if len(raw) == 0 {
		return schedule.Entry{}, fmt.Errorf("entry_json is empty")
	}
	var entry schedule.Entry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return schedule.Entry{}, err
	}
	return entry, nil
}

func unmarshalRun(raw []byte) (schedule.Run, error) {
	if len(raw) == 0 {
		return schedule.Run{}, fmt.Errorf("run_json is empty")
	}
	var run schedule.Run
	if err := json.Unmarshal(raw, &run); err != nil {
		return schedule.Run{}, err
	}
	return run, nil
}

func optionalTimeString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func timeString(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
