package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/emit"
	"github.com/tinoosan/workbench-core/pkg/types"
)

var (
	// ErrDropped indicates an event was intentionally dropped (e.g. non-blocking UI sinks)
	// and not delivered to that sink. Callers may treat this as non-fatal.
	ErrDropped = emit.ErrDropped
)

// Event is the unified event payload used both for emission and storage.
type Event = types.EventRecord

// EmitFunc is a convenience type for components that need to emit events without
// depending on a full Sink/Emitter.
type EmitFunc func(ctx context.Context, ev types.EventRecord)

func validateEvent(e types.EventRecord) error {
	if strings.TrimSpace(e.Type) == "" {
		return fmt.Errorf("event type is required")
	}
	if strings.TrimSpace(e.Message) == "" {
		return fmt.Errorf("event message is required")
	}
	return nil
}

func enabled(ptr *bool) bool {
	if ptr == nil {
		return true
	}
	return *ptr
}

type Message = emit.Message[Event]
type Sink = emit.Sink[Event]
type SinkFunc = emit.SinkFunc[Event]
type MultiSink = emit.MultiSink[Event]

type Emitter struct {
	RunID string
	Sink  Sink
}

func (e *Emitter) Emit(ctx context.Context, event types.EventRecord) error {
	if e == nil {
		return fmt.Errorf("events emitter is nil")
	}
	if strings.TrimSpace(e.RunID) == "" {
		return fmt.Errorf("events emitter runID is required")
	}
	if e.Sink == nil {
		return fmt.Errorf("events emitter sink is required")
	}
	if err := validateEvent(event); err != nil {
		return err
	}
	return e.Sink.Emit(ctx, Message{RunID: e.RunID, Payload: event})
}
