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

type MemberSQLiteRepository struct {
	db *sql.DB
}

func NewMemberSQLiteRepository(db *sql.DB) *MemberSQLiteRepository {
	return &MemberSQLiteRepository{db: db}
}

func (r *MemberSQLiteRepository) Get(ctx context.Context, id string) (member.Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return member.Record{}, fmt.Errorf("member id is required")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT member_json FROM members WHERE member_id = ?`, id).Scan(&raw)
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

func (r *MemberSQLiteRepository) List(ctx context.Context, filter member.Filter) ([]member.Record, error) {
	var clauses []string
	var args []any
	if filter.SpaceID != "" {
		clauses = append(clauses, "space_id = ?")
		args = append(args, filter.SpaceID)
	}
	if filter.ProjectID != "" {
		clauses = append(clauses, "project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.UserID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, filter.UserID)
	}
	if filter.MemberType != "" {
		clauses = append(clauses, "member_type = ?")
		args = append(args, filter.MemberType)
	}
	if filter.LifecycleState != "" {
		clauses = append(clauses, "lifecycle_state = ?")
		args = append(args, filter.LifecycleState)
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
		var member member.Record
		if err := json.Unmarshal([]byte(raw), &member); err != nil {
			return nil, fmt.Errorf("unmarshal member: %w", err)
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (r *MemberSQLiteRepository) Create(ctx context.Context, rosterMember member.Record) error {
	return r.upsert(ctx, rosterMember)
}

func (r *MemberSQLiteRepository) Update(ctx context.Context, rosterMember member.Record) error {
	return r.upsert(ctx, rosterMember)
}

func (r *MemberSQLiteRepository) upsert(ctx context.Context, rosterMember member.Record) error {
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
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(member_id) DO UPDATE SET
			space_id = excluded.space_id,
			project_id = excluded.project_id,
			user_id = excluded.user_id,
			member_type = excluded.member_type,
			lifecycle_state = excluded.lifecycle_state,
			member_json = excluded.member_json,
			registered_at = COALESCE(members.registered_at, excluded.registered_at),
			updated_at = excluded.updated_at`,
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
