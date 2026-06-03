package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/conversation"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

var _ conversation.Repository = (*SQLiteConversationRepository)(nil)
var _ conversation.Repository = (*PostgresConversationRepository)(nil)

const conversationTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

type SQLiteConversationRepository struct {
	*conversationSQLRepository
}

type PostgresConversationRepository struct {
	*conversationSQLRepository
}

type conversationSQLRepository struct {
	db      *sql.DB
	dialect storagedb.Dialect
	name    string
}

type conversationScannable interface {
	Scan(dest ...any) error
}

func NewSQLiteConversationRepository(handle *storagedb.Handle) (*SQLiteConversationRepository, error) {
	if handle == nil || handle.DB() == nil || handle.Dialect() == nil {
		return nil, fmt.Errorf("message sqlite conversation repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("message sqlite conversation repository: storage driver must be sqlite, got %q", handle.Driver())
	}
	if err := MigrateConversationSchema(context.Background(), handle.DB()); err != nil {
		return nil, err
	}
	return &SQLiteConversationRepository{conversationSQLRepository: &conversationSQLRepository{db: handle.DB(), dialect: handle.Dialect(), name: "sqlite"}}, nil
}

func NewPostgresConversationRepository(handle *storagedb.Handle) (*PostgresConversationRepository, error) {
	if handle == nil || handle.DB() == nil || handle.Dialect() == nil {
		return nil, fmt.Errorf("message postgres conversation repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("message postgres conversation repository: storage driver must be postgres, got %q", handle.Driver())
	}
	if err := MigrateConversationSchema(context.Background(), handle.DB()); err != nil {
		return nil, err
	}
	return &PostgresConversationRepository{conversationSQLRepository: &conversationSQLRepository{db: handle.DB(), dialect: handle.Dialect(), name: "postgres"}}, nil
}

func MigrateConversationSchema(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("message conversation migration: db is nil")
	}
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS message_conversation_messages (
			message_id     TEXT PRIMARY KEY,
			channel_id     TEXT NOT NULL,
			space_id       TEXT NOT NULL,
			member_id      TEXT NOT NULL,
			session_id     TEXT NOT NULL DEFAULT '',
			turn_id        TEXT NOT NULL DEFAULT '',
			direction      TEXT NOT NULL,
			sender_type    TEXT NOT NULL,
			sender_id      TEXT NOT NULL DEFAULT '',
			text           TEXT NOT NULL,
			attachments_json TEXT NOT NULL DEFAULT '[]',
			delivery_state TEXT NOT NULL DEFAULT '',
			render_state   TEXT NOT NULL,
			error          TEXT NOT NULL DEFAULT '',
			created_at     TEXT NOT NULL,
			updated_at     TEXT NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("message conversation migration: create table: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS message_conversation_attachments (
			attachment_id TEXT PRIMARY KEY,
			project_id    TEXT NOT NULL,
			space_id      TEXT NOT NULL,
			channel_id    TEXT NOT NULL,
			name          TEXT NOT NULL,
			media_type    TEXT NOT NULL,
			size_bytes    INTEGER NOT NULL,
			uri           TEXT NOT NULL,
			created_at    TEXT NOT NULL
		)`)
	if err != nil {
		return fmt.Errorf("message conversation migration: create attachments table: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS message_conversation_activities (
			activity_id  TEXT PRIMARY KEY,
			channel_id   TEXT NOT NULL,
			space_id     TEXT NOT NULL,
			member_id    TEXT NOT NULL,
			session_id   TEXT NOT NULL,
			turn_id      TEXT NOT NULL,
			tool_call_id TEXT NOT NULL,
			sequence     INTEGER NOT NULL DEFAULT 0,
			kind         TEXT NOT NULL,
			title        TEXT NOT NULL,
			status       TEXT NOT NULL,
			text         TEXT NOT NULL DEFAULT '',
			created_at   TEXT NOT NULL,
			completed_at TEXT,
			data_json    TEXT NOT NULL DEFAULT '{}'
		)`)
	if err != nil {
		return fmt.Errorf("message conversation migration: create activities table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE message_conversation_messages ADD COLUMN attachments_json TEXT NOT NULL DEFAULT '[]'`); err != nil && !isDuplicateColumnError(err) {
		return fmt.Errorf("message conversation migration: add attachments column: %w", err)
	}
	if _, err := db.ExecContext(ctx, `ALTER TABLE message_conversation_activities ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0`); err != nil && !isDuplicateColumnError(err) {
		return fmt.Errorf("message conversation migration: add activity sequence column: %w", err)
	}
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_message_conversation_channel_created ON message_conversation_messages (channel_id, created_at, message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_conversation_session_delivery_created ON message_conversation_messages (session_id, delivery_state, created_at, message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_conversation_member_created ON message_conversation_messages (member_id, created_at, message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_conversation_turn ON message_conversation_messages (turn_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_conversation_attachments_channel_created ON message_conversation_attachments (channel_id, created_at, attachment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_conversation_activities_channel_created ON message_conversation_activities (channel_id, sequence, created_at, activity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_conversation_activities_tool_call ON message_conversation_activities (session_id, turn_id, tool_call_id)`,
	} {
		if _, err := db.ExecContext(ctx, idx); err != nil {
			return fmt.Errorf("message conversation migration: index: %w", err)
		}
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate column") || strings.Contains(msg, "already exists")
}

func (r *conversationSQLRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

func (r *conversationSQLRepository) SaveActivity(ctx context.Context, activity conversation.Activity) error {
	if err := conversation.ValidateActivity(activity); err != nil {
		return err
	}
	data := activity.Data
	if data == nil {
		data = map[string]string{}
	}
	rawData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal message conversation activity %s data: %w", activity.ID, err)
	}
	var completedAt any
	if activity.CompletedAt != nil && !activity.CompletedAt.IsZero() {
		completedAt = formatConversationTime(*activity.CompletedAt)
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO message_conversation_activities (
			activity_id, channel_id, space_id, member_id, session_id, turn_id,
			tool_call_id, sequence, kind, title, status, text, created_at, completed_at, data_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (activity_id) DO UPDATE SET
			channel_id = excluded.channel_id,
			space_id = excluded.space_id,
			member_id = excluded.member_id,
			session_id = excluded.session_id,
			turn_id = excluded.turn_id,
			tool_call_id = excluded.tool_call_id,
			sequence = excluded.sequence,
			kind = excluded.kind,
			title = excluded.title,
			status = excluded.status,
			text = excluded.text,
			completed_at = excluded.completed_at,
			data_json = excluded.data_json`),
		activity.ID,
		activity.ChannelID,
		activity.SpaceID,
		activity.MemberID,
		activity.SessionID,
		activity.TurnID,
		activity.ToolCallID,
		activity.Sequence,
		activity.Kind,
		activity.Title,
		activity.Status,
		activity.Text,
		formatConversationTime(activity.CreatedAt),
		completedAt,
		string(rawData),
	)
	if err != nil {
		return fmt.Errorf("save message conversation activity %s: %w", activity.ID, err)
	}
	return nil
}

func (r *conversationSQLRepository) Save(ctx context.Context, msg conversation.Message) error {
	if err := conversation.ValidateMessage(msg); err != nil {
		return err
	}
	rawAttachments, err := json.Marshal(msg.Attachments)
	if err != nil {
		return fmt.Errorf("marshal message conversation message %s attachments: %w", msg.ID, err)
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO message_conversation_messages (
			message_id, channel_id, space_id, member_id, session_id, turn_id,
			direction, sender_type, sender_id, text, attachments_json, delivery_state, render_state,
			error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (message_id) DO UPDATE SET
			channel_id = excluded.channel_id,
			space_id = excluded.space_id,
			member_id = excluded.member_id,
			session_id = excluded.session_id,
			turn_id = excluded.turn_id,
			direction = excluded.direction,
			sender_type = excluded.sender_type,
			sender_id = excluded.sender_id,
			text = excluded.text,
			attachments_json = excluded.attachments_json,
			delivery_state = excluded.delivery_state,
			render_state = excluded.render_state,
			error = excluded.error,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at`),
		msg.ID,
		msg.ChannelID,
		msg.SpaceID,
		msg.MemberID,
		msg.SessionID,
		msg.TurnID,
		string(msg.Direction),
		msg.SenderType,
		msg.SenderID,
		msg.Text,
		string(rawAttachments),
		string(msg.Delivery),
		string(msg.Render),
		msg.Error,
		formatConversationTime(msg.CreatedAt),
		formatConversationTime(msg.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("save message conversation message %s: %w", msg.ID, err)
	}
	return nil
}

func (r *conversationSQLRepository) SaveAttachment(ctx context.Context, attachment conversation.Attachment) error {
	if err := conversation.ValidateAttachment(attachment); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO message_conversation_attachments (
			attachment_id, project_id, space_id, channel_id, name, media_type, size_bytes, uri, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (attachment_id) DO UPDATE SET
			project_id = excluded.project_id,
			space_id = excluded.space_id,
			channel_id = excluded.channel_id,
			name = excluded.name,
			media_type = excluded.media_type,
			size_bytes = excluded.size_bytes,
			uri = excluded.uri,
			created_at = excluded.created_at`),
		attachment.ID,
		attachment.ProjectID,
		attachment.SpaceID,
		attachment.ChannelID,
		attachment.Name,
		attachment.MediaType,
		attachment.SizeBytes,
		attachment.URI,
		formatConversationTime(attachment.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("save message conversation attachment %s: %w", attachment.ID, err)
	}
	return nil
}

func (r *conversationSQLRepository) AppendText(ctx context.Context, id string, delta string, updatedAt time.Time) (conversation.Message, error) {
	if id == "" {
		return conversation.Message{}, fmt.Errorf("id is required")
	}
	if delta == "" {
		return conversation.Message{}, fmt.Errorf("delta is required")
	}
	if updatedAt.IsZero() {
		return conversation.Message{}, fmt.Errorf("updatedAt is required")
	}
	_, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE message_conversation_messages
		SET text = text || ?, updated_at = ?
		WHERE message_id = ?`),
		delta,
		formatConversationTime(updatedAt),
		id,
	)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("append message conversation text %s: %w", id, err)
	}
	msg, err := r.get(ctx, id)
	if err != nil {
		return conversation.Message{}, err
	}
	if msg == nil {
		return conversation.Message{}, fmt.Errorf("message conversation message %s not found", id)
	}
	return *msg, nil
}

func (r *conversationSQLRepository) UpdateDelivery(ctx context.Context, id string, state conversation.DeliveryState, errText string, updatedAt time.Time) (conversation.Message, error) {
	return r.UpdateDeliveryBinding(ctx, id, state, "", "", errText, updatedAt)
}

func (r *conversationSQLRepository) UpdateDeliveryBinding(ctx context.Context, id string, state conversation.DeliveryState, sessionID string, turnID string, errText string, updatedAt time.Time) (conversation.Message, error) {
	if id == "" {
		return conversation.Message{}, fmt.Errorf("id is required")
	}
	if err := conversation.ValidateDeliveryState(state); err != nil {
		return conversation.Message{}, err
	}
	if state != conversation.DeliveryFailed && sessionID == "" {
		return conversation.Message{}, fmt.Errorf("sessionID is required")
	}
	if state != conversation.DeliveryFailed && turnID == "" {
		return conversation.Message{}, fmt.Errorf("turnID is required")
	}
	if state == conversation.DeliveryFailed && errText == "" {
		return conversation.Message{}, fmt.Errorf("error is required when delivery state is failed")
	}
	if updatedAt.IsZero() {
		return conversation.Message{}, fmt.Errorf("updatedAt is required")
	}
	_, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE message_conversation_messages
		SET delivery_state = ?, session_id = ?, turn_id = ?, error = ?, updated_at = ?
		WHERE message_id = ?`),
		string(state),
		sessionID,
		turnID,
		errText,
		formatConversationTime(updatedAt),
		id,
	)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("update message conversation message %s delivery: %w", id, err)
	}
	msg, err := r.get(ctx, id)
	if err != nil {
		return conversation.Message{}, err
	}
	if msg == nil {
		return conversation.Message{}, fmt.Errorf("message conversation message %s not found", id)
	}
	return *msg, nil
}

func (r *conversationSQLRepository) UpdateRender(ctx context.Context, id string, state conversation.RenderState, errText string, updatedAt time.Time) (conversation.Message, error) {
	if id == "" {
		return conversation.Message{}, fmt.Errorf("id is required")
	}
	if err := conversation.ValidateRenderState(state); err != nil {
		return conversation.Message{}, err
	}
	if state == conversation.RenderError && errText == "" {
		return conversation.Message{}, fmt.Errorf("error is required when render state is error")
	}
	if updatedAt.IsZero() {
		return conversation.Message{}, fmt.Errorf("updatedAt is required")
	}
	_, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE message_conversation_messages
		SET render_state = ?, error = ?, updated_at = ?
		WHERE message_id = ?`),
		string(state),
		errText,
		formatConversationTime(updatedAt),
		id,
	)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("update message conversation message %s render: %w", id, err)
	}
	msg, err := r.get(ctx, id)
	if err != nil {
		return conversation.Message{}, err
	}
	if msg == nil {
		return conversation.Message{}, fmt.Errorf("message conversation message %s not found", id)
	}
	return *msg, nil
}

func (r *conversationSQLRepository) ListByChannel(ctx context.Context, channelID string, limit int) ([]conversation.Message, error) {
	if channelID == "" {
		return nil, fmt.Errorf("channelID is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT message_id, channel_id, space_id, member_id, session_id, turn_id,
			direction, sender_type, sender_id, text, attachments_json, delivery_state, render_state,
			error, created_at, updated_at
		FROM message_conversation_messages
		WHERE channel_id = ?
		ORDER BY created_at ASC, message_id ASC
		LIMIT ?`), channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("list message conversation messages by channel %s: %w", channelID, err)
	}
	defer rows.Close()
	return scanConversationRows(rows)
}

func (r *conversationSQLRepository) ListActivitiesByChannel(ctx context.Context, channelID string, limit int) ([]conversation.Activity, error) {
	if channelID == "" {
		return nil, fmt.Errorf("channelID is required")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT activity_id, channel_id, space_id, member_id, session_id, turn_id,
			tool_call_id, sequence, kind, title, status, text, created_at, completed_at, data_json
		FROM message_conversation_activities
		WHERE channel_id = ? AND sequence > 0
		ORDER BY sequence ASC, created_at ASC, activity_id ASC
		LIMIT ?`), channelID, limit)
	if err != nil {
		return nil, fmt.Errorf("list message conversation activities by channel %s: %w", channelID, err)
	}
	defer rows.Close()
	var out []conversation.Activity
	for rows.Next() {
		activity, err := scanConversationActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, activity)
	}
	return out, rows.Err()
}

func (r *conversationSQLRepository) NextQueuedForSession(ctx context.Context, sessionID string) (*conversation.Message, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID is required")
	}
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT message_id, channel_id, space_id, member_id, session_id, turn_id,
			direction, sender_type, sender_id, text, attachments_json, delivery_state, render_state,
			error, created_at, updated_at
		FROM message_conversation_messages
		WHERE session_id = ? AND direction = ? AND delivery_state = ?
		ORDER BY created_at ASC, message_id ASC
		LIMIT 1`), sessionID, string(conversation.DirectionInbound), string(conversation.DeliveryQueued))
	return scanConversationRow(row)
}

func (r *conversationSQLRepository) Get(ctx context.Context, id string) (*conversation.Message, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	return r.get(ctx, id)
}

func (r *conversationSQLRepository) get(ctx context.Context, id string) (*conversation.Message, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT message_id, channel_id, space_id, member_id, session_id, turn_id,
			direction, sender_type, sender_id, text, attachments_json, delivery_state, render_state,
			error, created_at, updated_at
		FROM message_conversation_messages
		WHERE message_id = ?`), id)
	return scanConversationRow(row)
}

func scanConversationRows(rows *sql.Rows) ([]conversation.Message, error) {
	var out []conversation.Message
	for rows.Next() {
		msg, err := scanConversationScannable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func scanConversationRow(row *sql.Row) (*conversation.Message, error) {
	msg, err := scanConversationScannable(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (r *conversationSQLRepository) GetAttachments(ctx context.Context, ids []string) ([]conversation.Attachment, error) {
	cleaned := cleanAttachmentIDs(ids)
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("attachment ids are required")
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(cleaned)), ",")
	args := make([]any, 0, len(cleaned))
	for _, id := range cleaned {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT attachment_id, project_id, space_id, channel_id, name, media_type, size_bytes, uri, created_at
		FROM message_conversation_attachments
		WHERE attachment_id IN (`+placeholders+`)`), args...)
	if err != nil {
		return nil, fmt.Errorf("get message conversation attachments: %w", err)
	}
	defer rows.Close()
	found := map[string]conversation.Attachment{}
	for rows.Next() {
		attachment, err := scanConversationAttachment(rows)
		if err != nil {
			return nil, err
		}
		found[attachment.ID] = attachment
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]conversation.Attachment, 0, len(cleaned))
	for _, id := range cleaned {
		attachment, ok := found[id]
		if !ok {
			return nil, fmt.Errorf("message conversation attachment %s not found", id)
		}
		out = append(out, attachment)
	}
	return out, nil
}

