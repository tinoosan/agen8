package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var _ domain.SessionRepository = (*PostgresSessionRepository)(nil)

type PostgresSessionRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewPostgresSessionRepository(handle *storagedb.Handle) (*PostgresSessionRepository, error) {
	if handle == nil || handle.DB() == nil || handle.Dialect() == nil {
		return nil, fmt.Errorf("harness postgres session repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("harness postgres session repository: storage driver must be postgres, got %q", handle.Driver())
	}
	if err := MigrateSessionSchema(context.Background(), handle.DB()); err != nil {
		return nil, err
	}
	return &PostgresSessionRepository{db: handle.DB(), dialect: handle.Dialect()}, nil
}

func (r *PostgresSessionRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

func (r *PostgresSessionRepository) Save(ctx context.Context, session *domain.Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}
	mcpServersJSON, err := json.Marshal(session.MCPServers)
	if err != nil {
		return fmt.Errorf("marshal harness session mcp servers: %w", err)
	}
	var deactivatedAt *string
	if session.DeactivatedAt != nil {
		s := session.DeactivatedAt.UTC().Format(time.RFC3339Nano)
		deactivatedAt = &s
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO harness_sessions (
			session_id, project_id, member_id, space_id, status, inactive_reason, inactive_error,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			location_id, channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id) DO UPDATE SET
			project_id = excluded.project_id,
			location_id = excluded.location_id,
			status = excluded.status,
			inactive_reason = excluded.inactive_reason,
			inactive_error = excluded.inactive_error,
			activated_at = excluded.activated_at,
			deactivated_at = excluded.deactivated_at,
			tokens_in = excluded.tokens_in,
			tokens_out = excluded.tokens_out,
			model = excluded.model,
			effort = excluded.effort,
			permission_mode = excluded.permission_mode,
			config_ref = excluded.config_ref,
			session_ref = excluded.session_ref,
			channel_id = excluded.channel_id,
			workdir = excluded.workdir,
			display_name = excluded.display_name,
			member_type = excluded.member_type,
			lifecycle_state = excluded.lifecycle_state,
			system_prompt = excluded.system_prompt,
			mcp_token = excluded.mcp_token,
			mcp_servers_json = excluded.mcp_servers_json,
			claude_channel_url = excluded.claude_channel_url`),
		session.ID,
		session.ProjectID,
		session.MemberID,
		session.SpaceID,
		string(session.Status),
		string(session.InactiveReason),
		session.InactiveError,
		session.ActivatedAt.UTC().Format(time.RFC3339Nano),
		deactivatedAt,
		session.TokensIn,
		session.TokensOut,
		session.Kind,
		session.Model,
		session.Effort,
		session.PermissionMode,
		session.ConfigRef,
		session.Ref,
		session.LocationID,
		session.ChannelID,
		session.Workdir,
		session.DisplayName,
		session.MemberType,
		session.LifecycleState,
		session.SystemPrompt,
		session.MCPToken,
		string(mcpServersJSON),
		session.ClaudeChannelURL,
	)
	if err != nil {
		return fmt.Errorf("save harness session %s: %w", session.ID, err)
	}
	return nil
}

func (r *PostgresSessionRepository) Get(ctx context.Context, sessionID string) (*domain.Session, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url
		FROM harness_sessions WHERE session_id = ?`), sessionID)
	return scanSession(row)
}

func (r *PostgresSessionRepository) GetActiveByMember(ctx context.Context, memberID string) (*domain.Session, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url
		FROM harness_sessions WHERE member_id = ? AND status = 'active'
		LIMIT 1`), memberID)
	return scanSession(row)
}

func (r *PostgresSessionRepository) ListActive(ctx context.Context) ([]*domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url
		FROM harness_sessions WHERE status = 'active'
		ORDER BY activated_at DESC`))
	if err != nil {
		return nil, fmt.Errorf("list active harness sessions: %w", err)
	}
	defer rows.Close()

	var out []*domain.Session
	for rows.Next() {
		s, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresSessionRepository) ListByMember(ctx context.Context, memberID string) ([]*domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url
		FROM harness_sessions WHERE member_id = ?
		ORDER BY activated_at DESC`), memberID)
	if err != nil {
		return nil, fmt.Errorf("list harness sessions by member %s: %w", memberID, err)
	}
	defer rows.Close()

	var out []*domain.Session
	for rows.Next() {
		s, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresSessionRepository) ListBySpace(ctx context.Context, spaceID string) ([]*domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url
		FROM harness_sessions WHERE space_id = ?
		ORDER BY activated_at DESC`), spaceID)
	if err != nil {
		return nil, fmt.Errorf("list harness sessions by space %s: %w", spaceID, err)
	}
	defer rows.Close()

	var out []*domain.Session
	for rows.Next() {
		s, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
