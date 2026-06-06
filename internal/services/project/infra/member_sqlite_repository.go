package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

type MemberSQLiteRepository struct {
	db *sql.DB
}

func NewMemberSQLiteRepository(db *sql.DB) (*MemberSQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("project member sqlite repository: db is required")
	}
	repo := &MemberSQLiteRepository{db: db}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *MemberSQLiteRepository) Get(ctx context.Context, id string) (member.Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return member.Record{}, fmt.Errorf("member id is required")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT member_json FROM members WHERE member_id = ?`, id).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return member.Record{}, member.ErrNotFound
		}
		return member.Record{}, fmt.Errorf("get member %s: %w", id, err)
	}
	return unmarshalMember(raw, id)
}

func (r *MemberSQLiteRepository) List(ctx context.Context, filter member.Filter) ([]member.Record, error) {
	query, args := memberListQuery(filter, "?")
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	return scanMembers(rows)
}

func (r *MemberSQLiteRepository) Create(ctx context.Context, rosterMember member.Record) error {
	return r.upsert(ctx, rosterMember)
}

func (r *MemberSQLiteRepository) Update(ctx context.Context, rosterMember member.Record) error {
	return r.upsert(ctx, rosterMember)
}

func (r *MemberSQLiteRepository) upsert(ctx context.Context, rosterMember member.Record) error {
	cols, args, err := memberUpsertArgs(rosterMember)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO members (member_id, project_id, user_id, native_session_ref, member_type, lifecycle_state, harness_kind, member_json, registered_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(member_id) DO UPDATE SET
			project_id = excluded.project_id,
			user_id = excluded.user_id,
			native_session_ref = excluded.native_session_ref,
			member_type = excluded.member_type,
			lifecycle_state = excluded.lifecycle_state,
			harness_kind = excluded.harness_kind,
			member_json = excluded.member_json,
			registered_at = COALESCE(members.registered_at, excluded.registered_at),
			updated_at = excluded.updated_at`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("save member %s: %w", cols, err)
	}
	return nil
}

func (r *MemberSQLiteRepository) ensureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, memberCreateTableSQLite); err != nil {
		return fmt.Errorf("ensure members table: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_members_one_active_coordinator`); err != nil {
		return fmt.Errorf("drop stale coordinator index: %w", err)
	}
	for _, stmt := range memberIndexStatements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure members index: %w", err)
		}
	}
	return nil
}

const memberCreateTableSQLite = `
	CREATE TABLE IF NOT EXISTS members (
		member_id TEXT PRIMARY KEY,
		project_id TEXT NOT NULL,
		user_id TEXT NOT NULL DEFAULT 'local',
		native_session_ref TEXT NOT NULL DEFAULT '',
		member_type TEXT NOT NULL,
		lifecycle_state TEXT NOT NULL,
		harness_kind TEXT NOT NULL DEFAULT '',
		member_json TEXT NOT NULL,
		registered_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		last_seen_at TEXT DEFAULT ''
	)`

var memberIndexStatements = []string{
	`CREATE INDEX IF NOT EXISTS idx_members_project_state ON members(project_id, lifecycle_state, updated_at DESC)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_members_native_session ON members(project_id, harness_kind, native_session_ref) WHERE native_session_ref <> ''`,
}

func memberListQuery(filter member.Filter, placeholder string) (string, []any) {
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
	if filter.HarnessKind != "" {
		add("harness_kind", filter.HarnessKind)
	}
	if filter.NativeSessionRef != "" {
		add("native_session_ref", filter.NativeSessionRef)
	}
	if filter.MemberType != "" {
		add("member_type", filter.MemberType)
	}
	if filter.LifecycleState != "" {
		add("lifecycle_state", filter.LifecycleState)
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
	return query, args
}

func nextPlaceholder(placeholder string, n int) string {
	if placeholder == "?" {
		return "?"
	}
	return fmt.Sprintf("$%d", n)
}

func memberUpsertArgs(rosterMember member.Record) (string, []any, error) {
	memberID := strings.TrimSpace(string(rosterMember.ID))
	if memberID == "" {
		return "", nil, fmt.Errorf("member id is required")
	}
	projectID := strings.TrimSpace(rosterMember.ProjectID)
	if projectID == "" {
		return "", nil, fmt.Errorf("member project id is required")
	}
	payload, err := json.Marshal(rosterMember)
	if err != nil {
		return "", nil, fmt.Errorf("marshal member %s: %w", memberID, err)
	}
	userID := strings.TrimSpace(rosterMember.UserID)
	if userID == "" {
		userID = "local"
	}
	args := []any{
		memberID,
		projectID,
		userID,
		strings.TrimSpace(rosterMember.NativeSessionRef),
		rosterMember.MemberType,
		rosterMember.LifecycleState,
		strings.TrimSpace(rosterMember.HarnessKind),
		string(payload),
		rosterMember.RegisteredAt.UTC().Format(time.RFC3339Nano),
		rosterMember.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	return memberID, args, nil
}

func unmarshalMember(raw, id string) (member.Record, error) {
	var rosterMember member.Record
	if err := json.Unmarshal([]byte(raw), &rosterMember); err != nil {
		return member.Record{}, fmt.Errorf("unmarshal member %s: %w", id, err)
	}
	return rosterMember, nil
}

func scanMembers(rows *sql.Rows) ([]member.Record, error) {
	var members []member.Record
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		rosterMember, err := unmarshalMember(raw, "")
		if err != nil {
			return nil, err
		}
		members = append(members, rosterMember)
	}
	return members, rows.Err()
}
