package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type TailedEvent struct {
	Event      types.EventRecord
	NextOffset int64
}

// Event storage overview
//
// Canonical event log
//   - AppendEvent writes an event as one JSON object per line (JSONL) into SQLite.
//
// Trace mirror
//   - AppendEvent also mirrors the exact same bytes (including newline) into:
//       data/runs/<runId>/log/events.jsonl
//     so the trace VFS mount can be self-contained and offset-based polling is stable.
//
// Offset semantics
//   - ListEvents returns nextOffset as the last SQLite sequence id.
//   - That offset can be used later to fetch only new events via TailEvents.

// AppendEvent records a new event for the specified run.
// It validates inputs, ensures the run exists, and appends the event to SQLite.
// On success, the event is persisted to SQLite. A best-effort mirror is written to the
// run's trace log; mirror failure is logged but does not fail AppendEvent.
func AppendEvent(cfg config.Config, runID, eventType, message string, data map[string]string) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	if runID == "" {
		return fmt.Errorf("error appending event, runID cannot be blank")
	}

	if eventType == "" {
		return fmt.Errorf("error appending event, eventType cannot be blank")
	}

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

	event := types.NewEventRecord(runID, eventType, message, data)
	b, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshalling event: %w", err)
	}

	dataJSON := ""
	if len(event.Data) > 0 {
		if dbuf, err := json.Marshal(event.Data); err == nil {
			dataJSON = string(dbuf)
		} else {
			return fmt.Errorf("error marshalling event data: %w", err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO events (event_id, run_id, ts, type, message, data_json, event_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.EventID,
		runID,
		event.Timestamp.UTC().Format(time.RFC3339Nano),
		event.Type,
		event.Message,
		nullIfEmpty(dataJSON),
		string(b),
	); err != nil {
		return fmt.Errorf("error writing event for run %s: %w", runID, err)
	}

	if err := mirrorTraceEvent(cfg.DataDir, runID, b); err != nil {
		log.Printf("store: warning: failed to mirror trace event for run %s: %v", runID, err)
	}

	//f.Sync()
	return nil

}

func mirrorTraceEvent(dataDir, runID string, payload []byte) error {
	traceDir := fsutil.GetLogDir(dataDir, runID)
	if err := os.MkdirAll(traceDir, 0755); err != nil {
		return fmt.Errorf("error creating trace directory %s: %w", traceDir, err)
	}
	tracePath := filepath.Join(traceDir, "events.jsonl")
	tf, err := os.OpenFile(tracePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error appending trace event for run %s: %w", runID, err)
	}
	defer tf.Close()

	if _, err := tf.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("error writing trace event for run %s: %w", runID, err)
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
				rows, err := db.Query(
					`SELECT seq, event_json FROM events WHERE run_id = ? AND seq > ? ORDER BY seq`,
					runID,
					currentOffset,
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
