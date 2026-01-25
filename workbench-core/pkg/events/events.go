package events

import (
	"context"
	"fmt"
	"strings"
)

// Event is the unified event payload emitted by the host.
type Event struct {
	Type      string
	Message   string
	Data      map[string]string
	StoreData map[string]string
	Console   *bool
	Store     *bool
	History   *bool
	Origin    string
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

type Sink interface {
	Emit(ctx context.Context, runID string, event Event) error
}

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

type Emitter struct {
	RunID string
	Sink  Sink
}

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
