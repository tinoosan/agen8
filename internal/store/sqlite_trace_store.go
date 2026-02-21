package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/pkg/config"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/validate"
)

// SQLiteTraceStore reads trace events directly from the canonical SQLite events table.
//
// Cursor semantics: the cursor is the SQLite event sequence id (events.seq).
type SQLiteTraceStore struct {
	Cfg   config.Config
	RunID string
}

func (s SQLiteTraceStore) EventsSince(ctx context.Context, cursor pkgstore.TraceCursor, opts pkgstore.TraceSinceOptions) (pkgstore.TraceBatch, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.Cfg.Validate(); err != nil {
		return pkgstore.TraceBatch{}, err
	}
	if err := validate.NonEmpty("RunID", s.RunID); err != nil {
		return pkgstore.TraceBatch{}, err
	}
	offset, err := pkgstore.TraceCursorToInt64(cursor)
	if err != nil {
		return pkgstore.TraceBatch{}, fmt.Errorf("sqlite trace store: invalid cursor: %w", ErrInvalid)
	}
	if offset < 0 {
		offset = 0
	}

	maxBytes, limit := normalizeTraceLimits(opts.MaxBytes, opts.Limit, defaultTraceSinceMaxBytes, defaultTraceSinceLimit)

	db, err := getSQLiteDB(s.Cfg)
	if err != nil {
		return pkgstore.TraceBatch{}, err
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT seq, event_json FROM events WHERE run_id = ? AND seq > ? ORDER BY seq LIMIT ?`,
		s.RunID,
		offset,
		limit*10,
	)
	if err != nil {
		return pkgstore.TraceBatch{}, err
	}
	defer rows.Close()

	var (
		events        []pkgstore.TraceEvent
		bytesConsumed int
		linesTotal    int
		parsed        int
		parseErrors   int
		lastSeq       = offset
	)

	for rows.Next() {
		var seq int64
		var raw string
		if err := rows.Scan(&seq, &raw); err != nil {
			return pkgstore.TraceBatch{}, err
		}
		lineBytes := len(raw) + 1
		if bytesConsumed+lineBytes > maxBytes {
			break
		}
		linesTotal++
		bytesConsumed += lineBytes
		lastSeq = seq

		event, ok := parseTraceEvent(raw)
		if !ok {
			parseErrors++
			continue
		}
		parsed++
		event.Type = strings.TrimSpace(event.Type)
		event.Message = strings.TrimSpace(event.Message)
		events = append(events, event)
		if len(events) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return pkgstore.TraceBatch{}, err
	}

	return pkgstore.TraceBatch{
		Events:         events,
		CursorAfter:    pkgstore.TraceCursorFromInt64(lastSeq),
		BytesRead:      bytesConsumed,
		LinesTotal:     linesTotal,
		Parsed:         parsed,
		ParseErrors:    parseErrors,
		Returned:       len(events),
		ReturnedCapped: len(events) >= limit,
		Truncated:      bytesConsumed >= maxBytes || len(events) >= limit,
	}, nil
}

func (s SQLiteTraceStore) EventsLatest(ctx context.Context, opts pkgstore.TraceLatestOptions) (pkgstore.TraceBatch, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.Cfg.Validate(); err != nil {
		return pkgstore.TraceBatch{}, err
	}
	if err := validate.NonEmpty("RunID", s.RunID); err != nil {
		return pkgstore.TraceBatch{}, err
	}

	maxBytes, limit := normalizeTraceLimits(opts.MaxBytes, opts.Limit, defaultTraceLatestMaxBytes, defaultTraceLatestLimit)

	db, err := getSQLiteDB(s.Cfg)
	if err != nil {
		return pkgstore.TraceBatch{}, err
	}

	queryLimit := limit * 10
	if queryLimit < 2000 {
		queryLimit = 2000
	}
	if queryLimit > 5000 {
		queryLimit = 5000
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT seq, event_json FROM events WHERE run_id = ? ORDER BY seq DESC LIMIT ?`,
		s.RunID,
		queryLimit,
	)
	if err != nil {
		return pkgstore.TraceBatch{}, err
	}
	defer rows.Close()

	var (
		eventsNewest  []pkgstore.TraceEvent
		bytesConsumed int
		linesTotal    int
		parsed        int
		parseErrors   int
		latestSeq     int64
	)

	for rows.Next() {
		var seq int64
		var raw string
		if err := rows.Scan(&seq, &raw); err != nil {
			return pkgstore.TraceBatch{}, err
		}
		if latestSeq == 0 || seq > latestSeq {
			latestSeq = seq
		}

		lineBytes := len(raw) + 1
		if len(eventsNewest) > 0 && bytesConsumed+lineBytes > maxBytes {
			break
		}
		if len(eventsNewest) >= limit {
			break
		}
		linesTotal++
		bytesConsumed += lineBytes

		event, ok := parseTraceEvent(raw)
		if !ok {
			parseErrors++
			continue
		}
		parsed++
		event.Type = strings.TrimSpace(event.Type)
		event.Message = strings.TrimSpace(event.Message)
		eventsNewest = append(eventsNewest, event)
	}
	if err := rows.Err(); err != nil {
		return pkgstore.TraceBatch{}, err
	}

	// Reverse to chronological order.
	for i, j := 0, len(eventsNewest)-1; i < j; i, j = i+1, j-1 {
		eventsNewest[i], eventsNewest[j] = eventsNewest[j], eventsNewest[i]
	}

	return pkgstore.TraceBatch{
		Events:         eventsNewest,
		CursorAfter:    pkgstore.TraceCursorFromInt64(latestSeq),
		BytesRead:      bytesConsumed,
		LinesTotal:     linesTotal,
		Parsed:         parsed,
		ParseErrors:    parseErrors,
		Returned:       len(eventsNewest),
		ReturnedCapped: len(eventsNewest) >= limit,
		Truncated:      bytesConsumed >= maxBytes || len(eventsNewest) >= limit,
	}, nil
}
