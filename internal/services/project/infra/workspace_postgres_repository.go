package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/services/project/domain/workspace"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type WorkspacePostgresRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewWorkspacePostgresRepository(handle *storagedb.Handle) (*WorkspacePostgresRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("project workspace postgres repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("project workspace postgres repository: storage driver must be postgres, got %q", handle.Driver())
	}
	repo := &WorkspacePostgresRepository{db: handle.DB(), dialect: handle.Dialect()}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *WorkspacePostgresRepository) Get(ctx context.Context, id string) (workspace.Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return workspace.Record{}, fmt.Errorf("workspace id is required")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, r.rebind(`SELECT workspace_json FROM workspaces WHERE workspace_id = ?`), id).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workspace.Record{}, workspace.ErrNotFound
		}
		return workspace.Record{}, fmt.Errorf("get workspace %s: %w", id, err)
	}
	return unmarshalWorkspace(raw, id)
}

func (r *WorkspacePostgresRepository) List(ctx context.Context, filter workspace.Filter) ([]workspace.Record, error) {
	query, args := workspaceListQuery(filter, "?")
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()
	return scanWorkspaces(rows)
}

func (r *WorkspacePostgresRepository) Create(ctx context.Context, ws workspace.Record) error {
	return r.upsert(ctx, ws)
}

func (r *WorkspacePostgresRepository) Update(ctx context.Context, ws workspace.Record) error {
	return r.upsert(ctx, ws)
}

func (r *WorkspacePostgresRepository) upsert(ctx context.Context, ws workspace.Record) error {
	id, args, err := workspaceUpsertArgs(ws)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO workspaces (workspace_id, project_id, user_id, location_id, root, machine, lifecycle_state, workspace_json, linked_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			project_id = excluded.project_id,
			user_id = excluded.user_id,
			location_id = excluded.location_id,
			root = excluded.root,
			machine = excluded.machine,
			lifecycle_state = excluded.lifecycle_state,
			workspace_json = excluded.workspace_json,
			linked_at = COALESCE(workspaces.linked_at, excluded.linked_at),
			updated_at = excluded.updated_at`),
		args...,
	)
	if err != nil {
		return fmt.Errorf("save workspace %s: %w", id, err)
	}
	return nil
}

func (r *WorkspacePostgresRepository) ensureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, workspaceCreateTableSQLite); err != nil {
		return fmt.Errorf("ensure workspaces table: %w", err)
	}
	for _, stmt := range workspaceIndexStatements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure workspaces index: %w", err)
		}
	}
	return nil
}

func (r *WorkspacePostgresRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}
