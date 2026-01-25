package app

import (
	"context"
	"fmt"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/internal/tui"
)

// newTUIEmitter constructs an events emitter that writes:
//   - to the run event log
//   - to the session history
//   - to the Bubble Tea UI event stream
//
// If strict is true, emission errors panic (matching the current RunChatTUI behavior).
// If strict is false, errors are ignored (matching the current lazy-init behavior).
func newTUIEmitter(cfg config.Config, runID string, historySink *events.HistorySink, evCh chan events.Event, strict bool) (*events.Emitter, func(ctx context.Context, ev events.Event)) {
	emitter := &events.Emitter{
		RunID: runID,
		Sink: events.MultiSink{
			events.StoreSink{Store: storeEventAppender{cfg: cfg}},
			historySink,
			tui.EventSink{Ch: evCh},
		},
	}
	emit := func(ctx context.Context, ev events.Event) {
		if err := emitter.Emit(ctx, ev); err != nil && strict {
			// In the TUI we can't safely print. Fail fast; this indicates a host bug.
			panic(fmt.Errorf("emit event: %w", err))
		}
	}
	return emitter, emit
}

type storeEventAppender struct {
	cfg config.Config
}

func (s storeEventAppender) AppendEvent(ctx context.Context, runID, eventType, message string, data map[string]string) error {
	_ = ctx
	return store.AppendEvent(s.cfg, runID, eventType, message, data)
}
