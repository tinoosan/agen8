package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/workspace"
)

type WorkspaceSQLiteRepository struct {
	db *sql.DB
}

func NewWorkspaceSQLiteRepository(db *sql.DB) (*WorkspaceSQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("project workspace sqlite repository: db is required")
	}
	repo := &WorkspaceSQLiteRepository{db: db}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *WorkspaceSQLiteRepository) Get(ctx context.Context, id string) (workspace.Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return workspace.Record{}, fmt.Errorf("workspace id is required")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT workspace_json FROM workspaces WHERE workspace_id = ?`, id).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workspace.Record{}, workspace.ErrNotFound
		}
		return workspace.Record{}, fmt.Errorf("get workspace %s: %w", id, err)
	}
	return unmarshalWorkspace(raw, id)
}

func (r *WorkspaceSQLiteRepository) List(ctx context.Context, filter workspace.Filter) ([]workspace.Record, error) {
	query, args := workspaceListQuery(filter, "?")
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()
	return scanWorkspaces(rows)
}

func (r *WorkspaceSQLiteRepository) Create(ctx context.Context, ws workspace.Record) error {
	return r.upsert(ctx, ws)
}

func (r *WorkspaceSQLiteRepository) Update(ctx context.Context, ws workspace.Record) error {
	return r.upsert(ctx, ws)
}

func (r *WorkspaceSQLiteRepository) upsert(ctx context.Context, ws workspace.Record) error {
	id, args, err := workspaceUpsertArgs(ws)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
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
			updated_at = excluded.updated_at`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("save workspace %s: %w", id, err)
	}
	return nil
}

func (r *WorkspaceSQLiteRepository) ensureSchema(ctx context.Context) error {
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

const workspaceCreateTableSQLite = `
	CREATE TABLE IF NOT EXISTS workspaces (
		workspace_id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL DEFAULT 'local',
		location_id TEXT NOT NULL DEFAULT 'local',
		root TEXT NOT NULL,
		machine TEXT NOT NULL DEFAULT '',
		lifecycle_state TEXT NOT NULL,
		workspace_json TEXT NOT NULL,
		linked_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		last_seen_at TEXT DEFAULT ''
	)`

var workspaceIndexStatements = []string{
	`CREATE INDEX IF NOT EXISTS idx_workspaces_project_state ON workspaces(project_id, lifecycle_state, updated_at DESC)`,
	`CREATE INDEX IF NOT EXISTS idx_workspaces_identity ON workspaces(project_id, location_id, root, machine)`,
}

func workspaceListQuery(filter workspace.Filter, placeholder string) (string, []any) {
	var clauses []string
	var args []any
	add := func(col, val string) {
		clauses = append(clauses, fmt.Sprintf("%s = %s", col, nextPlaceholder(placeholder, len(args)+1)))
		args = append(args, val)
	}
	if filter.ProjectID != "" {
		add("project_id", filter.ProjectID)
	}
	if filter.UserID != "" {
		add("user_id", filter.UserID)
	}
	if filter.LocationID != "" {
		add("location_id", filter.LocationID)
	}
	if filter.Root != "" {
		add("root", filter.Root)
	}
	if filter.Machine != "" {
		add("machine", filter.Machine)
	}
	if filter.LifecycleState != "" {
		add("lifecycle_state", filter.LifecycleState)
	}
	query := "SELECT workspace_json FROM workspaces"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY linked_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}
	return query, args
}

func workspaceUpsertArgs(ws workspace.Record) (string, []any, error) {
	id := strings.TrimSpace(string(ws.ID))
	if id == "" {
		return "", nil, fmt.Errorf("workspace id is required")
	}
	projectID := strings.TrimSpace(ws.ProjectID)
	if projectID == "" {
		return "", nil, fmt.Errorf("workspace project id is required")
	}
	root := strings.TrimSpace(ws.Root)
	if root == "" {
		return "", nil, fmt.Errorf("workspace root is required")
	}
	payload, err := json.Marshal(ws)
	if err != nil {
		return "", nil, fmt.Errorf("marshal workspace %s: %w", id, err)
	}
	userID := strings.TrimSpace(ws.UserID)
	if userID == "" {
		userID = "local"
	}
	locationID := strings.TrimSpace(ws.LocationID)
	if locationID == "" {
		locationID = "local"
	}
	args := []any{
		id,
		projectID,
		userID,
		locationID,
		root,
		strings.TrimSpace(ws.Machine),
		ws.LifecycleState,
		string(payload),
		ws.LinkedAt.UTC().Format(time.RFC3339Nano),
		ws.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	return id, args, nil
}

func unmarshalWorkspace(raw, id string) (workspace.Record, error) {
	var ws workspace.Record
	if err := json.Unmarshal([]byte(raw), &ws); err != nil {
		return workspace.Record{}, fmt.Errorf("unmarshal workspace %s: %w", id, err)
	}
	return ws, nil
}

func scanWorkspaces(rows *sql.Rows) ([]workspace.Record, error) {
	var out []workspace.Record
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		ws, err := unmarshalWorkspace(raw, "")
		if err != nil {
			return nil, err
		}
		out = append(out, ws)
	}
	return out, rows.Err()
}
