package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
)

type ProjectRegistryRecord struct {
	ProjectRoot  string
	ProjectID    string
	ManifestPath string
	Enabled      bool
	CreatedAt    string
	UpdatedAt    string
	Metadata     map[string]any
}

func normalizeProjectRegistryRecord(record ProjectRegistryRecord) ProjectRegistryRecord {
	record.ProjectRoot = strings.TrimSpace(record.ProjectRoot)
	record.ProjectID = strings.TrimSpace(record.ProjectID)
	record.ManifestPath = strings.TrimSpace(record.ManifestPath)
	record.CreatedAt = strings.TrimSpace(record.CreatedAt)
	record.UpdatedAt = strings.TrimSpace(record.UpdatedAt)
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	return record
}

func UpsertProjectRegistry(ctx context.Context, cfg config.Config, record ProjectRegistryRecord) (ProjectRegistryRecord, error) {
	if err := cfg.Validate(); err != nil {
		return ProjectRegistryRecord{}, err
	}
	record = normalizeProjectRegistryRecord(record)
	if record.ProjectRoot == "" {
		return ProjectRegistryRecord{}, fmt.Errorf("project root is required: %w", ErrInvalid)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if record.CreatedAt == "" {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	metadataJSON, err := json.Marshal(record.Metadata)
	if err != nil {
		return ProjectRegistryRecord{}, fmt.Errorf("marshal project registry metadata: %w", err)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return ProjectRegistryRecord{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO project_registry (
			project_root, project_id, manifest_path, enabled, created_at, updated_at, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_root) DO UPDATE SET
			project_id=excluded.project_id,
			manifest_path=excluded.manifest_path,
			enabled=excluded.enabled,
			updated_at=excluded.updated_at,
			metadata_json=excluded.metadata_json
	`,
		record.ProjectRoot,
		nullIfEmpty(record.ProjectID),
		nullIfEmpty(record.ManifestPath),
		boolToInt(record.Enabled),
		record.CreatedAt,
		record.UpdatedAt,
		string(metadataJSON),
	)
	if err != nil {
		return ProjectRegistryRecord{}, fmt.Errorf("upsert project registry: %w", err)
	}
	return LoadProjectRegistry(ctx, cfg, record.ProjectRoot)
}

func LoadProjectRegistry(ctx context.Context, cfg config.Config, projectRoot string) (ProjectRegistryRecord, error) {
	if err := cfg.Validate(); err != nil {
		return ProjectRegistryRecord{}, err
	}
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return ProjectRegistryRecord{}, fmt.Errorf("project root is required: %w", ErrInvalid)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return ProjectRegistryRecord{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		record       ProjectRegistryRecord
		projectID    sql.NullString
		manifestPath sql.NullString
		enabled      int
		createdAt    sql.NullString
		updatedAt    sql.NullString
		metadataJSON sql.NullString
	)
	err = db.QueryRowContext(ctx, `
		SELECT project_root, project_id, manifest_path, enabled, created_at, updated_at, metadata_json
		FROM project_registry
		WHERE project_root = ?
	`, projectRoot).Scan(
		&record.ProjectRoot,
		&projectID,
		&manifestPath,
		&enabled,
		&createdAt,
		&updatedAt,
		&metadataJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectRegistryRecord{}, fmt.Errorf("project registry %s not found: %w", projectRoot, ErrNotFound)
		}
		return ProjectRegistryRecord{}, fmt.Errorf("load project registry: %w", err)
	}
	record.ProjectID = strings.TrimSpace(projectID.String)
	record.ManifestPath = strings.TrimSpace(manifestPath.String)
	record.Enabled = enabled != 0
	record.CreatedAt = strings.TrimSpace(createdAt.String)
	record.UpdatedAt = strings.TrimSpace(updatedAt.String)
	if strings.TrimSpace(metadataJSON.String) != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &record.Metadata); err != nil {
			return ProjectRegistryRecord{}, fmt.Errorf("unmarshal project registry metadata: %w", err)
		}
	}
	return normalizeProjectRegistryRecord(record), nil
}

func ListProjectRegistry(ctx context.Context, cfg config.Config) ([]ProjectRegistryRecord, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := db.QueryContext(ctx, `
		SELECT project_root, project_id, manifest_path, enabled, created_at, updated_at, metadata_json
		FROM project_registry
		ORDER BY updated_at DESC, project_root ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list project registry: %w", err)
	}
	defer rows.Close()
	out := []ProjectRegistryRecord{}
	for rows.Next() {
		var (
			record       ProjectRegistryRecord
			projectID    sql.NullString
			manifestPath sql.NullString
			enabled      int
			createdAt    sql.NullString
			updatedAt    sql.NullString
			metadataJSON sql.NullString
		)
		if err := rows.Scan(
			&record.ProjectRoot,
			&projectID,
			&manifestPath,
			&enabled,
			&createdAt,
			&updatedAt,
			&metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("scan project registry: %w", err)
		}
		record.ProjectID = strings.TrimSpace(projectID.String)
		record.ManifestPath = strings.TrimSpace(manifestPath.String)
		record.Enabled = enabled != 0
		record.CreatedAt = strings.TrimSpace(createdAt.String)
		record.UpdatedAt = strings.TrimSpace(updatedAt.String)
		if strings.TrimSpace(metadataJSON.String) != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &record.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal project registry metadata: %w", err)
			}
		}
		out = append(out, normalizeProjectRegistryRecord(record))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project registry: %w", err)
	}
	return out, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
