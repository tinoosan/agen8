package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// StoreSink appends events to an event store.
type StoreSink struct {
	Store domain.EventAppender
}

func (s StoreSink) Emit(ctx context.Context, msg Message) error {
	runID := msg.RunID
	event := msg.Payload
	if !domain.Enabled(event.Store) {
		return nil
	}
	if s.Store == nil {
		return fmt.Errorf("store_sink: event appender not configured")
	}
	// Daemon-level events (e.g. daemon.start when no bootstrap run) have no runID; skip persisting.
	if strings.TrimSpace(runID) == "" {
		return nil
	}
	ev := event
	if strings.TrimSpace(string(ev.RunID)) == "" {
		ev.RunID = types.RunID(runID)
	}
	if ev.StoreData != nil {
		ev.Data = ev.StoreData
	}
	return s.Store.Append(ctx, ev)
}
