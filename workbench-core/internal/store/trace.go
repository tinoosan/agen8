package store

import (
	"context"
)

// TraceCursor is an opaque, stable position token used by trace stores.
//
// Cursor exists to prevent ambiguity and churn around incremental retrieval.
// At the module boundary, callers should treat TraceCursor as an uninterpreted
// string: do not assume it is a byte offset, a timestamp, or an event id.
//
// DiskTraceStore encodes TraceCursor as a base-10 int64 byte offset into its JSONL file.
type TraceCursor = OffsetCursor

// TraceStore is the pluggable storage interface for run trace/event retrieval.
//
// This is the "module-first" replacement for encoding queries into VFS paths like:
//
//	/trace/events.since/<offset>
//	/trace/events.latest/<n>
//
// Implementations can be backed by:
// - disk JSONL files (DiskTraceStore)
// - in-memory buffers (tests)
// - remote stores (future)
//
// Cursor semantics
// - cursor is an opaque token.
// - cursorAfter is the next cursor to use to fetch only new content.
// - cursorAfter MUST be deterministic for the same underlying data and options.
type TraceStore interface {
	EventsSince(ctx context.Context, cursor TraceCursor, opts TraceSinceOptions) (TraceBatch, error)
	EventsLatest(ctx context.Context, opts TraceLatestOptions) (TraceBatch, error)
}

type TraceSinceOptions struct {
	// MaxBytes caps how many bytes are read from cursor onward.
	// If <= 0, a default is used.
	MaxBytes int

	// Limit caps how many parsed events are returned.
	// If <= 0, a default is used.
	Limit int
}

type TraceLatestOptions struct {
	// MaxBytes caps how many bytes from the end of the file are considered.
	// If <= 0, a default is used.
	MaxBytes int

	// Limit caps how many parsed events are returned.
	// If <= 0, a default is used.
	Limit int
}

// TraceBatch is one bounded retrieval of trace events.
type TraceBatch struct {
	Events      []TraceEvent `json:"events"`
	CursorAfter TraceCursor  `json:"cursorAfter"`

	// Accounting for auditability/observability.
	BytesRead      int  `json:"bytesRead"`
	LinesTotal     int  `json:"linesTotal"`
	Parsed         int  `json:"parsed"`
	ParseErrors    int  `json:"parseErrors"`
	Returned       int  `json:"returned"`
	ReturnedCapped bool `json:"returnedCapped"`

	// Truncated is true when the store applied a cap that may have prevented
	// returning all available events for the requested range (maxBytes/limit).
	Truncated bool `json:"truncated"`
}

// TraceEvent is the minimal event shape returned by trace store reads.
//
// We keep this smaller than types.Event for token efficiency:
// - runId/eventId are already known from the run context
// - Timestamp/Type/Message/Data are the useful reasoning fields
type TraceEvent struct {
	Timestamp string            `json:"timestamp"`
	Type      string            `json:"type"`
	Message   string            `json:"message"`
	Data      map[string]string `json:"data,omitempty"`
}

// TraceCursorFromInt64 encodes a byte offset cursor as an opaque token.
//
// DiskTraceStore uses this encoding. Other stores are free to use different formats.
func TraceCursorFromInt64(offset int64) TraceCursor {
	return TraceCursor(OffsetCursorFromInt64(offset))
}

// TraceCursorToInt64 decodes a DiskTraceStore cursor into a byte offset.
//
// If the cursor is empty, it decodes to 0.
func TraceCursorToInt64(c TraceCursor) (int64, error) {
	return OffsetCursorToInt64(OffsetCursor(c))
}
