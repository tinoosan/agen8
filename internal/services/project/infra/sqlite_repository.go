package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
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
		SELECT project_id, location_id, root, title, status, created_at, updated_at
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
		SELECT project_id, location_id, root, title, status, created_at, updated_at
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
		INSERT INTO projects (project_id, location_id, root, title, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			location_id = excluded.location_id,
			root = excluded.root,
			title = excluded.title,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, record.ID, record.LocationID, record.Root, record.Title, record.Status, timeString(record.CreatedAt), timeString(record.UpdatedAt))
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

type SQLiteClusterRepository struct {
	db *sql.DB
}

func NewSQLiteClusterRepository(db *sql.DB) (*SQLiteClusterRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("project sqlite cluster repository: db is required")
	}
	repo := &SQLiteClusterRepository{db: db}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteClusterRepository) List(ctx context.Context, filter cluster.Filter) ([]cluster.Record, error) {
	where, args, err := clusterWhere(filter)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT cluster_id, project_id, name, status, created_at, updated_at
		FROM project_clusters`+where+`
		ORDER BY name ASC, cluster_id ASC
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("list project clusters: %w", err)
	}
	defer rows.Close()
	return scanClusters(rows)
}

func (r *SQLiteClusterRepository) ListSpaces(ctx context.Context, clusterID cluster.ID) ([]cluster.SpaceRefRecord, error) {
	clusterID = cluster.ID(strings.TrimSpace(string(clusterID)))
	if clusterID == "" {
		return nil, fmt.Errorf("cluster id is required")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT cluster_id, space_id, sort_order, pinned
		FROM project_cluster_spaces
		WHERE cluster_id = ?
		ORDER BY pinned DESC, sort_order ASC, space_id ASC
	`, clusterID)
	if err != nil {
		return nil, fmt.Errorf("list project cluster spaces: %w", err)
	}
	defer rows.Close()
	return scanClusterSpaces(rows)
}

func (r *SQLiteClusterRepository) Save(ctx context.Context, record cluster.Record) (cluster.Record, error) {
	record, err := validateCluster(record)
	if err != nil {
		return cluster.Record{}, err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO project_clusters (cluster_id, project_id, name, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(cluster_id) DO UPDATE SET
			project_id = excluded.project_id,
			name = excluded.name,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, record.ID, record.ProjectID, record.Name, record.Status, timeString(record.CreatedAt), timeString(record.UpdatedAt))
	if err != nil {
		return cluster.Record{}, fmt.Errorf("save project cluster %s: %w", record.ID, err)
	}
	return r.get(ctx, record.ID)
}

func (r *SQLiteClusterRepository) SaveSpace(ctx context.Context, ref cluster.SpaceRefRecord) (cluster.SpaceRefRecord, error) {
	ref, err := validateSpaceRef(ref)
	if err != nil {
		return cluster.SpaceRefRecord{}, err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO project_cluster_spaces (cluster_id, space_id, sort_order, pinned)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(cluster_id, space_id) DO UPDATE SET
			sort_order = excluded.sort_order,
			pinned = excluded.pinned
	`, ref.ClusterID, ref.SpaceID, ref.SortOrder, pinnedInt(ref.Pinned))
	if err != nil {
		return cluster.SpaceRefRecord{}, fmt.Errorf("save project cluster space %s/%s: %w", ref.ClusterID, ref.SpaceID, err)
	}
	return r.getSpace(ctx, ref.ClusterID, ref.SpaceID)
}

func (r *SQLiteClusterRepository) RemoveSpace(ctx context.Context, clusterID cluster.ID, spaceID spacedomain.SpaceID) error {
	clusterID = cluster.ID(strings.TrimSpace(string(clusterID)))
	spaceID = spacedomain.SpaceID(strings.TrimSpace(string(spaceID)))
	if clusterID == "" {
		return fmt.Errorf("cluster id is required")
	}
	if spaceID == "" {
		return fmt.Errorf("space id is required")
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM project_cluster_spaces WHERE cluster_id = ? AND space_id = ?`, clusterID, spaceID)
	if err != nil {
		return fmt.Errorf("remove project cluster space: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return cluster.ErrNotFound
	}
	return nil
}

func (r *SQLiteClusterRepository) get(ctx context.Context, id cluster.ID) (cluster.Record, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT cluster_id, project_id, name, status, created_at, updated_at
		FROM project_clusters
		WHERE cluster_id = ?
	`, id)
	record, err := scanCluster(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cluster.Record{}, cluster.ErrNotFound
		}
		return cluster.Record{}, err
	}
	return record, nil
}

func (r *SQLiteClusterRepository) getSpace(ctx context.Context, clusterID cluster.ID, spaceID spacedomain.SpaceID) (cluster.SpaceRefRecord, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT cluster_id, space_id, sort_order, pinned
		FROM project_cluster_spaces
		WHERE cluster_id = ? AND space_id = ?
	`, clusterID, spaceID)
	ref, err := scanClusterSpace(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return cluster.SpaceRefRecord{}, cluster.ErrNotFound
		}
		return cluster.SpaceRefRecord{}, err
	}
	return ref, nil
}

func (r *SQLiteClusterRepository) ensureSchema(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS project_clusters (
			cluster_id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS project_cluster_spaces (
			cluster_id TEXT NOT NULL,
			space_id TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			pinned INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY(cluster_id, space_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_project_clusters_project ON project_clusters(project_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_project_cluster_spaces_space ON project_cluster_spaces(space_id)`,
	} {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure project cluster schema: %w", err)
		}
	}
	return nil
}
