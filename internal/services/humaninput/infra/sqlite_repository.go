package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var _ domain.Repository = (*SQLiteRepository)(nil)

type SQLiteRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
}

func NewSQLiteRepository(handle *storagedb.Handle) (*SQLiteRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("human input db handle is required")
	}
	repo := &SQLiteRepository{db: handle.DB(), dialect: handle.Dialect()}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *SQLiteRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

func (r *SQLiteRepository) ensureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS human_input_requests (
			id TEXT PRIMARY KEY,
			tool_call_id TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			idempotency_key TEXT NOT NULL,
			project_id TEXT NOT NULL,
			space_id TEXT NOT NULL,
			asker_member_id TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			declaration_kind TEXT NOT NULL,
			declaration_payload_json TEXT NOT NULL,
			status TEXT NOT NULL,
			result_json TEXT NOT NULL DEFAULT '{}',
			resolver_user_id TEXT NOT NULL DEFAULT '',
			resolver_member_id TEXT NOT NULL DEFAULT '',
			terminal_reason TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			resolved_at TEXT,
			version INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_human_input_idempotency
			ON human_input_requests(project_id, tool_name, tool_call_id, idempotency_key)`,
		`CREATE INDEX IF NOT EXISTS idx_human_input_pending_scope
			ON human_input_requests(project_id, space_id, status, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_human_input_channel_pending
			ON human_input_requests(channel_id, status, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_human_input_expiry
			ON human_input_requests(status, expires_at)`,
		`CREATE TABLE IF NOT EXISTS human_input_outbox (
			id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			delivered_at TEXT,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, stmt := range stmts {
		if _, err := r.db.ExecContext(ctx, r.rebind(stmt)); err != nil {
			return fmt.Errorf("ensure human input schema: %w", err)
		}
	}
	return nil
}

func (r *SQLiteRepository) CreatePending(ctx context.Context, req domain.Request) (domain.Request, error) {
	if err := req.Validate(); err != nil {
		return domain.Request{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Request{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, r.rebind(`
		INSERT INTO human_input_requests (
			id, tool_call_id, tool_name, idempotency_key, project_id, space_id,
			asker_member_id, channel_id, declaration_kind, declaration_payload_json,
			status, result_json, terminal_reason, created_at, expires_at, version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		string(req.ID), string(req.ToolCallID), req.ToolName, req.IdempotencyKey, req.ProjectID, req.SpaceID,
		req.AskerMemberID, req.ChannelID, string(req.Declaration.Kind), string(req.Declaration.Payload),
		string(req.Status), string(req.Result), req.TerminalReason, formatTime(req.CreatedAt), formatTime(req.ExpiresAt), req.Version,
	)
	if err != nil {
		existing, findErr := r.findByIdempotencyTx(ctx, tx, req.ProjectID, req.ToolName, req.ToolCallID, req.IdempotencyKey)
		if findErr == nil {
			return existing, tx.Commit()
		}
		return domain.Request{}, fmt.Errorf("create human input request: %w", err)
	}
	if err := insertOutbox(ctx, tx, r.rebind, req.ID, "humaninput.pending", req.CreatedAt); err != nil {
		return domain.Request{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Request{}, err
	}
	return req, nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id domain.RequestID) (domain.Request, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(selectRequestSQL()+` WHERE id = ?`), string(id))
	return scanRequest(row)
}

func (r *SQLiteRepository) FindByIdempotency(ctx context.Context, projectID, toolName string, toolCallID domain.ToolCallID, idempotencyKey string) (domain.Request, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(selectRequestSQL()+` WHERE project_id = ? AND tool_name = ? AND tool_call_id = ? AND idempotency_key = ?`),
		strings.TrimSpace(projectID), strings.TrimSpace(toolName), strings.TrimSpace(string(toolCallID)), strings.TrimSpace(idempotencyKey))
	return scanRequest(row)
}

func (r *SQLiteRepository) findByIdempotencyTx(ctx context.Context, tx *sql.Tx, projectID, toolName string, toolCallID domain.ToolCallID, idempotencyKey string) (domain.Request, error) {
	row := tx.QueryRowContext(ctx, r.rebind(selectRequestSQL()+` WHERE project_id = ? AND tool_name = ? AND tool_call_id = ? AND idempotency_key = ?`),
		strings.TrimSpace(projectID), strings.TrimSpace(toolName), strings.TrimSpace(string(toolCallID)), strings.TrimSpace(idempotencyKey))
	return scanRequest(row)
}

func (r *SQLiteRepository) ListPending(ctx context.Context, filter domain.Filter) ([]domain.Request, error) {
	query := selectRequestSQL() + ` WHERE status = ?`
	args := []any{string(domain.StatusPending)}
	if strings.TrimSpace(filter.ProjectID) != "" {
		query += ` AND project_id = ?`
		args = append(args, strings.TrimSpace(filter.ProjectID))
	}
	if strings.TrimSpace(filter.SpaceID) != "" {
		query += ` AND space_id = ?`
		args = append(args, strings.TrimSpace(filter.SpaceID))
	}
	if strings.TrimSpace(filter.ChannelID) != "" {
		query += ` AND channel_id = ?`
		args = append(args, strings.TrimSpace(filter.ChannelID))
	}
	if strings.TrimSpace(filter.MemberID) != "" {
		query += ` AND asker_member_id = ?`
		args = append(args, strings.TrimSpace(filter.MemberID))
	}
	if filter.Kind != "" {
		query += ` AND declaration_kind = ?`
		args = append(args, string(filter.Kind))
	}
	query += ` ORDER BY created_at ASC`
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query += ` LIMIT ? OFFSET ?`
	args = append(args, limit, filter.Offset)
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Request
	for rows.Next() {
		req, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, rows.Err()
}

func (r *SQLiteRepository) Resolve(ctx context.Context, mutation domain.ResolveMutation) (domain.Request, error) {
	if !domain.IsTerminal(mutation.Status) {
		return domain.Request{}, fmt.Errorf("human input resolve status must be terminal")
	}
	if len(mutation.Result) == 0 || !json.Valid(mutation.Result) {
		return domain.Request{}, fmt.Errorf("human input result must be valid JSON")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Request{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, r.rebind(`
		UPDATE human_input_requests
		SET status = ?, result_json = ?, resolver_user_id = ?, resolver_member_id = ?,
			terminal_reason = ?, resolved_at = ?, version = version + 1
		WHERE id = ? AND status = ? AND version = ?`),
		string(mutation.Status), string(mutation.Result), strings.TrimSpace(mutation.ResolverUserID),
		strings.TrimSpace(mutation.ResolverMemberID), strings.TrimSpace(mutation.TerminalReason),
		formatTime(mutation.ResolvedAt), string(mutation.ID), string(domain.StatusPending), mutation.ExpectedVersion,
	)
	if err != nil {
		return domain.Request{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return domain.Request{}, err
	}
	if affected != 1 {
		return domain.Request{}, fmt.Errorf("human input request %s is not pending at expected version", mutation.ID)
	}
	row := tx.QueryRowContext(ctx, r.rebind(selectRequestSQL()+` WHERE id = ?`), string(mutation.ID))
	req, err := scanRequest(row)
	if err != nil {
		return domain.Request{}, err
	}
	if err := insertOutbox(ctx, tx, r.rebind, mutation.ID, "humaninput.resolved", mutation.ResolvedAt); err != nil {
		return domain.Request{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Request{}, err
	}
	return req, nil
}

func (r *SQLiteRepository) ExpireDue(ctx context.Context, now time.Time, limit int) (domain.ExpireBatch, error) {
	if limit <= 0 {
		limit = 100
	}
	pending, err := r.ListPending(ctx, domain.Filter{Limit: limit})
	if err != nil {
		return domain.ExpireBatch{}, err
	}
	out := domain.ExpireBatch{}
	for _, req := range pending {
		if req.ExpiresAt.After(now) {
			continue
		}
		expired, err := r.Resolve(ctx, domain.ResolveMutation{
			ID:              req.ID,
			ExpectedVersion: req.Version,
			Status:          domain.StatusExpired,
			Result:          json.RawMessage(`{"expired":true}`),
			TerminalReason:  "expired",
			ResolvedAt:      now.UTC(),
		})
		if err != nil {
			return out, err
		}
		out.Requests = append(out.Requests, expired)
		out.Count++
	}
	return out, nil
}

func (r *SQLiteRepository) AbortByToolCall(ctx context.Context, toolCallID domain.ToolCallID, reason string) (domain.Request, error) {
	rows, err := r.db.QueryContext(ctx, r.rebind(selectRequestSQL()+` WHERE tool_call_id = ? AND status = ? ORDER BY created_at ASC LIMIT 1`),
		strings.TrimSpace(string(toolCallID)), string(domain.StatusPending))
	if err != nil {
		return domain.Request{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return domain.Request{}, sql.ErrNoRows
	}
	req, err := scanRequest(rows)
	if err != nil {
		return domain.Request{}, err
	}
	return r.Resolve(ctx, domain.ResolveMutation{
		ID:              req.ID,
		ExpectedVersion: req.Version,
		Status:          domain.StatusAborted,
		Result:          json.RawMessage(`{"aborted":true}`),
		TerminalReason:  strings.TrimSpace(reason),
		ResolvedAt:      time.Now().UTC(),
	})
}

func selectRequestSQL() string {
	return `SELECT id, tool_call_id, tool_name, idempotency_key, project_id, space_id,
		asker_member_id, channel_id, declaration_kind, declaration_payload_json,
		status, result_json, resolver_user_id, resolver_member_id, terminal_reason,
		created_at, expires_at, resolved_at, version FROM human_input_requests`
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRequest(s scanner) (domain.Request, error) {
	var (
		req                domain.Request
		id                 string
		toolCallID         string
		declarationKind    string
		declarationPayload string
		status             string
		result             string
		createdAt          string
		expiresAt          string
		resolvedAt         sql.NullString
		resolverUserID     string
		resolverMemberID   string
	)
	err := s.Scan(
		&id, &toolCallID, &req.ToolName, &req.IdempotencyKey, &req.ProjectID, &req.SpaceID,
		&req.AskerMemberID, &req.ChannelID, &declarationKind, &declarationPayload,
		&status, &result, &resolverUserID, &resolverMemberID, &req.TerminalReason,
		&createdAt, &expiresAt, &resolvedAt, &req.Version,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Request{}, fmt.Errorf("human input request not found")
		}
		return domain.Request{}, err
	}
	req.ID = domain.RequestID(id)
	req.ToolCallID = domain.ToolCallID(toolCallID)
	req.Declaration = domain.Declaration{Kind: domain.PrimitiveKind(declarationKind), Payload: json.RawMessage(declarationPayload)}
	req.Status = domain.Status(status)
	req.Result = json.RawMessage(result)
	req.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return domain.Request{}, err
	}
	req.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return domain.Request{}, err
	}
	if resolvedAt.Valid && strings.TrimSpace(resolvedAt.String) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, resolvedAt.String)
		if err != nil {
			return domain.Request{}, err
		}
		req.ResolvedAt = &parsed
	}
	return req, nil
}

func insertOutbox(ctx context.Context, tx *sql.Tx, rebind func(string) string, requestID domain.RequestID, eventType string, at time.Time) error {
	_, err := tx.ExecContext(ctx, rebind(`
		INSERT INTO human_input_outbox (id, request_id, event_type, payload_json, created_at)
		VALUES (?, ?, ?, ?, ?)`),
		"hi_outbox_"+string(requestID)+"_"+eventType, string(requestID), eventType, "{}", formatTime(at))
	if err != nil {
		return fmt.Errorf("insert human input outbox: %w", err)
	}
	return nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
