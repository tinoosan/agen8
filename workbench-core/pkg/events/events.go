package events

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/types"
)

var (
	// ErrDropped indicates an event was intentionally dropped (e.g. non-blocking UI sinks)
	// and not delivered to that sink. Callers may treat this as non-fatal.
	ErrDropped = errors.New("event dropped")
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

type Sink interface {
	Emit(ctx context.Context, runID string, event types.EventRecord) error
}

type MultiSink []Sink

func (m MultiSink) Emit(ctx context.Context, runID string, event Event) error {
	var errs error
	for _, s := range m {
		if s == nil {
			continue
		}
		if err := s.Emit(ctx, runID, event); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

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
	return e.Sink.Emit(ctx, e.RunID, event)
}
