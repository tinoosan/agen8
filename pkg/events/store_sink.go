package events

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8/pkg/types"
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

func (s StoreSink) Emit(ctx context.Context, msg Message) error {
	runID := msg.RunID
	event := msg.Payload
	if !enabled(event.Store) {
		return nil
	}
	if s.Store == nil {
		return nil
	}
	// Daemon-level events (e.g. daemon.start when no bootstrap run) have no runID; skip persisting.
	if strings.TrimSpace(runID) == "" {
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
