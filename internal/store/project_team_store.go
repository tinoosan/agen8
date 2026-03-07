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

const (
	ProjectTeamStatusRegistered = "registered"
	ProjectTeamStatusActive     = "active"
	ProjectTeamStatusInactive   = "inactive"
)

// ProjectTeamRecord stores canonical project-owned team metadata.
type ProjectTeamRecord struct {
	ProjectRoot      string
	ProjectID        string
	TeamID           string
	ProfileID        string
	PrimarySessionID string
	CoordinatorRunID string
	Status           string
	CreatedAt        string
	UpdatedAt        string
	Metadata         map[string]any
}

func normalizeProjectTeamRecord(record ProjectTeamRecord) ProjectTeamRecord {
	record.ProjectRoot = strings.TrimSpace(record.ProjectRoot)
	record.ProjectID = strings.TrimSpace(record.ProjectID)
	record.TeamID = strings.TrimSpace(record.TeamID)
	record.ProfileID = strings.TrimSpace(record.ProfileID)
	record.PrimarySessionID = strings.TrimSpace(record.PrimarySessionID)
	record.CoordinatorRunID = strings.TrimSpace(record.CoordinatorRunID)
	record.Status = strings.TrimSpace(record.Status)
	if record.Status == "" {
		record.Status = ProjectTeamStatusRegistered
	}
	record.CreatedAt = strings.TrimSpace(record.CreatedAt)
	record.UpdatedAt = strings.TrimSpace(record.UpdatedAt)
	if record.Metadata == nil {
		record.Metadata = map[string]any{}
	}
	return record
}

