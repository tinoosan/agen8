package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/types"
)

type TailedEvent struct {
	Event      types.EventRecord
	NextOffset int64
}

// EventFilter specifies filtering and pagination for event queries.
type EventFilter struct {
	RunID string // required: filter by run

	// Pagination
	Limit     int   // max results (default: 100, 0 = use default)
	Offset    int   // skip N events (for page-based pagination)
	AfterSeq  int64 // return events with seq > AfterSeq (for cursor-based pagination)
	BeforeSeq int64 // return events with seq < BeforeSeq (for reverse pagination)

	// Filtering by event type
	Types []string // filter to specific event types (empty = all types)

	// Sorting
	SortDesc bool // true = newest first (DESC), false = oldest first (ASC, default)
}

// Event storage overview
//
// Canonical event log
//   - AppendEvent writes an event record into SQLite.
//
// Activity index
//   - AppendEvent also maintains a compact activities index in SQLite so UIs can paginate
//     activity history without scanning the full event log.

// AppendEvent records a new event for the specified run.
// It validates inputs, ensures the run exists, and appends the event to SQLite.
// On success, the event and any derived indexes are persisted to SQLite.
func AppendEvent(ctx context.Context, cfg config.Config, event types.EventRecord) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	runID := strings.TrimSpace(event.RunID)
	if runID == "" {
		return fmt.Errorf("error appending event, runID cannot be blank")
	}

	eventType := strings.TrimSpace(event.Type)
	if eventType == "" {
		return fmt.Errorf("error appending event, eventType cannot be blank")
	}

	message := strings.TrimSpace(event.Message)
	if message == "" {
		return fmt.Errorf("error appending event, message cannot be blank")
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return err
	}
	if err := ensureRunExists(db, runID); err != nil {
		return err
	}

	origin := strings.TrimSpace(event.Origin)
	data := event.Data
	if data != nil {
		// Back-compat: accept "origin" stored in the Data map, but do not persist it there.
		if rawOrigin, ok := data["origin"]; ok {
			if origin == "" {
				origin = strings.TrimSpace(rawOrigin)
			}
			// Never mutate the input map.
			if len(data) == 1 {
				data = nil
			} else {
				out := make(map[string]string, len(data)-1)
				for k, v := range data {
					if k == "origin" {
						continue
					}
					out[k] = v
				}
				data = out
			}
		}
	}

	if strings.TrimSpace(event.EventID) == "" {
		event.EventID = "event-" + uuid.NewString()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	event.RunID = runID
	event.Type = eventType
	event.Message = message
	event.Data = data
	event.Origin = origin

	b, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshalling event: %w", err)
	}

	dataJSON := ""
	if len(data) > 0 {
		if dbuf, err := json.Marshal(data); err == nil {
			dataJSON = string(dbuf)
		} else {
			return fmt.Errorf("error marshalling event data: %w", err)
		}
	}

	txCtx := ctx
	if txCtx == nil {
		txCtx = context.Background()
	}
	tx, err := db.BeginTx(txCtx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(
		`INSERT INTO events (event_id, run_id, ts, type, message, data_json, event_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.EventID,
		runID,
		event.Timestamp.UTC().Format(time.RFC3339Nano),
		event.Type,
		event.Message,
		nullIfEmpty(dataJSON),
		string(b),
	)
	if err != nil {
		return fmt.Errorf("error writing event for run %s: %w", runID, err)
	}
	eventSeq, _ := res.LastInsertId()

	// Maintain Activity index for agent operations.
	if err := upsertActivityFromEventTx(tx, runID, eventSeq, event); err != nil {
		return fmt.Errorf("error upserting activity: %w", err)
	}

	//f.Sync()
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing event append: %w", err)
	}
	return nil

}

// ListEvents retrieves all recorded events for a given run ID.
// It reads from SQLite, validates each entry, and returns them in order.
func ListEvents(cfg config.Config, runID string) ([]types.EventRecord, int64, error) {
	if err := cfg.Validate(); err != nil {
		return nil, 0, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, 0, err
	}
	events := make([]types.EventRecord, 0)
	rows, err := db.Query(`SELECT event_json FROM events WHERE run_id = ? ORDER BY seq`, runID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	lineNum := 0
	for rows.Next() {
		lineNum++
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		var event types.EventRecord
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, 0, fmt.Errorf("error reading event at row %d: %w", lineNum, err)
		}
		if event.RunID != runID {
			return nil, 0, fmt.Errorf("error reading event at row %d: runID mismatch", lineNum)
		}
		if event.EventID == "" {
			return nil, 0, fmt.Errorf("error reading event at row %d: eventId cannot be blank", lineNum)
		}
		if event.Timestamp.IsZero() {
			return nil, 0, fmt.Errorf("error reading event at row %d: timestamp cannot be zero", lineNum)
		}
		if event.Type == "" {
			return nil, 0, fmt.Errorf("error reading event at row %d: type cannot be blank", lineNum)
		}
		if event.Message == "" {
			return nil, 0, fmt.Errorf("error reading event at row %d: message cannot be blank", lineNum)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	var nextOffset int64
	if err := db.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM events WHERE run_id = ?`, runID).Scan(&nextOffset); err != nil {
		return nil, 0, err
	}
	return events, nextOffset, nil
}

