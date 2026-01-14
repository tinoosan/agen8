package events

import (
	"context"
	"fmt"
	"strings"
)

// Event is the unified event payload emitted by the host.
//
// The workbench host emits events for observability and traceability:
// - Console: compact JSON lines in the terminal (human-readable "tail -f" experience)
// - Store:   append-only JSONL log on disk (run-scoped provenance record)
//
// Most events go to both sinks. Some events are console-only (e.g. token usage).
//
// Data compatibility:
//   - Data is the default map used by sinks.
//   - StoreData optionally overrides Data for the store sink only.
//     This preserves the current behavior where the console may include extra fields
//     (e.g. maxBytes) while the on-disk event log remains stable/minimal.
type Event struct {
	Type    string            // required (event type)
	Message string            // required (short description)
	Data    map[string]string // optional

	// StoreData is optional. If set, StoreSink uses this map instead of Data.
	StoreData map[string]string

	// Console controls whether the event is emitted to the console sink.
	// If nil, defaults to true.
	Console *bool

	// Store controls whether the event is emitted to the store sink.
	// If nil, defaults to true.
	Store *bool

	// History controls whether the event is emitted to the history sink.
	// If nil, defaults to true.
	History *bool

	// Origin is optional metadata for history.
	// Typical values: "user", "agent", "env".
	Origin string
}

func (e Event) validate() error {
	if e.Type == "" {
		return fmt.Errorf("event type is required")
	}
	if e.Message == "" {
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

// Sink is a destination for events.
//
// Sinks should be simple and synchronous: no buffering, retries, or background work.
type Sink interface {
	Emit(ctx context.Context, runID string, event Event) error
}

// MultiSink fans out events to multiple sinks in order.
//
// If a sink returns an error, fanout stops and that error is returned.
type MultiSink []Sink

func (m MultiSink) Emit(ctx context.Context, runID string, event Event) error {
	for _, s := range m {
		if s == nil {
			continue
		}
		if err := s.Emit(ctx, runID, event); err != nil {
			return err
		}
	}
	return nil
}

// Emitter binds a runID to a sink so call sites don't repeat it.
//
// Callers should emit exactly one event per "thing that happened" by calling Emit once.
type Emitter struct {
	RunID string
	Sink  Sink
}

// Emit emits a single event to the configured sink(s).
func (e *Emitter) Emit(ctx context.Context, event Event) error {
	if e == nil {
		return fmt.Errorf("events emitter is nil")
	}
	if strings.TrimSpace(e.RunID) == "" {
		return fmt.Errorf("events emitter runID is required")
	}
	if e.Sink == nil {
		return fmt.Errorf("events emitter sink is required")
	}
	if err := event.validate(); err != nil {
		return err
	}
	return e.Sink.Emit(ctx, e.RunID, event)
}
