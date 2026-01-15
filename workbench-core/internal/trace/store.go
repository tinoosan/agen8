package trace

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Cursor is an opaque, stable position token used by trace stores.
//
// Cursor exists to prevent ambiguity and churn around incremental retrieval.
// At the tool/module boundary, callers should treat Cursor as an uninterpreted
// string: do not assume it is a byte offset, a timestamp, or an event id.
//
// DiskTraceStore encodes Cursor as a base-10 int64 byte offset into its JSONL file.
type Cursor string

// Store is the pluggable storage interface for trace/event retrieval.
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
type Store interface {
	EventsSince(ctx context.Context, cursor Cursor, opts SinceOptions) (Batch, error)
	EventsLatest(ctx context.Context, opts LatestOptions) (Batch, error)
}

type SinceOptions struct {
	// MaxBytes caps how many bytes are read from cursor onward.
	// If <= 0, a default is used.
	MaxBytes int

	// Limit caps how many parsed events are returned.
	// If <= 0, a default is used.
	Limit int
}

type LatestOptions struct {
	// MaxBytes caps how many bytes from the end of the file are considered.
	// If <= 0, a default is used.
	MaxBytes int

	// Limit caps how many parsed events are returned.
	// If <= 0, a default is used.
	Limit int
}

// Batch is one bounded retrieval of trace events.
type Batch struct {
	Events      []Event `json:"events"`
	CursorAfter Cursor  `json:"cursorAfter"`

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

// Event is the minimal event shape returned by module actions.
//
// We keep this smaller than types.Event for token efficiency:
// - runId/eventId are already known from the run context
// - Timestamp/Type/Message/Data are the useful reasoning fields
type Event struct {
	Timestamp string            `json:"timestamp"`
	Type      string            `json:"type"`
	Message   string            `json:"message"`
	Data      map[string]string `json:"data,omitempty"`
}

// CursorFromInt64 encodes a byte offset cursor as an opaque token.
//
// DiskTraceStore uses this encoding. Other stores are free to use different formats.
func CursorFromInt64(offset int64) Cursor {
	if offset < 0 {
		offset = 0
	}
	return Cursor(strconv.FormatInt(offset, 10))
}

// CursorToInt64 decodes a DiskTraceStore cursor into a byte offset.
//
// If the cursor is empty, it decodes to 0.
func CursorToInt64(c Cursor) (int64, error) {
	s := strings.TrimSpace(string(c))
	if s == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid cursor")
	}
	return n, nil
}