func scanConversationScannable(row conversationScannable) (conversation.Message, error) {
	var msg conversation.Message
	var direction, delivery, render, attachmentsJSON, createdAt, updatedAt string
	err := row.Scan(
		&msg.ID,
		&msg.ChannelID,
		&msg.SpaceID,
		&msg.MemberID,
		&msg.SessionID,
		&msg.TurnID,
		&direction,
		&msg.SenderType,
		&msg.SenderID,
		&msg.Text,
		&attachmentsJSON,
		&delivery,
		&render,
		&msg.Error,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return conversation.Message{}, err
	}
	msg.Direction = conversation.Direction(direction)
	if strings.TrimSpace(attachmentsJSON) != "" {
		if err := json.Unmarshal([]byte(attachmentsJSON), &msg.Attachments); err != nil {
			return conversation.Message{}, fmt.Errorf("unmarshal message conversation attachments: %w", err)
		}
	}
	msg.Delivery = conversation.DeliveryState(delivery)
	msg.Render = conversation.RenderState(render)
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("parse message conversation created_at %q: %w", createdAt, err)
	}
	msg.CreatedAt = parsedCreatedAt
	parsedUpdatedAt, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return conversation.Message{}, fmt.Errorf("parse message conversation updated_at %q: %w", updatedAt, err)
	}
	msg.UpdatedAt = parsedUpdatedAt
	if err := conversation.ValidateMessage(msg); err != nil {
		return conversation.Message{}, fmt.Errorf("scan message conversation message %s: %w", msg.ID, err)
	}
	return msg, nil
}

