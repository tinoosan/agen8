package store

import (
	"context"
	"fmt"

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

	var records []traceRecord
	var seqs []int64

	for rows.Next() {
		var seq int64
		var raw string
		if err := rows.Scan(&seq, &raw); err != nil {
			return pkgstore.TraceBatch{}, err
		}
		records = append(records, traceRecord{raw: raw, size: len(raw) + 1})
		seqs = append(seqs, seq)
	}
	if err := rows.Err(); err != nil {
		return pkgstore.TraceBatch{}, err
	}

	result := buildTraceSinceBatch(records, maxBytes, limit)
	lastSeq := offset
	if result.consumed > 0 {
		lastSeq = seqs[result.consumed-1]
	}
	result.batch.CursorAfter = pkgstore.TraceCursorFromInt64(lastSeq)
	return result.batch, nil
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

	var records []traceRecord
	var latestSeq int64

	for rows.Next() {
		var seq int64
		var raw string
		if err := rows.Scan(&seq, &raw); err != nil {
			return pkgstore.TraceBatch{}, err
		}
		if latestSeq == 0 || seq > latestSeq {
			latestSeq = seq
		}
		records = append(records, traceRecord{raw: raw, size: len(raw) + 1})
	}
	if err := rows.Err(); err != nil {
		return pkgstore.TraceBatch{}, err
	}

	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	batch := buildTraceLatestBatch(records, maxBytes, limit, false)
	batch.CursorAfter = pkgstore.TraceCursorFromInt64(latestSeq)
	return batch, nil
}
