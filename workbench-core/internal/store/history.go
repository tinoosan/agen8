package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// HistoryAppender is used by sinks to record history.
type HistoryAppender interface {
	AppendLine(ctx context.Context, line []byte) error
}

// HistoryReader is used by VFS/context to read history.
type HistoryReader interface {
	ReadAll(ctx context.Context) ([]byte, error)
	LinesSince(ctx context.Context, cursor HistoryCursor, opts HistorySinceOptions) (HistoryBatch, error)
	LinesLatest(ctx context.Context, opts HistoryLatestOptions) (HistoryBatch, error)
}

// HistoryCursor is an opaque position token used for incremental history retrieval.
//
// Cursor exists so higher-level code (agent loop, context updater, UI) can fetch only new
// history entries deterministically without encoding queries into VFS paths.
//
// DiskHistoryStore encodes HistoryCursor as a base-10 int64 byte offset into its JSONL file.
type HistoryCursor string

// HistoryStore is the host-side storage interface backing the VFS mount "/history".
//
// History is a verifiable, append-only source of truth for raw interactions between:
//   - users
//   - agents
//   - the environment/host
//
// Unlike /trace (curated agent-facing events) and /memory (curated host-governed notes),
// history is immutable and append-only.
//
// The agent can read history via VFS, but cannot write to it.
//
// Cursor semantics
// - cursor is an opaque token.
// - cursorAfter is the next cursor to use to fetch only new content.
// - cursorAfter MUST be deterministic for the same underlying data and options.
type HistoryStore interface {
	HistoryAppender
	HistoryReader
}

type HistorySinceOptions struct {
	// MaxBytes caps how many bytes are read from cursor onward.
	// If <= 0, a default is used.
	MaxBytes int

	// Limit caps how many parsed lines are returned.
	// If <= 0, a default is used.
	Limit int
}

type HistoryLatestOptions struct {
	// MaxBytes caps how many bytes from the end of the file are considered.
	// If <= 0, a default is used.
	MaxBytes int

	// Limit caps how many parsed lines are returned.
	// If <= 0, a default is used.
	Limit int
}

// HistoryBatch is one bounded retrieval of history entries.
//
// Events are returned as raw JSON bytes (one object per entry) so callers can:
// - filter without committing to a single schema
// - unmarshal only the fields they need
type HistoryBatch struct {
	Lines       [][]byte      `json:"lines"`
	CursorAfter HistoryCursor `json:"cursorAfter"`

	BytesRead      int  `json:"bytesRead"`
	LinesTotal     int  `json:"linesTotal"`
	Returned       int  `json:"returned"`
	ReturnedCapped bool `json:"returnedCapped"`
	Truncated      bool `json:"truncated"`
}

// HistoryCursorFromInt64 encodes a byte offset cursor as an opaque token.
func HistoryCursorFromInt64(offset int64) HistoryCursor {
	if offset < 0 {
		offset = 0
	}
	return HistoryCursor(strconv.FormatInt(offset, 10))
}

// HistoryCursorToInt64 decodes a DiskHistoryStore cursor into a byte offset.
//
// If the cursor is empty, it decodes to 0.
func HistoryCursorToInt64(c HistoryCursor) (int64, error) {
	s := strings.TrimSpace(string(c))
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid cursor: %w", ErrInvalid)
	}
	return n, nil
}

