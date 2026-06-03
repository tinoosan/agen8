package infra

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// ProjectionUpsertFunc is called inside the append transaction to maintain
// derived read models for agent spaces.
type ProjectionUpsertFunc func(tx *sql.Tx, dialect storagedb.Dialect, runID string, eventSeq int64, ev types.EventRecord) error

// SQLiteRepository implements domain.EventRepository backed by SQLite.
type SQLiteRepository struct {
	db          *sql.DB
	dialect     storagedb.Dialect
	spaceUpsert ProjectionUpsertFunc
}

var _ domain.EventRepository = (*SQLiteRepository)(nil)

// Option configures a SQLiteRepository.
type Option func(*SQLiteRepository)

// WithSpaceEntryUpsert injects the agent-space projection upsert callback.
func WithSpaceEntryUpsert(fn ProjectionUpsertFunc) Option {
	return func(r *SQLiteRepository) { r.spaceUpsert = fn }
}

func NewSQLRepository(handle *storagedb.Handle, opts ...Option) *SQLiteRepository {
	r := &SQLiteRepository{db: handle.DB(), dialect: handle.Dialect()}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r *SQLiteRepository) rebind(query string) string {
	return storagedb.Rebind(query, r.dialect)
}

// Append persists an already-normalized event within a transaction.
func (r *SQLiteRepository) Append(ctx context.Context, event types.EventRecord) error {
	runID := strings.TrimSpace(string(event.RunID))
	if runID == "" {
		return domain.ErrRunIDRequired
	}
	if strings.TrimSpace(event.Type) == "" {
		return fmt.Errorf("append event: eventType is required")
	}
	if strings.TrimSpace(event.Message) == "" {
		return fmt.Errorf("append event: message is required")
	}

	if err := ensureRunExists(r.db, r.dialect, runID); err != nil {
		return err
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshalling event: %w", err)
	}

	dataJSON := ""
	if len(event.Data) > 0 {
		dbuf, err := json.Marshal(event.Data)
		if err != nil {
			return fmt.Errorf("error marshalling event data: %w", err)
		}
		dataJSON = string(dbuf)
	}

	return r.appendOnce(ctx, runID, event, string(eventJSON), dataJSON)
}

