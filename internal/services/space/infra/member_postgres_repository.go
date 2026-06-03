package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type MemberPostgresRepository struct {
	db *sql.DB
}

func NewMemberPostgresRepository(db *sql.DB) *MemberPostgresRepository {
	return &MemberPostgresRepository{db: db}
}

func (r *MemberPostgresRepository) Get(ctx context.Context, id string) (member.Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return member.Record{}, fmt.Errorf("member id is required")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT member_json FROM members WHERE member_id = $1`, id).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return member.Record{}, member.ErrNotFound
		}
		return member.Record{}, fmt.Errorf("get member %s: %w", id, err)
	}
	var rosterMember member.Record
	if err := json.Unmarshal([]byte(raw), &rosterMember); err != nil {
		return member.Record{}, fmt.Errorf("unmarshal member %s: %w", id, err)
	}
	return rosterMember, nil
}

func (r *MemberPostgresRepository) List(ctx context.Context, filter member.Filter) ([]member.Record, error) {
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
	if filter.MemberType != "" {
		clauses = append(clauses, fmt.Sprintf("member_type = $%d", argN))
		args = append(args, filter.MemberType)
		argN++
	}
	if filter.LifecycleState != "" {
		clauses = append(clauses, fmt.Sprintf("lifecycle_state = $%d", argN))
		args = append(args, filter.LifecycleState)
		argN++
	}
	query := "SELECT member_json FROM members"
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY registered_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var members []member.Record
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		var rosterMember member.Record
		if err := json.Unmarshal([]byte(raw), &rosterMember); err != nil {
			return nil, fmt.Errorf("unmarshal member: %w", err)
		}
		members = append(members, rosterMember)
	}
	return members, rows.Err()
}

func (r *MemberPostgresRepository) Create(ctx context.Context, rosterMember member.Record) error {
	return r.upsert(ctx, rosterMember)
}

func (r *MemberPostgresRepository) Update(ctx context.Context, rosterMember member.Record) error {
	return r.upsert(ctx, rosterMember)
}

func (r *MemberPostgresRepository) upsert(ctx context.Context, rosterMember member.Record) error {
	memberID := strings.TrimSpace(string(rosterMember.ID))
	if memberID == "" {
		return fmt.Errorf("member id is required")
	}
	payload, err := json.Marshal(rosterMember)
	if err != nil {
		return fmt.Errorf("marshal member: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO members (member_id, space_id, project_id, user_id, member_type, lifecycle_state, member_json, registered_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT(member_id) DO UPDATE SET
			space_id = EXCLUDED.space_id,
			project_id = EXCLUDED.project_id,
			user_id = EXCLUDED.user_id,
			member_type = EXCLUDED.member_type,
			lifecycle_state = EXCLUDED.lifecycle_state,
			member_json = EXCLUDED.member_json,
			registered_at = COALESCE(members.registered_at, EXCLUDED.registered_at),
			updated_at = EXCLUDED.updated_at`,
		memberID,
		strings.TrimSpace(string(rosterMember.SpaceID)),
		strings.TrimSpace(string(rosterMember.ProjectID)),
		rosterMember.UserID,
		rosterMember.MemberType,
		rosterMember.LifecycleState,
		string(payload),
		rosterMember.RegisteredAt.UTC().Format(time.RFC3339Nano),
		rosterMember.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}
