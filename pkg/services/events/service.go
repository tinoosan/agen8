package events

import (
	"context"

	"github.com/tinoosan/agen8/pkg/types"
)

// Filter specifies filtering and pagination for event queries.
// Mirrors the store's EventFilter so callers do not depend on internal/store.
type Filter struct {
	RunID string // required: filter by run

	// Pagination
	Limit     int   // max results (default: 100, 0 = use default)
	Offset    int   // skip N events (for page-based pagination)
	AfterSeq  int64 // return events with seq > AfterSeq (for cursor-based pagination)
	BeforeSeq int64 // return events with seq < BeforeSeq (for reverse pagination)

	// Filtering by event type
	Types []string // filter to specific event types (empty = all types)

	// Sorting
	SortDesc bool // true = newest first (DESC), false = oldest first (ASC, default)
}

// TailedEvent is one event from a tail stream with the next offset for resuming.
type TailedEvent struct {
	Event      types.EventRecord
	NextOffset int64
}

// EventStore is the interface for appending and querying events.
// Implemented by *Service; useful for tests and future backends.
type EventStore interface {
	Append(ctx context.Context, event types.EventRecord) error
	ListPaginated(ctx context.Context, filter Filter) ([]types.EventRecord, int64, error)
	Count(ctx context.Context, filter Filter) (int, error)
	LatestSeq(ctx context.Context, runID string) (int64, error)
	Tail(ctx context.Context, runID string, fromOffset int64) (<-chan TailedEvent, <-chan error)
}

// Ensure *Service implements EventStore.
var _ EventStore = (*Service)(nil)
