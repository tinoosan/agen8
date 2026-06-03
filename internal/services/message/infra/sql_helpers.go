package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

func (r *sqlStore) ensureSchema(ctx context.Context) error {
	if err := r.ensureMessageSchema(ctx); err != nil {
		return err
	}
	if err := r.ensureChannelSchema(ctx); err != nil {
		return err
	}
	return nil
}

func (r *sqlStore) ensureMessageSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, r.rebind(`
		CREATE TABLE IF NOT EXISTS agent_messages (
			message_id TEXT PRIMARY KEY,
			intent_id TEXT NOT NULL,
			correlation_id TEXT NOT NULL DEFAULT '',
			causation_id TEXT NOT NULL DEFAULT '',
			producer TEXT NOT NULL DEFAULT '',
			space_id TEXT NOT NULL,
			source_member_id TEXT NOT NULL DEFAULT '',
			destination_member_id TEXT NOT NULL,
			channel_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			body_json `+r.jsonCol+` NOT NULL,
			task_ref TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			visible_at TEXT NOT NULL,
			metadata_json `+r.jsonCol+` NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			consumed_at TEXT,
			consumed_by TEXT NOT NULL DEFAULT '',
			message_json `+r.jsonCol+` NOT NULL,
			UNIQUE(destination_member_id, intent_id)
		)
	`)); err != nil {
		return fmt.Errorf("ensure agent_messages table: %w", err)
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_agent_messages_member_queue ON agent_messages(destination_member_id, status, visible_at, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_messages_space ON agent_messages(space_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_messages_channel ON agent_messages(channel_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_messages_correlation ON agent_messages(correlation_id)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_messages_task_ref ON agent_messages(task_ref)`,
	} {
		if _, err := r.db.ExecContext(ctx, r.rebind(stmt)); err != nil {
			return fmt.Errorf("ensure agent_messages index: %w", err)
		}
	}
	return nil
}

func (r *sqlStore) ensureChannelSchema(ctx context.Context) error {
	if _, err := r.db.ExecContext(ctx, r.rebind(`
		CREATE TABLE IF NOT EXISTS message_channels (
			channel_id TEXT PRIMARY KEY,
			space_id TEXT NOT NULL,
			project_id TEXT NOT NULL DEFAULT '',
			member_id TEXT NOT NULL,
			status TEXT NOT NULL,
			last_message_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			channel_json `+r.jsonCol+` NOT NULL
		)
	`)); err != nil {
		return fmt.Errorf("ensure message_channels table: %w", err)
	}
	if _, err := r.db.ExecContext(ctx, r.rebind(`
		CREATE TABLE IF NOT EXISTS message_channel_reads (
			user_id TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			PRIMARY KEY(user_id, channel_id)
		)
	`)); err != nil {
		return fmt.Errorf("ensure message_channel_reads table: %w", err)
	}
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_message_channels_space ON message_channels(space_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_message_channels_member ON message_channels(space_id, member_id)`,
		`CREATE INDEX IF NOT EXISTS idx_message_channel_reads_user ON message_channel_reads(user_id)`,
	} {
		if _, err := r.db.ExecContext(ctx, r.rebind(stmt)); err != nil {
			return fmt.Errorf("ensure message_channels index: %w", err)
		}
	}
	return nil
}

func messageWhere(filter domain.MessageFilter) (string, []any, error) {
	if filter.Limit < 0 || filter.Offset < 0 {
		return "", nil, domain.ErrInvalidFilter
	}
	var clauses []string
	var args []any
	addID := func(column, value string) {
		if strings.TrimSpace(value) != "" {
			clauses = append(clauses, column+" = ?")
			args = append(args, strings.TrimSpace(value))
		}
	}
	addID("space_id", string(filter.SpaceID))
	addID("source_member_id", string(filter.SourceMemberID))
	addID("destination_member_id", string(filter.DestinationMemberID))
	addID("channel_id", string(filter.ChannelID))
	addID("correlation_id", string(filter.CorrelationID))
	addID("task_ref", string(filter.TaskRef))
	if len(filter.Kinds) > 0 {
		ph := make([]string, 0, len(filter.Kinds))
		for _, kind := range filter.Kinds {
			kind = types.AgentMessageKind(strings.TrimSpace(string(kind)))
			if kind == "" {
				continue
			}
			ph = append(ph, "?")
			args = append(args, string(kind))
		}
		if len(ph) > 0 {
			clauses = append(clauses, "kind IN ("+strings.Join(ph, ", ")+")")
		}
	}
	if len(filter.Statuses) > 0 {
		ph := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			status = types.MessageStatus(strings.TrimSpace(string(status)))
			if status == "" {
				continue
			}
			ph = append(ph, "?")
			args = append(args, string(status))
		}
		if len(ph) > 0 {
			clauses = append(clauses, "status IN ("+strings.Join(ph, ", ")+")")
		}
	}
	if filter.Since != nil {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, formatTime(filter.Since.UTC()))
	}
	if filter.Until != nil {
		clauses = append(clauses, "created_at <= ?")
		args = append(args, formatTime(filter.Until.UTC()))
	}
	if len(clauses) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args, nil
}

