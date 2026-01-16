package tui

import (
	"context"

	"github.com/tinoosan/workbench-core/internal/events"
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

func (s EventSink) Emit(_ context.Context, _ string, event events.Event) error {
	if s.Ch == nil {
		return nil
	}
	select {
	case s.Ch <- event:
	default:
		// If the UI is slow, drop rather than block the host loop.
	}
	return nil
}
