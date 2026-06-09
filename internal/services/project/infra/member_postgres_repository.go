package infra

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type MemberPostgresRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewMemberPostgresRepository(handle *storagedb.Handle) (*MemberPostgresRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("project member postgres repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("project member postgres repository: storage driver must be postgres, got %q", handle.Driver())
	}
	repo := &MemberPostgresRepository{db: handle.DB(), dialect: handle.Dialect()}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *MemberPostgresRepository) Get(ctx context.Context, id string) (member.Record, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return member.Record{}, fmt.Errorf("member id is required")
	}
	var raw string
	err := r.db.QueryRowContext(ctx, r.rebind(`SELECT member_json FROM members WHERE member_id = ?`), id).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return member.Record{}, member.ErrNotFound
		}
		return member.Record{}, fmt.Errorf("get member %s: %w", id, err)
	}
	return unmarshalMember(raw, id)
}

func (r *MemberPostgresRepository) List(ctx context.Context, filter member.Filter) ([]member.Record, error) {
	query, args := memberListQuery(filter, "?")
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	return scanMembers(rows)
}

func (r *MemberPostgresRepository) Create(ctx context.Context, rosterMember member.Record) error {
	return r.upsert(ctx, rosterMember)
}

func (r *MemberPostgresRepository) Update(ctx context.Context, rosterMember member.Record) error {
	return r.upsert(ctx, rosterMember)
}

func (r *MemberPostgresRepository) upsert(ctx context.Context, rosterMember member.Record) error {
	memberID, args, err := memberUpsertArgs(rosterMember)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
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
			updated_at = excluded.updated_at`),
		args...,
	)
	if err != nil {
		return fmt.Errorf("save member %s: %w", memberID, err)
	}
	return nil
}

func (r *MemberPostgresRepository) ensureSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, memberCreateTableSQLite); err != nil {
		return fmt.Errorf("ensure members table: %w", err)
	}
	for _, stmt := range memberIndexStatements {
		if _, err := r.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("ensure members index: %w", err)
		}
	}
	return nil
}

func (r *MemberPostgresRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}
