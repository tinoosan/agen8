package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/message/domain/channel"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type sqlStore struct {
	db      *sql.DB
	dialect storagedb.Dialect
	jsonCol string
}

func newSQLStore(handle *storagedb.Handle) *sqlStore {
	return &sqlStore{
		db:      handle.DB(),
		dialect: handle.Dialect(),
		jsonCol: handle.Dialect().JSONType(),
	}
}

func (r *sqlStore) SaveQueued(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	msg = msg.Normalized()
	if err := validateMessageForSave(msg); err != nil {
		return types.AgentMessage{}, err
	}
	if msg.Status != types.MessageStatusQueuedTyped {
		return types.AgentMessage{}, fmt.Errorf("message status must be queued")
	}
	payload, body, metadata, err := marshalMessage(msg)
	if err != nil {
		return types.AgentMessage{}, err
	}
	res, err := r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO agent_messages (
			message_id, intent_id, correlation_id, causation_id, producer,
			space_id, source_member_id, destination_member_id, channel_id,
			kind, subject, body_json, task_ref, status, visible_at,
			metadata_json, created_at, updated_at, consumed_at, consumed_by, message_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(destination_member_id, intent_id) DO NOTHING
	`),
		msg.ID, msg.IntentID, msg.CorrelationID, msg.CausationID, msg.Producer,
		msg.SpaceID, msg.SourceMemberID, msg.DestinationMemberID, msg.ChannelID,
		msg.Kind, msg.Subject, body, msg.TaskRef, msg.Status, formatTime(msg.VisibleAt),
		metadata, formatTime(msg.CreatedAt), formatTime(msg.UpdatedAt), formatOptionalTime(msg.ConsumedAt), msg.ConsumedBy, payload,
	)
	if err != nil {
		return types.AgentMessage{}, fmt.Errorf("save queued message %s: %w", msg.ID, err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return r.getByDestinationIntent(ctx, msg.DestinationMemberID, msg.IntentID)
	}
	return r.Get(ctx, msg.ID)
}

func (r *sqlStore) NextQueuedForMember(ctx context.Context, memberID member.ID, now time.Time) (types.AgentMessage, error) {
	memberID = member.ID(strings.TrimSpace(string(memberID)))
	if memberID == "" {
		return types.AgentMessage{}, fmt.Errorf("member id is required")
	}
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT message_json
		FROM agent_messages
		WHERE destination_member_id = ?
		  AND status = ?
		  AND visible_at <= ?
		ORDER BY visible_at ASC, created_at ASC, message_id ASC
		LIMIT 1
	`), memberID, types.MessageStatusQueuedTyped, formatTime(now.UTC()))
	msg, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.AgentMessage{}, domain.ErrMessageNotFound
		}
		return types.AgentMessage{}, fmt.Errorf("next queued message for %s: %w", memberID, err)
	}
	return msg, nil
}