func (r *SQLiteRepository) appendOnce(ctx context.Context, runID string, event types.EventRecord, eventJSON, dataJSON string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	severity := domain.EventSeverity(event.Type, event.Data)
	category := domain.EventCategory(event.Type)
	origin := strings.TrimSpace(event.Origin)

	insertSQL := `INSERT INTO events (event_id, run_id, ts, type, message, data_json, event_json, severity, category, origin)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	args := []any{
		event.EventID,
		runID,
		event.Timestamp.UTC().Format(time.RFC3339Nano),
		event.Type,
		event.Message,
		nullIfEmpty(dataJSON),
		eventJSON,
		severity,
		category,
		origin,
	}
	var eventSeq int64
	if r.dialect != nil && r.dialect.Placeholder(1) != "?" {
		if err := tx.QueryRowContext(ctx, r.rebind(insertSQL+` RETURNING seq`), args...).Scan(&eventSeq); err != nil {
			return fmt.Errorf("error writing event for run %s: %w", runID, err)
		}
	} else {
		res, err := tx.ExecContext(ctx, r.rebind(insertSQL), args...)
		if err != nil {
			return fmt.Errorf("error writing event for run %s: %w", runID, err)
		}
		eventSeq, err = res.LastInsertId()
		if err != nil {
			return fmt.Errorf("last insert id for event: %w", err)
		}
	}

	if r.spaceUpsert != nil {
		if err := r.spaceUpsert(tx, r.dialect, runID, eventSeq, event); err != nil {
			return fmt.Errorf("error upserting agent space entry: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing event append: %w", err)
	}
	return nil
}

// ListPaginated returns events with server-side pagination.
func (r *SQLiteRepository) ListPaginated(ctx context.Context, filter domain.EventFilter) ([]types.EventRecord, int64, error) {
	runID := strings.TrimSpace(filter.RunID)
	if runID == "" {
		return nil, 0, fmt.Errorf("runID is required in EventFilter")
	}

	whereClause, args := buildFilterClause(runID, filter)
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

	rows, err := r.db.QueryContext(ctx, r.rebind(query), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list events paginated: %w", err)
	}
	defer rows.Close()

	events := make([]types.EventRecord, 0, limit)
	var maxSeq, minSeq int64
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
		if string(event.RunID) != runID {
			return nil, 0, fmt.Errorf("runID mismatch: expected %s, got %s", runID, string(event.RunID))
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

// Count returns the total number of events matching the filter.
func (r *SQLiteRepository) Count(ctx context.Context, filter domain.EventFilter) (int, error) {
	runID := strings.TrimSpace(filter.RunID)
	if runID == "" {
		return 0, fmt.Errorf("runID is required in EventFilter")
	}

	whereClause, args := buildFilterClause(runID, filter)
	query := `SELECT COUNT(*) FROM events WHERE ` + whereClause

	var count int
	if err := r.db.QueryRowContext(ctx, r.rebind(query), args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// LatestSeq returns the maximum seq number for a run.
func (r *SQLiteRepository) LatestSeq(ctx context.Context, runID string) (int64, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return 0, domain.ErrRunIDRequired
	}

	var seq int64
	if err := r.db.QueryRowContext(ctx, r.rebind(`SELECT COALESCE(MAX(seq), 0) FROM events WHERE run_id = ?`), runID).Scan(&seq); err != nil {
		return 0, err
	}
	return seq, nil
}

// Tail streams events for a run from a specific offset, polling every 100ms.
func (r *SQLiteRepository) Tail(ctx context.Context, runID string, fromOffset int64) (<-chan domain.TailedEvent, <-chan error) {
	eventCh := make(chan domain.TailedEvent)
	errCh := make(chan error, 1)
	go func() {
		defer close(eventCh)
		defer close(errCh)

		if fromOffset < 0 {
			errCh <- fmt.Errorf("fromOffset cannot be negative")
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
				rows, err := r.db.QueryContext(
					ctx,
					r.rebind(`SELECT seq, event_json FROM events WHERE run_id = ? AND seq > ? ORDER BY seq LIMIT ?`),
					runID,
					currentOffset,
					tailBatchLimit,
				)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
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
					if string(event.RunID) != runID {
						rows.Close()
						errCh <- fmt.Errorf("runID mismatch: expected %s, got %s", runID, string(event.RunID))
						return
					}
					currentOffset = seq
					select {
					case <-ctx.Done():
						rows.Close()
						return
					case eventCh <- domain.TailedEvent{
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

// --- Inlined helpers ---

func buildFilterClause(runID string, filter domain.EventFilter) (string, []any) {
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

	if len(filter.Severities) > 0 {
		placeholders := make([]string, len(filter.Severities))
		for i := range filter.Severities {
			placeholders[i] = "?"
		}
		clause += ` AND severity IN (` + strings.Join(placeholders, ",") + `)`
		for _, s := range filter.Severities {
			args = append(args, s)
		}
	}
	if len(filter.Categories) > 0 {
		placeholders := make([]string, len(filter.Categories))
		for i := range filter.Categories {
			placeholders[i] = "?"
		}
		clause += ` AND category IN (` + strings.Join(placeholders, ",") + `)`
		for _, c := range filter.Categories {
			args = append(args, c)
		}
	}
	if filter.Search != "" {
		clause += ` AND (message LIKE ? COLLATE NOCASE OR type LIKE ? COLLATE NOCASE)`
		search := "%" + filter.Search + "%"
		args = append(args, search, search)
	}
	if filter.Origin != "" {
		clause += ` AND origin = ?`
		args = append(args, filter.Origin)
	}

	return clause, args
}

func ensureRunExists(db *sql.DB, dialect storagedb.Dialect, runID string) error {
	if strings.TrimSpace(runID) == "" {
		return fmt.Errorf("runID cannot be blank")
	}
	var exists int
	if err := db.QueryRow(storagedb.Rebind(`SELECT 1 FROM runs WHERE run_id = ? LIMIT 1`, dialect), runID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot append event, run %s does not exist: %w", runID, os.ErrNotExist)
		}
		return fmt.Errorf("cannot append event, error reading run %s: %w", runID, err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
