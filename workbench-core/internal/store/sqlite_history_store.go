package store

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/bytesutil"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/pkg/validate"
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

func (s *SQLiteHistoryStore) LinesSince(_ context.Context, cursor HistoryCursor, opts HistorySinceOptions) (HistoryBatch, error) {
	if err := s.ensure(); err != nil {
		return HistoryBatch{CursorAfter: cursor}, err
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 16 * 1024
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}

	offset, err := HistoryCursorToInt64(cursor)
	if err != nil {
		return HistoryBatch{CursorAfter: HistoryCursorFromInt64(0)}, fmt.Errorf("invalid cursor: %w", ErrInvalid)
	}

	rows, err := s.DB.Query(
		`SELECT seq, line_json FROM history WHERE session_id = ? AND seq > ? ORDER BY seq`,
		s.SessionID,
		offset,
	)
	if err != nil {
		return HistoryBatch{CursorAfter: HistoryCursorFromInt64(offset)}, err
	}
	defer rows.Close()

	var (
		out        [][]byte
		bytesRead  int
		linesTotal int
		truncated  bool
		lastSeq    = offset
	)

	for rows.Next() {
		if bytesRead >= maxBytes {
			truncated = true
			break
		}
		var seq int64
		var line string
		if err := rows.Scan(&seq, &line); err != nil {
			return HistoryBatch{CursorAfter: HistoryCursorFromInt64(lastSeq)}, err
		}
		linesTotal++
		lastSeq = seq
		line = strings.TrimRight(line, "\n")
		lineBytes := []byte(line)
		bytesRead += len(lineBytes) + 1
		trim := bytesutil.TrimRightNewlines(lineBytes)
		if len(trim) > 0 {
			out = append(out, append([]byte(nil), trim...))
		}
		if len(out) >= limit {
			truncated = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		return HistoryBatch{CursorAfter: HistoryCursorFromInt64(lastSeq)}, err
	}

	return HistoryBatch{
		Lines:          out,
		CursorAfter:    HistoryCursorFromInt64(lastSeq),
		BytesRead:      bytesRead,
		LinesTotal:     linesTotal,
		Returned:       len(out),
		ReturnedCapped: len(out) >= limit,
		Truncated:      truncated,
	}, nil
}

func (s *SQLiteHistoryStore) LinesLatest(_ context.Context, opts HistoryLatestOptions) (HistoryBatch, error) {
	if err := s.ensure(); err != nil {
		return HistoryBatch{CursorAfter: HistoryCursorFromInt64(0)}, err
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 16 * 1024
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 200
	}

	rows, err := s.DB.Query(
		`SELECT seq, line_json FROM history WHERE session_id = ? ORDER BY seq DESC LIMIT ?`,
		s.SessionID,
		limit,
	)
	if err != nil {
		return HistoryBatch{CursorAfter: HistoryCursorFromInt64(0)}, err
	}
	defer rows.Close()

	var (
		lines      [][]byte
		bytesRead  int
		linesTotal int
		lastSeq    int64
		truncated  bool
	)

	for rows.Next() {
		var seq int64
		var line string
		if err := rows.Scan(&seq, &line); err != nil {
			return HistoryBatch{CursorAfter: HistoryCursorFromInt64(lastSeq)}, err
		}
		linesTotal++
		if seq > lastSeq {
			lastSeq = seq
		}
		line = strings.TrimRight(line, "\n")
		lineBytes := []byte(line)
		if bytesRead+len(lineBytes)+1 > maxBytes {
			truncated = true
			continue
		}
		bytesRead += len(lineBytes) + 1
		trim := bytesutil.TrimRightNewlines(lineBytes)
		if len(trim) > 0 {
			lines = append(lines, append([]byte(nil), trim...))
		}
	}
	if err := rows.Err(); err != nil {
		return HistoryBatch{CursorAfter: HistoryCursorFromInt64(lastSeq)}, err
	}

	// Reverse to chronological order.
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}

	return HistoryBatch{
		Lines:          lines,
		CursorAfter:    HistoryCursorFromInt64(lastSeq),
		BytesRead:      bytesRead,
		LinesTotal:     linesTotal,
		Returned:       len(lines),
		ReturnedCapped: truncated || len(lines) >= limit,
		Truncated:      truncated,
	}, nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
