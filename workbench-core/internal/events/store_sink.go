package events

import (
	"context"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/store"
)

// StoreSink appends events to the on-disk run event log via store.AppendEvent.
type StoreSink struct {
	Cfg config.Config
}

func (s StoreSink) Emit(_ context.Context, runID string, event Event) error {
	if !enabled(event.Store) {
		return nil
	}
	data := event.Data
	if event.StoreData != nil {
		data = event.StoreData
	}
	// Note: Cfg is validated inside store.AppendEvent.
	return store.AppendEvent(s.Cfg, runID, event.Type, event.Message, data)
}