// buildEventFilterClause builds the shared WHERE clause fragment and query args
// for filtering events by run ID, cursor bounds (AfterSeq/BeforeSeq), and type list.
// It returns a clause like " AND seq > ? AND type IN (?,?)" and the corresponding args
// (the initial "run_id = ?" / runID arg is included as the first element).
func buildEventFilterClause(runID string, filter EventFilter) (string, []any) {
	clause := `run_id = ?`
	args := []any{runID}

	if filter.AfterSeq > 0 {
		clause += ` AND seq > ?`
		args = append(args, filter.AfterSeq)
	}
	if filter.BeforeSeq > 0 {
		clause += ` AND seq < ?`
		args = append(args, filter.BeforeSeq)
	}

	typesFilter := make([]string, 0, len(filter.Types))
	seenTypes := make(map[string]struct{}, len(filter.Types))
	for _, t := range filter.Types {
		tt := strings.TrimSpace(t)
		if tt == "" {
			continue
		}
		if _, ok := seenTypes[tt]; ok {
			continue
		}
		seenTypes[tt] = struct{}{}
		typesFilter = append(typesFilter, tt)
	}
	if len(typesFilter) > 0 {
		placeholders := make([]string, len(typesFilter))
		for i, t := range typesFilter {
			placeholders[i] = "?"
			args = append(args, t)
		}
		clause += fmt.Sprintf(` AND type IN (%s)`, strings.Join(placeholders, ","))
	}

	return clause, args
}

// ListEventsPaginated returns events with server-side pagination.
// Uses SQL LIMIT/OFFSET or cursor-based pagination for efficient querying.
//
// The returned cursor is suitable for chaining:
//   - ASC: use it as AfterSeq for the next page.
//   - DESC: use it as BeforeSeq for the next page.
func ListEventsPaginated(cfg config.Config, filter EventFilter) ([]types.EventRecord, int64, error) {
	if err := cfg.Validate(); err != nil {
		return nil, 0, err
	}

	runID := strings.TrimSpace(filter.RunID)
	if runID == "" {
		return nil, 0, fmt.Errorf("runID is required in EventFilter")
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, 0, err
	}

	whereClause, args := buildEventFilterClause(runID, filter)
	query := `SELECT seq, event_json FROM events WHERE ` + whereClause

	if filter.SortDesc {
		query += ` ORDER BY seq DESC`
	} else {
		query += ` ORDER BY seq ASC`
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	query += ` LIMIT ?`
	args = append(args, limit)

	if filter.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, filter.Offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list events paginated: %w", err)
	}
	defer rows.Close()

	events := make([]types.EventRecord, 0, limit)
	var maxSeq int64
	var minSeq int64
	for rows.Next() {
		var seq int64
		var raw string
		if err := rows.Scan(&seq, &raw); err != nil {
			return nil, 0, err
		}
		var event types.EventRecord
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, 0, fmt.Errorf("unmarshal event: %w", err)
		}
		if event.RunID != runID {
			return nil, 0, fmt.Errorf("runID mismatch: expected %s, got %s", runID, event.RunID)
		}
		events = append(events, event)
		if maxSeq == 0 || seq > maxSeq {
			maxSeq = seq
		}
		if minSeq == 0 || seq < minSeq {
			minSeq = seq
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if filter.SortDesc {
		return events, minSeq, nil
	}
	return events, maxSeq, nil
}

// CountEvents returns the total number of events matching the filter.
// Used for pagination UI ("showing X of Y events").
func CountEvents(cfg config.Config, filter EventFilter) (int, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}

	runID := strings.TrimSpace(filter.RunID)
	if runID == "" {
		return 0, fmt.Errorf("runID is required in EventFilter")
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return 0, err
	}

	whereClause, args := buildEventFilterClause(runID, filter)
	query := `SELECT COUNT(*) FROM events WHERE ` + whereClause

	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// GetLatestEventSeq returns the maximum seq number for a run without loading events.
// Useful for getting the tail offset without loading all events into memory.
func GetLatestEventSeq(cfg config.Config, runID string) (int64, error) {
	if err := cfg.Validate(); err != nil {
		return 0, err
	}

	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, fmt.Errorf("runID cannot be blank")
	}

	db, err := getSQLiteDB(cfg)
	if err != nil {
		return 0, err
	}

	var seq int64
	if err := db.QueryRow(`SELECT COALESCE(MAX(seq), 0) FROM events WHERE run_id = ?`, runID).Scan(&seq); err != nil {
		return 0, err
	}
	return seq, nil
}

