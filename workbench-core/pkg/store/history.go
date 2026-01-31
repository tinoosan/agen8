package store

import "context"

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
type HistoryCursor = OffsetCursor

// HistoryStore is the host-side storage interface backing the VFS mount "/history".
//
// SQLite-backed history is the canonical source of truth. Disk-backed history is
// provided for VFS compatibility and should be treated as a mirror.
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
	return HistoryCursor(OffsetCursorFromInt64(offset))
}

// HistoryCursorToInt64 decodes a DiskHistoryStore cursor into a byte offset.
//
// If the cursor is empty, it decodes to 0.
func HistoryCursorToInt64(c HistoryCursor) (int64, error) {
	return OffsetCursorToInt64(OffsetCursor(c))
}
