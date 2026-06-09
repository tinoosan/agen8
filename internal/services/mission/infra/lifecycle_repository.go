package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/internal/core/types"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	"github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func (r *SQLiteRepository) AppendLifecycleEvent(ctx context.Context, event types.EventRecord) error {
	return appendLifecycleEvent(ctx, r.db, nil, event)
}

func (r *SQLiteRepository) ListLifecycleEvents(ctx context.Context, missionID mission.MissionID, filter missionapp.LifecycleHistoryFilter) ([]types.EventRecord, int, error) {
	return listLifecycleEvents(ctx, r.db, nil, missionID, filter)
}

func (r *PostgresRepository) AppendLifecycleEvent(ctx context.Context, event types.EventRecord) error {
	return appendLifecycleEvent(ctx, r.db, r.dialect, event)
}

func (r *PostgresRepository) ListLifecycleEvents(ctx context.Context, missionID mission.MissionID, filter missionapp.LifecycleHistoryFilter) ([]types.EventRecord, int, error) {
	return listLifecycleEvents(ctx, r.db, r.dialect, missionID, filter)
}

func appendLifecycleEvent(ctx context.Context, db *sql.DB, dialect storagedb.Dialect, event types.EventRecord) error {
	event, missionID, keyResultID, err := normalizeLifecycleEvent(event)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal lifecycle event: %w", err)
	}
	query := storagedb.Rebind(`
		INSERT INTO mission_lifecycle_events (
			event_id, mission_id, key_result_id, event_type, created_at, event_json
		) VALUES (?, ?, ?, ?, ?, ?)
	`, dialect)
	if _, err := db.ExecContext(ctx, query, string(event.EventID), missionID, keyResultID, event.Type, event.CreatedAt, string(payload)); err != nil {
		return fmt.Errorf("append lifecycle event %s: %w", event.EventID, err)
	}
	return nil
}

func listLifecycleEvents(ctx context.Context, db *sql.DB, dialect storagedb.Dialect, missionID mission.MissionID, filter missionapp.LifecycleHistoryFilter) ([]types.EventRecord, int, error) {
	missionID = mission.MissionID(strings.TrimSpace(string(missionID)))
	if missionID == "" {
		return nil, 0, fmt.Errorf("mission id is required")
	}
	if filter.Limit < 0 {
		return nil, 0, fmt.Errorf("lifecycle history limit must be non-negative")
	}
	if filter.Offset < 0 {
		return nil, 0, fmt.Errorf("lifecycle history offset must be non-negative")
	}
	where, args := lifecycleWhere(missionID, filter.Types)
	countQuery := storagedb.Rebind("SELECT COUNT(*) FROM mission_lifecycle_events"+where, dialect)
	var count int
	if err := db.QueryRowContext(ctx, countQuery, args...).Scan(&count); err != nil {
		return nil, 0, fmt.Errorf("count lifecycle events: %w", err)
	}
	query := "SELECT event_json FROM mission_lifecycle_events" + where + " ORDER BY created_at DESC, event_id DESC"
	queryArgs := append([]any(nil), args...)
	if filter.Limit > 0 {
		query += " LIMIT ?"
		queryArgs = append(queryArgs, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		queryArgs = append(queryArgs, filter.Offset)
	}
	rows, err := db.QueryContext(ctx, storagedb.Rebind(query, dialect), queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list lifecycle events: %w", err)
	}
	defer rows.Close()
	events := make([]types.EventRecord, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, fmt.Errorf("scan lifecycle event: %w", err)
		}
		var event types.EventRecord
		if err := json.Unmarshal(raw, &event); err != nil {
			return nil, 0, fmt.Errorf("unmarshal lifecycle event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate lifecycle events: %w", err)
	}
	return events, count, nil
}

func normalizeLifecycleEvent(event types.EventRecord) (types.EventRecord, string, string, error) {
	missionID := strings.TrimSpace(event.Data["missionId"])
	if missionID == "" {
		missionID = strings.TrimSpace(string(event.RunID))
	}
	if missionID == "" {
		return types.EventRecord{}, "", "", fmt.Errorf("lifecycle event mission id is required")
	}
	if strings.TrimSpace(event.Type) == "" {
		return types.EventRecord{}, "", "", fmt.Errorf("lifecycle event type is required")
	}
	if strings.TrimSpace(event.Message) == "" {
		return types.EventRecord{}, "", "", fmt.Errorf("lifecycle event message is required")
	}
	if strings.TrimSpace(string(event.EventID)) == "" {
		event.EventID = types.EventID("event-" + uuid.NewString())
	}
	if strings.TrimSpace(event.CreatedAt) == "" {
		event.CreatedAt = timeString(time.Now().UTC())
	}
	if event.Data == nil {
		event.Data = map[string]string{}
	}
	event.Data["missionId"] = missionID
	event.RunID = types.RunID(missionID)
	return event, missionID, strings.TrimSpace(event.Data["keyResultId"]), nil
}

func lifecycleWhere(missionID mission.MissionID, eventTypes []string) (string, []any) {
	args := []any{string(missionID)}
	where := " WHERE mission_id = ?"
	cleanTypes := make([]string, 0, len(eventTypes))
	for _, raw := range eventTypes {
		if eventType := strings.TrimSpace(raw); eventType != "" {
			cleanTypes = append(cleanTypes, eventType)
		}
	}
	if len(cleanTypes) == 0 {
		return where, args
	}
	placeholders := make([]string, 0, len(cleanTypes))
	for _, eventType := range cleanTypes {
		placeholders = append(placeholders, "?")
		args = append(args, eventType)
	}
	where += " AND event_type IN (" + strings.Join(placeholders, ", ") + ")"
	return where, args
}