func (r *sqlStore) DeferQueued(ctx context.Context, messageID types.AgentMessageID, visibleAt time.Time, updatedAt time.Time) (types.AgentMessage, error) {
	messageID = types.AgentMessageID(strings.TrimSpace(string(messageID)))
	if messageID == "" {
		return types.AgentMessage{}, fmt.Errorf("message id is required")
	}
	msg, err := r.Get(ctx, messageID)
	if err != nil {
		return types.AgentMessage{}, err
	}
	if msg.Status != types.MessageStatusQueuedTyped {
		return types.AgentMessage{}, domain.ErrConsumed
	}
	msg.VisibleAt = visibleAt.UTC()
	msg.UpdatedAt = updatedAt.UTC()
	payload, body, metadata, err := marshalMessage(msg)
	if err != nil {
		return types.AgentMessage{}, err
	}
	res, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE agent_messages
		SET visible_at = ?, updated_at = ?, body_json = ?, metadata_json = ?, message_json = ?
		WHERE message_id = ?
		  AND status = ?
	`), formatTime(msg.VisibleAt), formatTime(msg.UpdatedAt), body, metadata, payload, msg.ID, types.MessageStatusQueuedTyped)
	if err != nil {
		return types.AgentMessage{}, fmt.Errorf("defer queued message %s: %w", msg.ID, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return types.AgentMessage{}, err
	}
	if rows == 0 {
		return types.AgentMessage{}, domain.ErrConsumed
	}
	return r.Get(ctx, msg.ID)
}

func (r *sqlStore) MarkConsumed(ctx context.Context, msg types.AgentMessage) (types.AgentMessage, error) {
	msg = msg.Normalized()
	if msg.ID == "" {
		return types.AgentMessage{}, fmt.Errorf("message id is required")
	}
	if msg.Status != types.MessageStatusConsumedTyped {
		return types.AgentMessage{}, fmt.Errorf("message status must be consumed")
	}
	if msg.ConsumedBy == "" {
		return types.AgentMessage{}, fmt.Errorf("consumed by is required")
	}
	if msg.ConsumedAt == nil || msg.ConsumedAt.IsZero() {
		return types.AgentMessage{}, fmt.Errorf("consumed at is required")
	}
	payload, body, metadata, err := marshalMessage(msg)
	if err != nil {
		return types.AgentMessage{}, err
	}
	res, err := r.db.ExecContext(ctx, r.rebind(`
		UPDATE agent_messages
		SET status = ?, consumed_at = ?, consumed_by = ?, updated_at = ?,
		    body_json = ?, metadata_json = ?, message_json = ?
		WHERE message_id = ?
		  AND status = ?
	`), msg.Status, formatOptionalTime(msg.ConsumedAt), msg.ConsumedBy, formatTime(msg.UpdatedAt),
		body, metadata, payload, msg.ID, types.MessageStatusQueuedTyped)
	if err != nil {
		return types.AgentMessage{}, fmt.Errorf("mark message consumed %s: %w", msg.ID, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return types.AgentMessage{}, err
	}
	if rows == 0 {
		return types.AgentMessage{}, domain.ErrConsumed
	}
	return r.Get(ctx, msg.ID)
}

func (r *sqlStore) Get(ctx context.Context, id types.AgentMessageID) (types.AgentMessage, error) {
	id = types.AgentMessageID(strings.TrimSpace(string(id)))
	if id == "" {
		return types.AgentMessage{}, fmt.Errorf("message id is required")
	}
	row := r.db.QueryRowContext(ctx, r.rebind(`SELECT message_json FROM agent_messages WHERE message_id = ?`), id)
	msg, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.AgentMessage{}, domain.ErrMessageNotFound
		}
		return types.AgentMessage{}, fmt.Errorf("get message %s: %w", id, err)
	}
	return msg, nil
}

func (r *sqlStore) List(ctx context.Context, filter domain.MessageFilter) ([]types.AgentMessage, error) {
	where, args, err := messageWhere(filter)
	if err != nil {
		return nil, err
	}
	query := `SELECT message_json FROM agent_messages` + where + ` ORDER BY created_at DESC, message_id ASC`
	if filter.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()
	var out []types.AgentMessage
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *sqlStore) Count(ctx context.Context, filter domain.MessageFilter) (int, error) {
	where, args, err := messageWhere(filter)
	if err != nil {
		return 0, err
	}
	var count int
	if err := r.db.QueryRowContext(ctx, r.rebind(`SELECT COUNT(*) FROM agent_messages`+where), args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return count, nil
}

func (r *sqlStore) Save(ctx context.Context, ch types.Channel) (types.Channel, error) {
	ch = channel.WrapChannel(ch).Inner()
	if err := validateChannelForSave(ch); err != nil {
		return types.Channel{}, err
	}
	payload, err := json.Marshal(ch)
	if err != nil {
		return types.Channel{}, fmt.Errorf("marshal channel: %w", err)
	}
	_, err = r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO message_channels (
			channel_id, space_id, project_id, member_id, status,
			last_message_at, created_at, updated_at, channel_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			space_id = excluded.space_id,
			project_id = excluded.project_id,
			member_id = excluded.member_id,
			status = excluded.status,
			last_message_at = excluded.last_message_at,
			updated_at = excluded.updated_at,
			channel_json = excluded.channel_json
	`), ch.ID, ch.SpaceID, ch.ProjectID, ch.MemberID, ch.Status,
		formatOptionalTime(ch.LastMessageAt), formatTime(ch.CreatedAt), formatTime(ch.UpdatedAt), string(payload))
	if err != nil {
		return types.Channel{}, fmt.Errorf("save channel %s: %w", ch.ID, err)
	}
	return r.Load(ctx, ch.ID)
}

func (r *sqlStore) Load(ctx context.Context, channelID types.ChannelID) (types.Channel, error) {
	channelID = types.ChannelID(strings.TrimSpace(string(channelID)))
	if channelID == "" {
		return types.Channel{}, fmt.Errorf("channel id is required")
	}
	row := r.db.QueryRowContext(ctx, r.rebind(`SELECT channel_json, last_message_at FROM message_channels WHERE channel_id = ?`), channelID)
	ch, err := scanChannel(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Channel{}, channel.ErrNotFound
		}
		return types.Channel{}, fmt.Errorf("load channel %s: %w", channelID, err)
	}
	return ch, nil
}

