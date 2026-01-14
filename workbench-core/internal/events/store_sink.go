package events

import (
	"context"

	"github.com/tinoosan/workbench-core/internal/store"
)

// StoreSink appends events to the on-disk run event log via store.AppendEvent.
type StoreSink struct{}

func (StoreSink) Emit(_ context.Context, runID string, event Event) error {
	if !enabled(event.Store) {
		return nil
	}
	data := event.Data
	if event.StoreData != nil {
		data = event.StoreData
	}
	return store.AppendEvent(runID, event.Type, event.Message, data)
}