func UpsertProjectTeam(ctx context.Context, cfg config.Config, record ProjectTeamRecord) (ProjectTeamRecord, error) {
	if err := cfg.Validate(); err != nil {
		return ProjectTeamRecord{}, err
	}
	record = normalizeProjectTeamRecord(record)
	if record.ProjectRoot == "" {
		return ProjectTeamRecord{}, fmt.Errorf("project root is required: %w", ErrInvalid)
	}
	if record.TeamID == "" {
		return ProjectTeamRecord{}, fmt.Errorf("team id is required: %w", ErrInvalid)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if record.CreatedAt == "" {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	metadataJSON, err := json.Marshal(record.Metadata)
	if err != nil {
		return ProjectTeamRecord{}, fmt.Errorf("marshal project team metadata: %w", err)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return ProjectTeamRecord{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO project_teams (
			project_root, project_id, team_id, profile_id, primary_session_id, coordinator_run_id, status, created_at, updated_at, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_root, team_id) DO UPDATE SET
			project_id=excluded.project_id,
			profile_id=excluded.profile_id,
			primary_session_id=excluded.primary_session_id,
			coordinator_run_id=excluded.coordinator_run_id,
			status=excluded.status,
			updated_at=excluded.updated_at,
			metadata_json=excluded.metadata_json
	`,
		record.ProjectRoot,
		nullIfEmpty(record.ProjectID),
		record.TeamID,
		nullIfEmpty(record.ProfileID),
		nullIfEmpty(record.PrimarySessionID),
		nullIfEmpty(record.CoordinatorRunID),
		record.Status,
		record.CreatedAt,
		record.UpdatedAt,
		string(metadataJSON),
	)
	if err != nil {
		return ProjectTeamRecord{}, fmt.Errorf("upsert project team: %w", err)
	}
	return LoadProjectTeam(ctx, cfg, record.ProjectRoot, record.TeamID)
}

func LoadProjectTeam(ctx context.Context, cfg config.Config, projectRoot, teamID string) (ProjectTeamRecord, error) {
	if err := cfg.Validate(); err != nil {
		return ProjectTeamRecord{}, err
	}
	projectRoot = strings.TrimSpace(projectRoot)
	teamID = strings.TrimSpace(teamID)
	if projectRoot == "" || teamID == "" {
		return ProjectTeamRecord{}, fmt.Errorf("project root and team id are required: %w", ErrInvalid)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return ProjectTeamRecord{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var (
		record       ProjectTeamRecord
		projectID    sql.NullString
		profileID    sql.NullString
		sessionID    sql.NullString
		coordinator  sql.NullString
		status       sql.NullString
		createdAt    sql.NullString
		updatedAt    sql.NullString
		metadataJSON sql.NullString
	)
	err = db.QueryRowContext(ctx, `
		SELECT project_root, project_id, team_id, profile_id, primary_session_id, coordinator_run_id, status, created_at, updated_at, metadata_json
		FROM project_teams
		WHERE project_root = ? AND team_id = ?
	`, projectRoot, teamID).Scan(
		&record.ProjectRoot,
		&projectID,
		&record.TeamID,
		&profileID,
		&sessionID,
		&coordinator,
		&status,
		&createdAt,
		&updatedAt,
		&metadataJSON,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectTeamRecord{}, fmt.Errorf("project team %s/%s not found: %w", projectRoot, teamID, ErrNotFound)
		}
		return ProjectTeamRecord{}, fmt.Errorf("load project team: %w", err)
	}
	record.ProjectID = strings.TrimSpace(projectID.String)
	record.ProfileID = strings.TrimSpace(profileID.String)
	record.PrimarySessionID = strings.TrimSpace(sessionID.String)
	record.CoordinatorRunID = strings.TrimSpace(coordinator.String)
	record.Status = strings.TrimSpace(status.String)
	record.CreatedAt = strings.TrimSpace(createdAt.String)
	record.UpdatedAt = strings.TrimSpace(updatedAt.String)
	if strings.TrimSpace(metadataJSON.String) != "" {
		if err := json.Unmarshal([]byte(metadataJSON.String), &record.Metadata); err != nil {
			return ProjectTeamRecord{}, fmt.Errorf("unmarshal project team metadata: %w", err)
		}
	}
	return normalizeProjectTeamRecord(record), nil
}

func ListProjectTeams(ctx context.Context, cfg config.Config, projectRoot string) ([]ProjectTeamRecord, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil, fmt.Errorf("project root is required: %w", ErrInvalid)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rows, err := db.QueryContext(ctx, `
		SELECT project_root, project_id, team_id, profile_id, primary_session_id, coordinator_run_id, status, created_at, updated_at, metadata_json
		FROM project_teams
		WHERE project_root = ?
		ORDER BY updated_at DESC, team_id ASC
	`, projectRoot)
	if err != nil {
		return nil, fmt.Errorf("list project teams: %w", err)
	}
	defer rows.Close()
	out := []ProjectTeamRecord{}
	for rows.Next() {
		var (
			record       ProjectTeamRecord
			projectID    sql.NullString
			profileID    sql.NullString
			sessionID    sql.NullString
			coordinator  sql.NullString
			status       sql.NullString
			createdAt    sql.NullString
			updatedAt    sql.NullString
			metadataJSON sql.NullString
		)
		if err := rows.Scan(
			&record.ProjectRoot,
			&projectID,
			&record.TeamID,
			&profileID,
			&sessionID,
			&coordinator,
			&status,
			&createdAt,
			&updatedAt,
			&metadataJSON,
		); err != nil {
			return nil, fmt.Errorf("scan project team: %w", err)
		}
		record.ProjectID = strings.TrimSpace(projectID.String)
		record.ProfileID = strings.TrimSpace(profileID.String)
		record.PrimarySessionID = strings.TrimSpace(sessionID.String)
		record.CoordinatorRunID = strings.TrimSpace(coordinator.String)
		record.Status = strings.TrimSpace(status.String)
		record.CreatedAt = strings.TrimSpace(createdAt.String)
		record.UpdatedAt = strings.TrimSpace(updatedAt.String)
		if strings.TrimSpace(metadataJSON.String) != "" {
			if err := json.Unmarshal([]byte(metadataJSON.String), &record.Metadata); err != nil {
				return nil, fmt.Errorf("unmarshal project team metadata: %w", err)
			}
		}
		out = append(out, normalizeProjectTeamRecord(record))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project teams: %w", err)
	}
	return out, nil
}

func DeleteProjectTeam(ctx context.Context, cfg config.Config, projectRoot, teamID string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	projectRoot = strings.TrimSpace(projectRoot)
	teamID = strings.TrimSpace(teamID)
	if projectRoot == "" || teamID == "" {
		return fmt.Errorf("project root and team id are required: %w", ErrInvalid)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err = db.ExecContext(ctx, `DELETE FROM project_teams WHERE project_root = ? AND team_id = ?`, projectRoot, teamID)
	if err != nil {
		return fmt.Errorf("delete project team: %w", err)
	}
	return nil
}

func UpdateProjectTeamStatus(ctx context.Context, cfg config.Config, projectRoot, teamID, status string, metadata map[string]any) (ProjectTeamRecord, error) {
	record, err := LoadProjectTeam(ctx, cfg, projectRoot, teamID)
	if err != nil {
		return ProjectTeamRecord{}, err
	}
	record.Status = strings.TrimSpace(status)
	if metadata != nil {
		if record.Metadata == nil {
			record.Metadata = map[string]any{}
		}
		for k, v := range metadata {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			record.Metadata[k] = v
		}
	}
	return UpsertProjectTeam(ctx, cfg, record)
}

func DeleteTeamScopedData(ctx context.Context, cfg config.Config, teamID string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return fmt.Errorf("team id is required: %w", ErrInvalid)
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	statements := []string{
		`DELETE FROM artifacts WHERE team_id = ?`,
		`DELETE FROM messages WHERE team_id = ?`,
		`DELETE FROM tasks WHERE team_id = ?`,
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt, teamID); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "no such table") {
				continue
			}
			return fmt.Errorf("delete team scoped data: %w", err)
		}
	}
	return nil
}
