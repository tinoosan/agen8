package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/pkg/bytesutil"
	"github.com/tinoosan/agen8/pkg/config"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/validate"
)

// SQLiteHistoryStore is a session-scoped HistoryStore backed by SQLite.
type SQLiteHistoryStore struct {
	SessionID string
	DB        *sql.DB
}

// NewSQLiteHistoryStore constructs a SQLiteHistoryStore for a sessionID under cfg.DataDir.
func NewSQLiteHistoryStore(cfg config.Config, sessionID string) (*SQLiteHistoryStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return nil, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	return &SQLiteHistoryStore{SessionID: sessionID, DB: db}, nil
}

func (s *SQLiteHistoryStore) ensure() error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("sqlite history store not configured")
	}
	if err := validate.NonEmpty("sessionId", s.SessionID); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteHistoryStore) ReadAll(_ context.Context) ([]byte, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	rows, err := s.DB.Query(`SELECT line_json FROM history WHERE session_id = ? ORDER BY seq`, s.SessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buf bytes.Buffer
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\n")
		if line == "" {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (s *SQLiteHistoryStore) AppendLine(_ context.Context, line []byte) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if line == nil {
		return fmt.Errorf("line is required")
	}
	trimmed := bytesutil.TrimRightNewlines(line)
	if len(trimmed) == 0 {
		return nil
	}

	var parsed struct {
		ID        string            `json:"id"`
		Timestamp string            `json:"ts"`
		RunID     string            `json:"runId"`
		Origin    string            `json:"origin"`
		Kind      string            `json:"kind"`
		Message   string            `json:"message"`
		Model     string            `json:"model,omitempty"`
		Data      map[string]string `json:"data,omitempty"`
	}
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return fmt.Errorf("sqlite history store: invalid json: %w", err)
	}

	dataJSON := ""
	if len(parsed.Data) > 0 {
		if b, err := json.Marshal(parsed.Data); err == nil {
			dataJSON = string(b)
		} else {
			return fmt.Errorf("sqlite history store: marshal data: %w", err)
		}
	}

	_, err := s.DB.Exec(
		`INSERT INTO history (id, session_id, run_id, ts, origin, kind, message, model, data_json, line_json)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		parsed.ID,
		s.SessionID,
		parsed.RunID,
		parsed.Timestamp,
		parsed.Origin,
		parsed.Kind,
		parsed.Message,
		nullIfEmpty(parsed.Model),
		nullIfEmpty(dataJSON),
		string(trimmed),
	)
	if err != nil {
		return fmt.Errorf("sqlite history store: insert: %w", err)
	}
	return nil
}

func (s *SQLiteHistoryStore) LinesSince(_ context.Context, cursor pkgstore.HistoryCursor, opts pkgstore.HistorySinceOptions) (pkgstore.HistoryBatch, error) {
	if err := s.ensure(); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: cursor}, err
	}

	maxBytes, limit := normalizeHistoryLimits(opts.MaxBytes, opts.Limit)

	offset, err := pkgstore.HistoryCursorToInt64(cursor)
	if err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, fmt.Errorf("invalid cursor: %w", ErrInvalid)
	}

	rows, err := s.DB.Query(
		`SELECT seq, line_json FROM history WHERE session_id = ? AND seq > ? ORDER BY seq`,
		s.SessionID,
		offset,
	)
	if err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(offset)}, err
	}
	defer rows.Close()

	var records []historyRecord
	var seqs []int64

	for rows.Next() {
		var seq int64
		var line string
		if err := rows.Scan(&seq, &line); err != nil {
			return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(offset)}, err
		}
		line = strings.TrimRight(line, "\n")
		records = append(records, historyRecord{line: []byte(line), size: len(line) + 1})
		seqs = append(seqs, seq)
	}
	if err := rows.Err(); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(offset)}, err
	}

	result := buildHistorySinceBatch(records, maxBytes, limit)
	lastSeq := offset
	if result.consumed > 0 {
		lastSeq = seqs[result.consumed-1]
	}
	result.batch.CursorAfter = pkgstore.HistoryCursorFromInt64(lastSeq)
	return result.batch, nil
}

func (s *SQLiteHistoryStore) LinesLatest(_ context.Context, opts pkgstore.HistoryLatestOptions) (pkgstore.HistoryBatch, error) {
	if err := s.ensure(); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, err
	}

	maxBytes, limit := normalizeHistoryLimits(opts.MaxBytes, opts.Limit)

	rows, err := s.DB.Query(
		`SELECT seq, line_json FROM history WHERE session_id = ? ORDER BY seq DESC LIMIT ?`,
		s.SessionID,
		limit,
	)
	if err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, err
	}
	defer rows.Close()

	var records []historyRecord
	var lastSeq int64

	for rows.Next() {
		var seq int64
		var line string
		if err := rows.Scan(&seq, &line); err != nil {
			return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(lastSeq)}, err
		}
		if seq > lastSeq {
			lastSeq = seq
		}
		line = strings.TrimRight(line, "\n")
		records = append(records, historyRecord{line: []byte(line), size: len(line) + 1})
	}
	if err := rows.Err(); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(lastSeq)}, err
	}

	// Convert newest-first query results into chronological order for the shared batch builder.
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}
	batch := buildHistoryLatestBatch(records, maxBytes, limit, false)
	batch.CursorAfter = pkgstore.HistoryCursorFromInt64(lastSeq)
	return batch, nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
