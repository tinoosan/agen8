package events

import (
	"context"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/types"
)

// StoreAppender is a minimal interface for persisting events.
// Context is reserved for future cancellation/timeout; current implementations may ignore it.
type StoreAppender interface {
	AppendEvent(ctx context.Context, event types.EventRecord) error
}

// StoreSink appends events to an event store.
type StoreSink struct {
	Store StoreAppender
}

func (s StoreSink) Emit(ctx context.Context, runID string, event Event) error {
	if !enabled(event.Store) {
		return nil
	}
	if s.Store == nil {
		return nil
	}
	ev := event
	if strings.TrimSpace(ev.RunID) == "" {
		ev.RunID = runID
	}
	if ev.StoreData != nil {
		ev.Data = ev.StoreData
	}
	return s.Store.AppendEvent(ctx, ev)
}