func validateMessageForSave(msg types.AgentMessage) error {
	switch {
	case msg.ID == "":
		return fmt.Errorf("message id is required")
	case msg.IntentID == "":
		return fmt.Errorf("intent id is required")
	case msg.SpaceID == "":
		return fmt.Errorf("space id is required")
	case msg.DestinationMemberID == "":
		return fmt.Errorf("destination member id is required")
	case msg.Kind == "":
		return fmt.Errorf("kind is required")
	case msg.Status == "":
		return fmt.Errorf("status is required")
	case msg.VisibleAt.IsZero():
		return fmt.Errorf("visible at is required")
	case msg.CreatedAt.IsZero():
		return fmt.Errorf("created at is required")
	case msg.UpdatedAt.IsZero():
		return fmt.Errorf("updated at is required")
	}
	return nil
}

func validateChannelForSave(ch types.Channel) error {
	switch {
	case ch.ID == "":
		return fmt.Errorf("channel id is required")
	case ch.SpaceID == "":
		return fmt.Errorf("space id is required")
	case strings.TrimSpace(ch.MemberID) == "":
		return fmt.Errorf("member id is required")
	case strings.TrimSpace(ch.Status) == "":
		return fmt.Errorf("status is required")
	case ch.CreatedAt.IsZero():
		return fmt.Errorf("created at is required")
	case ch.UpdatedAt.IsZero():
		return fmt.Errorf("updated at is required")
	}
	return nil
}

func marshalMessage(msg types.AgentMessage) (payload, body, metadata string, err error) {
	rawPayload, err := json.Marshal(msg)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal message: %w", err)
	}
	rawBody, err := json.Marshal(msg.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal message body: %w", err)
	}
	rawMetadata, err := json.Marshal(msg.Metadata)
	if err != nil {
		return "", "", "", fmt.Errorf("marshal message metadata: %w", err)
	}
	return string(rawPayload), string(rawBody), string(rawMetadata), nil
}

func scanMessage(scanner interface{ Scan(dest ...any) error }) (types.AgentMessage, error) {
	var raw []byte
	if err := scanner.Scan(&raw); err != nil {
		return types.AgentMessage{}, err
	}
	var msg types.AgentMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return types.AgentMessage{}, fmt.Errorf("unmarshal message: %w", err)
	}
	return msg.Normalized(), nil
}

func scanChannel(scanner interface{ Scan(dest ...any) error }) (types.Channel, error) {
	var raw []byte
	var lastActivity sql.NullString
	if err := scanner.Scan(&raw, &lastActivity); err != nil {
		return types.Channel{}, err
	}
	var ch types.Channel
	if err := json.Unmarshal(raw, &ch); err != nil {
		return types.Channel{}, fmt.Errorf("unmarshal channel: %w", err)
	}
	if lastActivity.Valid {
		if at, err := time.Parse(time.RFC3339Nano, lastActivity.String); err == nil && !at.IsZero() {
			ch.LastMessageAt = &at
		}
	}
	return channel.WrapChannel(ch).Inner(), nil
}

func cleanChannelIDs(channelIDs []types.ChannelID) []types.ChannelID {
	out := make([]types.ChannelID, 0, len(channelIDs))
	seen := map[types.ChannelID]struct{}{}
	for _, id := range channelIDs {
		id = types.ChannelID(strings.TrimSpace(string(id)))
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

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func formatOptionalTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return formatTime(*t)
}
