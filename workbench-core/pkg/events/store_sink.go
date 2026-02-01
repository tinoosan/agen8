package events

import (
	"context"
	"strings"
)

// StoreAppender is a minimal interface for persisting events.
type StoreAppender interface {
	AppendEvent(ctx context.Context, runID, eventType, message string, data map[string]string) error
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
	data := event.Data
	if event.StoreData != nil {
		data = event.StoreData
	}
	origin := strings.TrimSpace(event.Origin)
	if origin != "" {
		// Never mutate the input map.
		out := make(map[string]string, len(data)+1)
		for k, v := range data {
			out[k] = v
		}
		if _, ok := out["origin"]; !ok {
			out["origin"] = origin
		}
		data = out
	}
	return s.Store.AppendEvent(ctx, runID, event.Type, event.Message, data)
}
