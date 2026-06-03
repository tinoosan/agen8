package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/services/events/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

var ErrDropped = errors.New("emission dropped")

type Message struct {
	RunID   string
	Payload types.EventRecord
}

type Sink interface {
	Emit(ctx context.Context, msg Message) error
}

type SinkFunc func(ctx context.Context, msg Message) error

func (f SinkFunc) Emit(ctx context.Context, msg Message) error {
	return f(ctx, msg)
}

type MultiSink []Sink

func (m MultiSink) Emit(ctx context.Context, msg Message) error {
	var errs error
	for _, s := range m {
		if s == nil {
			continue
		}
		if err := s.Emit(ctx, msg); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// EmitFunc is a convenience type for components that need to emit events without
// depending on a full Sink/Emitter.
type EmitFunc func(ctx context.Context, ev types.EventRecord)

// Emitter validates and emits events through a Sink.
type Emitter struct {
	RunID string
	Sink  Sink
}

func (e *Emitter) Emit(ctx context.Context, event types.EventRecord) error {
	if e.Sink == nil {
		return fmt.Errorf("events emitter sink is required")
	}
	if err := domain.ValidateEvent(event); err != nil {
		return err
	}
	return e.Sink.Emit(ctx, Message{RunID: e.RunID, Payload: event})
}