func scanConversationAttachment(row conversationScannable) (conversation.Attachment, error) {
	var attachment conversation.Attachment
	var createdAt string
	if err := row.Scan(
		&attachment.ID,
		&attachment.ProjectID,
		&attachment.SpaceID,
		&attachment.ChannelID,
		&attachment.Name,
		&attachment.MediaType,
		&attachment.SizeBytes,
		&attachment.URI,
		&createdAt,
	); err != nil {
		return conversation.Attachment{}, err
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return conversation.Attachment{}, fmt.Errorf("parse message conversation attachment created_at %q: %w", createdAt, err)
	}
	attachment.CreatedAt = parsedCreatedAt
	if err := conversation.ValidateAttachment(attachment); err != nil {
		return conversation.Attachment{}, fmt.Errorf("scan message conversation attachment %s: %w", attachment.ID, err)
	}
	return attachment, nil
}

func cleanAttachmentIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func scanConversationActivity(row conversationScannable) (conversation.Activity, error) {
	var activity conversation.Activity
	var createdAt string
	var completedAt sql.NullString
	var rawData string
	err := row.Scan(
		&activity.ID,
		&activity.ChannelID,
		&activity.SpaceID,
		&activity.MemberID,
		&activity.SessionID,
		&activity.TurnID,
		&activity.ToolCallID,
		&activity.Sequence,
		&activity.Kind,
		&activity.Title,
		&activity.Status,
		&activity.Text,
		&createdAt,
		&completedAt,
		&rawData,
	)
	if err != nil {
		return conversation.Activity{}, err
	}
	parsedCreatedAt, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return conversation.Activity{}, fmt.Errorf("parse message conversation activity created_at %q: %w", createdAt, err)
	}
	activity.CreatedAt = parsedCreatedAt
	if completedAt.Valid && completedAt.String != "" {
		parsedCompletedAt, err := time.Parse(time.RFC3339Nano, completedAt.String)
		if err != nil {
			return conversation.Activity{}, fmt.Errorf("parse message conversation activity completed_at %q: %w", completedAt.String, err)
		}
		activity.CompletedAt = &parsedCompletedAt
	}
	if rawData != "" {
		if err := json.Unmarshal([]byte(rawData), &activity.Data); err != nil {
			return conversation.Activity{}, fmt.Errorf("unmarshal message conversation activity data: %w", err)
		}
	}
	if activity.Data == nil {
		activity.Data = map[string]string{}
	}
	if err := conversation.ValidateActivity(activity); err != nil {
		return conversation.Activity{}, fmt.Errorf("scan message conversation activity %s: %w", activity.ID, err)
	}
	return activity, nil
}

func formatConversationTime(t time.Time) string {
	return t.UTC().Format(conversationTimeFormat)
}