// TailEvents streams events for a given run from a specific byte offset.
// It follows the file and sends new events as they are appended.
// Each TailedEvent includes the NextOffset for resuming after refresh.
// The caller should cancel the context to stop tailing.
// Both returned channels are closed when the function exits.
func TailEvents(cfg config.Config, ctx context.Context, runID string, fromOffset int64) (<-chan TailedEvent, <-chan error) {
	eventCh := make(chan TailedEvent)
	errCh := make(chan error, 1)

	if err := cfg.Validate(); err != nil {
		errCh <- err
		close(eventCh)
		close(errCh)
		return eventCh, errCh
	}

	go func() {
		defer close(eventCh)
		defer close(errCh)

		// Validate offset is non-negative (always, regardless of run existence)
		if fromOffset < 0 {
			errCh <- fmt.Errorf("fromOffset cannot be negative")
			return
		}

		db, err := getSQLiteDB(cfg)
		if err != nil {
			errCh <- err
			return
		}
		currentOffset := fromOffset
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				const tailBatchLimit = 100
				rows, err := db.Query(
					`SELECT seq, event_json FROM events WHERE run_id = ? AND seq > ? ORDER BY seq LIMIT ?`,
					runID,
					currentOffset,
					tailBatchLimit,
				)
				if err != nil {
					errCh <- err
					return
				}
				for rows.Next() {
					var seq int64
					var raw string
					if err := rows.Scan(&seq, &raw); err != nil {
						rows.Close()
						errCh <- err
						return
					}
					var event types.EventRecord
					if err := json.Unmarshal([]byte(raw), &event); err != nil {
						rows.Close()
						errCh <- fmt.Errorf("error unmarshalling event: %w", err)
						return
					}
					if event.RunID != runID {
						rows.Close()
						errCh <- fmt.Errorf("runID mismatch: expected %s, got %s", runID, event.RunID)
						return
					}
					currentOffset = seq
					select {
					case <-ctx.Done():
						rows.Close()
						return
					case eventCh <- TailedEvent{
						Event:      event,
						NextOffset: currentOffset,
					}:
					}
				}
				if err := rows.Err(); err != nil {
					rows.Close()
					errCh <- err
					return
				}
				rows.Close()
			}
		}
	}()

	return eventCh, errCh
}

func ensureRunExists(db *sql.DB, runID string) error {
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("runID cannot be blank")
	}
	var exists int
	if err := db.QueryRow(`SELECT 1 FROM runs WHERE run_id = ? LIMIT 1`, runID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot append event, run %s does not exist: %w", runID, errors.Join(ErrNotFound, os.ErrNotExist))
		}
		return fmt.Errorf("cannot append event, error reading run %s: %w", runID, err)
	}
	return nil
}
