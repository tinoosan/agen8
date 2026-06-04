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

var _ domain.SessionRepository = (*SQLiteSessionRepository)(nil)

type SQLiteSessionRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewSQLiteSessionRepository(handle *storagedb.Handle) (*SQLiteSessionRepository, error) {
	if handle == nil || handle.DB() == nil || handle.Dialect() == nil {
		return nil, fmt.Errorf("harness session db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("harness sqlite session repository: storage driver must be sqlite, got %q", handle.Driver())
	}
	repo := &SQLiteSessionRepository{db: handle.DB(), dialect: handle.Dialect()}
	if err := MigrateSessionSchema(context.Background(), handle.DB()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteSessionRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

func MigrateSessionSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("harness session migration: db is nil")
	}
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS harness_sessions (
			session_id     TEXT PRIMARY KEY,
			project_id     TEXT NOT NULL DEFAULT '',
			location_id    TEXT NOT NULL DEFAULT 'local',
			member_id      TEXT NOT NULL,
			space_id       TEXT NOT NULL,
			status         TEXT NOT NULL DEFAULT 'active',
			inactive_reason TEXT NOT NULL DEFAULT '',
			inactive_error TEXT NOT NULL DEFAULT '',
			activated_at   TEXT NOT NULL,
			deactivated_at TEXT,
			tokens_in      INTEGER NOT NULL DEFAULT 0,
			tokens_out     INTEGER NOT NULL DEFAULT 0,
			harness_kind   TEXT NOT NULL,
			model          TEXT NOT NULL,
			effort         TEXT NOT NULL,
			permission_mode TEXT NOT NULL DEFAULT '',
			config_ref     TEXT NOT NULL DEFAULT '',
			session_ref    TEXT NOT NULL DEFAULT '',
			channel_id     TEXT NOT NULL DEFAULT '',
			workdir        TEXT NOT NULL DEFAULT '',
			display_name   TEXT NOT NULL DEFAULT '',
			member_type    TEXT NOT NULL DEFAULT '',
			lifecycle_state TEXT NOT NULL DEFAULT '',
			system_prompt  TEXT NOT NULL DEFAULT '',
			mcp_token      TEXT NOT NULL DEFAULT '',
			mcp_servers_json TEXT NOT NULL DEFAULT '[]',
			claude_channel_url TEXT NOT NULL DEFAULT '',
			claude_channel_instance_id TEXT NOT NULL DEFAULT '',
			claude_channel_started_at TEXT
		)`)
	if err != nil {
		return fmt.Errorf("harness session migration: create table: %w", err)
	}
	if err := ensureSessionColumns(ctx, db); err != nil {
		return err
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_harness_sessions_member_status ON harness_sessions (member_id, status)`,
		`CREATE INDEX IF NOT EXISTS idx_harness_sessions_space ON harness_sessions (space_id)`,
	} {
		if _, err := db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("harness session migration: index: %w", err)
		}
	}
	return nil
}

func ensureSessionColumns(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT * FROM harness_sessions LIMIT 0`)
	if err != nil {
		return fmt.Errorf("harness session migration: inspect columns: %w", err)
	}
	columns, err := rows.Columns()
	closeErr := rows.Close()
	if err != nil {
		return fmt.Errorf("harness session migration: list columns: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("harness session migration: close column inspection: %w", closeErr)
	}
	existing := map[string]bool{}
	for _, column := range columns {
		existing[column] = true
	}
	additions := map[string]string{
		"project_id":                 "TEXT NOT NULL DEFAULT ''",
		"location_id":                "TEXT NOT NULL DEFAULT 'local'",
		"channel_id":                 "TEXT NOT NULL DEFAULT ''",
		"workdir":                    "TEXT NOT NULL DEFAULT ''",
		"display_name":               "TEXT NOT NULL DEFAULT ''",
		"member_type":                "TEXT NOT NULL DEFAULT ''",
		"lifecycle_state":            "TEXT NOT NULL DEFAULT ''",
		"system_prompt":              "TEXT NOT NULL DEFAULT ''",
		"mcp_token":                  "TEXT NOT NULL DEFAULT ''",
		"mcp_servers_json":           "TEXT NOT NULL DEFAULT '[]'",
		"claude_channel_url":         "TEXT NOT NULL DEFAULT ''",
		"claude_channel_instance_id": "TEXT NOT NULL DEFAULT ''",
		"claude_channel_started_at":  "TEXT",
		"permission_mode":            "TEXT NOT NULL DEFAULT ''",
		"config_ref":                 "TEXT NOT NULL DEFAULT ''",
	}
	for column, ddl := range additions {
		if existing[column] {
			continue
		}
		if _, err := db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE harness_sessions ADD COLUMN %s %s", column, ddl)); err != nil {
			return fmt.Errorf("harness session migration: add column %s: %w", column, err)
		}
	}
	return nil
}

func (r *SQLiteSessionRepository) Save(ctx context.Context, session *domain.Session) error {
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
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url,
			claude_channel_instance_id, claude_channel_started_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
			claude_channel_url = excluded.claude_channel_url,
			claude_channel_instance_id = excluded.claude_channel_instance_id,
			claude_channel_started_at = excluded.claude_channel_started_at`),
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
		session.ClaudeChannelInstanceID,
		formatOptionalTime(session.ClaudeChannelStartedAt),
	)
	if err != nil {
		return fmt.Errorf("save harness session %s: %w", session.ID, err)
	}
	return nil
}

