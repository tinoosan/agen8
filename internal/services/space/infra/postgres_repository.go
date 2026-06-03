package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Get(ctx context.Context, id domain.SpaceID) (domain.SpaceRecord, error) {
	spaceID := strings.TrimSpace(string(id))
	if spaceID == "" {
		return domain.SpaceRecord{}, fmt.Errorf("space id is required")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT space_json FROM spaces WHERE space_id = $1`, spaceID).Scan(&raw)
	if err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("get space %s: %w", spaceID, err)
	}
	var space domain.SpaceRecord
	if err := json.Unmarshal([]byte(raw), &space); err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("unmarshal space %s: %w", spaceID, err)
	}
	return space, nil
}

func (r *PostgresRepository) List(ctx context.Context, filter domain.SpaceFilter) ([]domain.SpaceRecord, error) {
	var clauses []string
	var args []any
	argN := 1
	if filter.SpaceID != "" {
		clauses = append(clauses, fmt.Sprintf("space_id = $%d", argN))
		args = append(args, filter.SpaceID)
		argN++
	}
	if filter.ProjectID != "" {
		clauses = append(clauses, fmt.Sprintf("project_id = $%d", argN))
		args = append(args, filter.ProjectID)
		argN++
	}
	if filter.UserID != "" {
		clauses = append(clauses, fmt.Sprintf("user_id = $%d", argN))
		args = append(args, filter.UserID)
		argN++
	}
	if filter.Status != "" {
		clauses = append(clauses, fmt.Sprintf("status = $%d", argN))
		args = append(args, filter.Status)
		argN++
	}
	query := "SELECT space_json FROM spaces"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY created_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list spaces: %w", err)
	}
	defer rows.Close()
	var spaces []domain.SpaceRecord
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan space: %w", err)
		}
		var space domain.SpaceRecord
		if err := json.Unmarshal([]byte(raw), &space); err != nil {
			return nil, fmt.Errorf("unmarshal space: %w", err)
		}
		spaces = append(spaces, space)
	}
	return spaces, rows.Err()
}

func (r *PostgresRepository) Create(ctx context.Context, space domain.SpaceRecord) error {
	return r.upsert(ctx, space)
}

func (r *PostgresRepository) Update(ctx context.Context, space domain.SpaceRecord) error {
	return r.upsert(ctx, space)
}

func (r *PostgresRepository) Delete(ctx context.Context, id domain.SpaceID) error {
	spaceID := strings.TrimSpace(string(id))
	if spaceID == "" {
		return fmt.Errorf("space id is required")
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM spaces WHERE space_id = $1`, spaceID)
	return err
}

func (r *PostgresRepository) upsert(ctx context.Context, space domain.SpaceRecord) error {
	spaceID := strings.TrimSpace(string(space.ID))
	if spaceID == "" {
		return fmt.Errorf("space id is required")
	}
	payload, err := json.Marshal(space)
	if err != nil {
		return fmt.Errorf("marshal space: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO spaces (space_id, user_id, project_id, status, title, plan_mode, space_json, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(space_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			project_id = EXCLUDED.project_id,
			status = EXCLUDED.status,
			title = EXCLUDED.title,
			plan_mode = EXCLUDED.plan_mode,
			space_json = EXCLUDED.space_json,
			created_at = COALESCE(spaces.created_at, EXCLUDED.created_at),
			updated_at = EXCLUDED.updated_at`,
		spaceID,
		space.UserID,
		strings.TrimSpace(string(space.ProjectID)),
		space.Status,
		space.Title,
		space.PlanMode,
		string(payload),
		space.CreatedAt.UTC().Format(time.RFC3339Nano),
		space.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}