func (r *sqlStore) LoadMemberChannel(ctx context.Context, spaceID spacedomain.SpaceID, memberID member.ID) (types.Channel, error) {
	return r.Load(ctx, channel.MemberChannelID(spaceID, memberID))
}

func (r *sqlStore) ListBySpace(ctx context.Context, spaceID spacedomain.SpaceID) ([]types.Channel, error) {
	spaceID = spacedomain.SpaceID(strings.TrimSpace(string(spaceID)))
	if spaceID == "" {
		return nil, fmt.Errorf("space id is required")
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT channel_json, last_message_at
		FROM message_channels
		WHERE space_id = ?
		ORDER BY created_at ASC, channel_id ASC
	`), spaceID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()
	var out []types.Channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *sqlStore) RecordActivity(ctx context.Context, channelID types.ChannelID, at time.Time) error {
	channelID = types.ChannelID(strings.TrimSpace(string(channelID)))
	if channelID == "" {
		return fmt.Errorf("channel id is required")
	}
	if at.IsZero() {
		return fmt.Errorf("activity timestamp is required")
	}
	loaded, err := r.Load(ctx, channelID)
	if err != nil {
		return err
	}
	next, err := channel.WrapChannel(loaded).MarkActivity(at)
	if err != nil {
		return err
	}
	_, err = r.Save(ctx, next.Inner())
	return err
}

func (r *sqlStore) MarkRead(ctx context.Context, userID string, channelID types.ChannelID, at time.Time) error {
	userID = strings.TrimSpace(userID)
	channelID = types.ChannelID(strings.TrimSpace(string(channelID)))
	if userID == "" {
		return fmt.Errorf("user id is required")
	}
	if channelID == "" {
		return fmt.Errorf("channel id is required")
	}
	if at.IsZero() {
		return fmt.Errorf("seen timestamp is required")
	}
	ts := formatTime(at.UTC())
	_, err := r.db.ExecContext(ctx, r.rebind(`
		INSERT INTO message_channel_reads (user_id, channel_id, last_seen_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, channel_id) DO UPDATE SET
			last_seen_at = CASE
				WHEN excluded.last_seen_at > message_channel_reads.last_seen_at THEN excluded.last_seen_at
				ELSE message_channel_reads.last_seen_at
			END
	`), userID, channelID, ts)
	if err != nil {
		return fmt.Errorf("mark channel read: %w", err)
	}
	return nil
}

func (r *sqlStore) DeleteForMember(ctx context.Context, spaceID spacedomain.SpaceID, memberID member.ID) error {
	spaceID = spacedomain.SpaceID(strings.TrimSpace(string(spaceID)))
	memberID = member.ID(strings.TrimSpace(string(memberID)))
	if spaceID == "" {
		return fmt.Errorf("space id is required")
	}
	if memberID == "" {
		return fmt.Errorf("member id is required")
	}
	res, err := r.db.ExecContext(ctx, r.rebind(`DELETE FROM message_channels WHERE space_id = ? AND member_id = ?`), spaceID, memberID)
	if err != nil {
		return fmt.Errorf("delete member channel: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return channel.ErrNotFound
	}
	return nil
}

func (r *sqlStore) UnreadCountsByChannel(ctx context.Context, userID string, channelIDs []types.ChannelID) (map[types.ChannelID]int, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user id is required")
	}
	out := make(map[types.ChannelID]int, len(channelIDs))
	ids := cleanChannelIDs(channelIDs)
	if len(ids) == 0 {
		return out, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+1)
	args = append(args, userID)
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.db.QueryContext(ctx, r.rebind(`
		SELECT c.channel_id, COUNT(m.message_id)
		FROM message_channels c
		LEFT JOIN message_channel_reads r
		  ON r.channel_id = c.channel_id
		 AND r.user_id = ?
		LEFT JOIN agent_messages m
		  ON m.channel_id = c.channel_id
		 AND m.created_at > COALESCE(r.last_seen_at, '')
		WHERE c.channel_id IN (`+placeholders+`)
		GROUP BY c.channel_id
	`), args...)
	if err != nil {
		return nil, fmt.Errorf("query unread channel counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id types.ChannelID
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		out[id] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *sqlStore) getByDestinationIntent(ctx context.Context, memberID member.ID, intentID types.IntentID) (types.AgentMessage, error) {
	row := r.db.QueryRowContext(ctx, r.rebind(`
		SELECT message_json
		FROM agent_messages
		WHERE destination_member_id = ?
		  AND intent_id = ?
	`), memberID, intentID)
	msg, err := scanMessage(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.AgentMessage{}, domain.ErrMessageNotFound
		}
		return types.AgentMessage{}, err
	}
	return msg, nil
}

func (r *sqlStore) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}
