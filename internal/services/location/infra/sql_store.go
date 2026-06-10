package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	locationdomain "github.com/tinoosan/agen8/internal/services/location/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type sqlStore struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func newSQLStore(handle *storagedb.Handle) *sqlStore {
	return &sqlStore{
		db:      handle.DB(),
		dialect: handle.Dialect(),
	}
}

func (r *sqlStore) Get(ctx context.Context, id locationdomain.ID) (locationdomain.Record, error) {
	id = locationdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return locationdomain.Record{}, fmt.Errorf("location id is required")
	}
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT location_id, kind, label, host, port, username, status, ready,
		       credential_ref, reachable, file_browsing, exec_ready, codex_ready, claude_ready,
		       git_diff_enabled, last_probe_error, last_probed_at, created_at, updated_at
		FROM locations
		WHERE location_id = ?
	`), id)
	record, err := scanLocation(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return locationdomain.Record{}, locationdomain.ErrNotFound
		}
		return locationdomain.Record{}, fmt.Errorf("get location %s: %w", id, err)
	}
	return record, nil
}

func (r *sqlStore) List(ctx context.Context, filter locationdomain.Filter) ([]locationdomain.Record, error) {
	where, args, err := locationWhere(filter)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT location_id, kind, label, host, port, username, status, ready,
		       credential_ref, reachable, file_browsing, exec_ready, codex_ready, claude_ready,
		       git_diff_enabled, last_probe_error, last_probed_at, created_at, updated_at
		FROM locations` + where + `
		ORDER BY label ASC, location_id ASC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list locations: %w", err)
	}
	defer rows.Close()
	return scanLocations(rows)
}

func (r *sqlStore) Save(ctx context.Context, record locationdomain.Record) (locationdomain.Record, error) {
	record, err := validateLocation(record)
	if err != nil {
		return locationdomain.Record{}, err
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO locations (
			location_id, kind, label, host, port, username, status, ready,
			credential_ref, reachable, file_browsing, exec_ready, codex_ready, claude_ready,
			git_diff_enabled, last_probe_error, last_probed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(location_id) DO UPDATE SET
			kind = excluded.kind,
			label = excluded.label,
			host = excluded.host,
			port = excluded.port,
			username = excluded.username,
			status = excluded.status,
			ready = excluded.ready,
			credential_ref = excluded.credential_ref,
			reachable = excluded.reachable,
			file_browsing = excluded.file_browsing,
			exec_ready = excluded.exec_ready,
			codex_ready = excluded.codex_ready,
			claude_ready = excluded.claude_ready,
			git_diff_enabled = excluded.git_diff_enabled,
			last_probe_error = excluded.last_probe_error,
			last_probed_at = excluded.last_probed_at,
			updated_at = excluded.updated_at
	`),
		record.ID, record.Kind, record.Label, record.Address.Host, record.Address.Port, record.Address.Username,
		record.Status, boolInt(record.Ready), record.CredentialRef, boolInt(record.Probe.Reachable),
		boolInt(record.Probe.FileBrowsing), boolInt(record.Probe.Exec), boolInt(record.Probe.Codex), boolInt(record.Probe.Claude),
		boolInt(record.GitDiffEnabled),
		record.LastProbeError, formatOptionalTime(record.LastProbedAt), formatTime(record.CreatedAt), formatTime(record.UpdatedAt),
	)
	if err != nil {
		return locationdomain.Record{}, fmt.Errorf("save location %s: %w", record.ID, err)
	}
	return r.Get(ctx, record.ID)
}

func (r *sqlStore) Delete(ctx context.Context, id locationdomain.ID) error {
	id = locationdomain.ID(strings.TrimSpace(string(id)))
	if id == "" {
		return fmt.Errorf("location id is required")
	}
	res, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM locations WHERE location_id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete location %s: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete location %s: %w", id, err)
	}
	if affected == 0 {
		return locationdomain.ErrNotFound
	}
	return nil
}

func (r *sqlStore) ensureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS locations (
			location_id TEXT PRIMARY KEY,
			kind TEXT NOT NULL,
			label TEXT NOT NULL,
			host TEXT NOT NULL DEFAULT '',
			port INTEGER NOT NULL DEFAULT 0,
			username TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			ready INTEGER NOT NULL DEFAULT 0,
			credential_ref TEXT NOT NULL DEFAULT '',
			reachable INTEGER NOT NULL DEFAULT 0,
			file_browsing INTEGER NOT NULL DEFAULT 0,
			exec_ready INTEGER NOT NULL DEFAULT 0,
			codex_ready INTEGER NOT NULL DEFAULT 0,
			claude_ready INTEGER NOT NULL DEFAULT 0,
			git_diff_enabled INTEGER NOT NULL DEFAULT 0,
			last_probe_error TEXT NOT NULL DEFAULT '',
			last_probed_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure locations table: %w", err)
	}
	if err := r.ensureColumn(ctx, "locations", "claude_ready", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := r.ensureColumn(ctx, "locations", "git_diff_enabled", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_locations_kind ON locations(kind)`,
		`CREATE INDEX IF NOT EXISTS idx_locations_status ON locations(status)`,
		`CREATE INDEX IF NOT EXISTS idx_locations_ready ON locations(ready)`,
		`CREATE INDEX IF NOT EXISTS idx_locations_updated_at ON locations(updated_at)`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure locations index: %w", err)
		}
	}
	return nil
}

func (r *sqlStore) ensureColumn(ctx context.Context, table, name, definition string) error {
	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, name, definition)
	if _, err := r.db.ExecContext(ctx, stmt); err != nil {
		if isDuplicateColumnError(err) {
			return nil
		}
		return fmt.Errorf("ensure %s.%s column: %w", table, name, err)
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}

func (r *sqlStore) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}
