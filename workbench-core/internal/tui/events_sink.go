package tui

import (
	"context"
	"errors"

	"github.com/tinoosan/workbench-core/pkg/events"
)

// EventSink delivers host events to the TUI.
//
// The app host already emits events through events.Emitter. The TUI registers this
// sink in a MultiSink so it can render the same events inline in the chat timeline.
//
// This sink does not perform formatting; it simply forwards the event payload.
type EventSink struct {
	Ch chan<- events.Event
}

func (s EventSink) Emit(_ context.Context, msg events.Message) (err error) {
	if s.Ch == nil {
		return nil
	}
	event := msg.Payload
	dropped := false
	// The host may emit a final "run completed" event after the UI has begun shutting
	// down. If the channel has already been closed, sending would panic. Treat that
	// as a no-op: events are still persisted by other sinks (store/history).
	defer func() {
		if recover() != nil {
			dropped = true
		}
		if dropped {
			err = errors.Join(err, events.ErrDropped)
		}
	}()
	select {
	case s.Ch <- event:
	default:
		// If the UI is slow, drop rather than block the host loop.
		dropped = true
		err = errors.Join(err, events.ErrDropped)
	}
	return err
}
