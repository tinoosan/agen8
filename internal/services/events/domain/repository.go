package domain

import (
	"context"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// EventAppender persists events.
type EventAppender interface {
	Append(ctx context.Context, event types.EventRecord) error
}

// EventReader queries persisted events.
type EventReader interface {
	ListPaginated(ctx context.Context, filter EventFilter) ([]types.EventRecord, int64, error)
	Count(ctx context.Context, filter EventFilter) (int, error)
	LatestSeq(ctx context.Context, runID string) (int64, error)
}

// EventTailer streams events from a given offset.
type EventTailer interface {
	Tail(ctx context.Context, runID string, fromOffset int64) (<-chan TailedEvent, <-chan error)
}

// EventRepository combines all event storage operations.
type EventRepository interface {
	EventAppender
	EventReader
	EventTailer
}

// EventService is the subset used by RPC handlers and broadcasting.
// Replaces internal/app.EventsAppender.
type EventService interface {
	EventAppender
	EventReader
}
