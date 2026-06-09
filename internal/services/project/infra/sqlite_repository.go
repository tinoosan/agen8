package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/core/types"
	"github.com/tinoosan/agen8/internal/services/project/domain/project"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) (*SQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("project sqlite repository: db is required")
	}
	repo := &SQLiteRepository{db: db}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id types.ProjectID) (project.Record, error) {
	id = types.ProjectID(strings.TrimSpace(string(id)))
	if id == "" {
		return project.Record{}, fmt.Errorf("project id is required")
	}
	row := r.db.QueryRowContext(ctx, `
		SELECT project_id, location_id, root, title, status, created_at, updated_at, user_id
		FROM projects
		WHERE project_id = ?
	`, id)
	record, err := scanProject(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project.Record{}, project.ErrNotFound
		}
		return project.Record{}, fmt.Errorf("get project %s: %w", id, err)
	}
	return record, nil
}

func (r *SQLiteRepository) List(ctx context.Context, filter project.Filter) ([]project.Record, error) {
	where, args, err := projectWhere(filter)
	if err != nil {
		return nil, err
	}
	query := `
		SELECT project_id, location_id, root, title, status, created_at, updated_at, user_id
		FROM projects` + where + `
		ORDER BY updated_at DESC, project_id ASC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	return scanProjects(rows)
}

func (r *SQLiteRepository) Save(ctx context.Context, record project.Record) (project.Record, error) {
	record, err := validateProject(record)
	if err != nil {
		return project.Record{}, err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO projects (project_id, location_id, root, title, status, user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			location_id = excluded.location_id,
			root = excluded.root,
			title = excluded.title,
			status = excluded.status,
			user_id = excluded.user_id,
			updated_at = excluded.updated_at
	`, record.ID, record.LocationID, record.Root, record.Title, record.Status, record.UserID, timeString(record.CreatedAt), timeString(record.UpdatedAt))
	if err != nil {
		return project.Record{}, fmt.Errorf("save project %s: %w", record.ID, err)
	}
	return r.Get(ctx, record.ID)
}

func (r *SQLiteRepository) Delete(ctx context.Context, id types.ProjectID) error {
	id = types.ProjectID(strings.TrimSpace(string(id)))
	if id == "" {
		return fmt.Errorf("project id is required")
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM projects WHERE project_id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project %s: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete project %s: %w", id, err)
	}
	if affected == 0 {
		return project.ErrNotFound
	}
	return nil
}

func (r *SQLiteRepository) ensureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS projects (
			project_id TEXT PRIMARY KEY,
			location_id TEXT NOT NULL DEFAULT 'local',
			root TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			user_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("ensure projects table: %w", err)
	}
	for _, stmt := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_location_root ON projects(location_id, root)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_updated_at ON projects(updated_at)`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure projects index: %w", err)
		}
	}
	return nil
}
