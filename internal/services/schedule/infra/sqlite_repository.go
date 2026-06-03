package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type SQLiteRepository struct {
	*SQLRepository
}

func NewSQLiteRepository(handle *storagedb.Handle) (*SQLiteRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("schedule sqlite repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("schedule sqlite repository: storage driver must be sqlite, got %q", handle.Driver())
	}
	repo := &SQLiteRepository{SQLRepository: newSQLRepository(handle.DB(), handle.Dialect())}
	if err := repo.cutOverLegacyTable(context.Background(), "schedule_entries", "entry_json"); err != nil {
		return nil, err
	}
	if err := repo.ensureSchema(context.Background(), false); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) cutOverLegacyTable(ctx context.Context, table string, requiredColumn string) error {
	exists, err := r.tableExists(ctx, table)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	hasColumn, err := r.tableHasColumn(ctx, table, requiredColumn)
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	if err := r.dropIndexesForTable(ctx, table); err != nil {
		return err
	}
	legacyName := fmt.Sprintf("%s_legacy_%d", table, time.Now().UnixNano())
	if _, err := r.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s", table, legacyName)); err != nil {
		return fmt.Errorf("cut over legacy schedule table %s: %w", table, err)
	}
	return nil
}

func (r *SQLiteRepository) tableExists(ctx context.Context, table string) (bool, error) {
	var name string
	err := r.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("check table %s: %w", table, err)
	}
	return true, nil
}

func (r *SQLiteRepository) tableHasColumn(ctx context.Context, table string, column string) (bool, error) {
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("inspect table %s: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scan table info %s: %w", table, err)
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterate table info %s: %w", table, err)
	}
	return false, nil
}

func (r *SQLiteRepository) dropIndexesForTable(ctx context.Context, table string) error {
	rows, err := r.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ?`, table)
	if err != nil {
		return fmt.Errorf("list indexes for %s: %w", table, err)
	}
	defer rows.Close()
	var indexes []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("scan index for %s: %w", table, err)
		}
		if strings.HasPrefix(name, "sqlite_autoindex_") {
			continue
		}
		indexes = append(indexes, name)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate indexes for %s: %w", table, err)
	}
	for _, name := range indexes {
		if _, err := r.db.ExecContext(ctx, fmt.Sprintf("DROP INDEX IF EXISTS %s", name)); err != nil {
			return fmt.Errorf("drop index %s: %w", name, err)
		}
	}
	return nil
}