func (r *SQLiteSessionRepository) Get(ctx context.Context, sessionRef string) (*domain.Session, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url,
			claude_channel_instance_id, claude_channel_started_at
		FROM harness_sessions WHERE session_id = ?`), sessionRef)
	return scanSession(row)
}

func (r *SQLiteSessionRepository) GetActiveByMember(ctx context.Context, memberID string) (*domain.Session, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url,
			claude_channel_instance_id, claude_channel_started_at
		FROM harness_sessions WHERE member_id = ? AND status = 'active'
		LIMIT 1`), memberID)
	return scanSession(row)
}

func (r *SQLiteSessionRepository) ListActive(ctx context.Context) ([]*domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url,
			claude_channel_instance_id, claude_channel_started_at
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

func (r *SQLiteSessionRepository) ListByMember(ctx context.Context, memberID string) ([]*domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url,
			claude_channel_instance_id, claude_channel_started_at
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

func (r *SQLiteSessionRepository) ListBySpace(ctx context.Context, spaceID string) ([]*domain.Session, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT session_id, member_id, space_id, status, inactive_reason, inactive_error,
			project_id, location_id,
			activated_at, deactivated_at, tokens_in, tokens_out,
			harness_kind, model, effort, permission_mode, config_ref, session_ref,
			channel_id, workdir, display_name, member_type, lifecycle_state,
			system_prompt, mcp_token, mcp_servers_json, claude_channel_url,
			claude_channel_instance_id, claude_channel_started_at
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

type scannable interface {
	Scan(dest ...any) error
}

func scanSession(row *sql.Row) (*domain.Session, error) {
	var s domain.Session
	var status, inactiveReason, activatedAtStr string
	var mcpServersJSON string
	var deactivatedAtStr, claudeChannelStartedAtStr *string
	err := row.Scan(
		&s.ID, &s.MemberID, &s.SpaceID,
		&status, &inactiveReason, &s.InactiveError,
		&s.ProjectID, &s.LocationID,
		&activatedAtStr, &deactivatedAtStr,
		&s.TokensIn, &s.TokensOut,
		&s.Kind, &s.Model, &s.Effort, &s.PermissionMode, &s.ConfigRef, &s.Ref,
		&s.ChannelID, &s.Workdir, &s.DisplayName, &s.MemberType, &s.LifecycleState,
		&s.SystemPrompt, &s.MCPToken, &mcpServersJSON, &s.ClaudeChannelURL,
		&s.ClaudeChannelInstanceID, &claudeChannelStartedAtStr,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan harness session: %w", err)
	}
	return hydrateScannedSession(s, status, inactiveReason, activatedAtStr, deactivatedAtStr, mcpServersJSON, claudeChannelStartedAtStr)
}

func scanSessionRow(rows *sql.Rows) (*domain.Session, error) {
	var s domain.Session
	var status, inactiveReason, activatedAtStr string
	var mcpServersJSON string
	var deactivatedAtStr, claudeChannelStartedAtStr *string
	err := rows.Scan(
		&s.ID, &s.MemberID, &s.SpaceID,
		&status, &inactiveReason, &s.InactiveError,
		&s.ProjectID, &s.LocationID,
		&activatedAtStr, &deactivatedAtStr,
		&s.TokensIn, &s.TokensOut,
		&s.Kind, &s.Model, &s.Effort, &s.PermissionMode, &s.ConfigRef, &s.Ref,
		&s.ChannelID, &s.Workdir, &s.DisplayName, &s.MemberType, &s.LifecycleState,
		&s.SystemPrompt, &s.MCPToken, &mcpServersJSON, &s.ClaudeChannelURL,
		&s.ClaudeChannelInstanceID, &claudeChannelStartedAtStr,
	)
	if err != nil {
		return nil, fmt.Errorf("scan harness session row: %w", err)
	}
	return hydrateScannedSession(s, status, inactiveReason, activatedAtStr, deactivatedAtStr, mcpServersJSON, claudeChannelStartedAtStr)
}

func hydrateScannedSession(s domain.Session, status, inactiveReason, activatedAtStr string, deactivatedAtStr *string, mcpServersJSON string, claudeChannelStartedAtStr *string) (*domain.Session, error) {
	s.Status = domain.SessionStatus(status)
	s.InactiveReason = domain.InactiveReason(inactiveReason)
	activatedAt, err := time.Parse(time.RFC3339Nano, activatedAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse harness session activated_at %q: %w", activatedAtStr, err)
	}
	s.ActivatedAt = activatedAt
	if deactivatedAtStr != nil {
		t, err := time.Parse(time.RFC3339Nano, *deactivatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse harness session deactivated_at %q: %w", *deactivatedAtStr, err)
		}
		s.DeactivatedAt = &t
	}
	if mcpServersJSON != "" {
		if err := json.Unmarshal([]byte(mcpServersJSON), &s.MCPServers); err != nil {
			return nil, fmt.Errorf("decode harness session mcp servers: %w", err)
		}
	}
	if claudeChannelStartedAtStr != nil && *claudeChannelStartedAtStr != "" {
		t, err := time.Parse(time.RFC3339Nano, *claudeChannelStartedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse harness session claude_channel_started_at %q: %w", *claudeChannelStartedAtStr, err)
		}
		s.ClaudeChannelStartedAt = &t
	}
	if s.PermissionMode == "" {
		switch s.Kind {
		case "codex":
			s.PermissionMode = "codex/full-access"
		case "claude-cli":
			s.PermissionMode = "claude-code/bypass-permissions"
		}
	}
	return &s, nil
}

func formatOptionalTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}
