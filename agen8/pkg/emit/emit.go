package emit

import (
	"context"
	"errors"
)

// ErrDropped indicates an emission was intentionally dropped (e.g. closed ordered emitter).
// Callers may treat this as non-fatal.
var ErrDropped = errors.New("emission dropped")

// Message envelopes a payload with its run identifier.
type Message[T any] struct {
	RunID   string
	Payload T
}

// Sink receives emitted messages.
type Sink[T any] interface {
	Emit(ctx context.Context, msg Message[T]) error
}

// SinkFunc adapts a function to a Sink.
type SinkFunc[T any] func(ctx context.Context, msg Message[T]) error

func (f SinkFunc[T]) Emit(ctx context.Context, msg Message[T]) error {
	return f(ctx, msg)
}

// MultiSink fans out emissions to multiple sinks in-order.
type MultiSink[T any] []Sink[T]

func (m MultiSink[T]) Emit(ctx context.Context, msg Message[T]) error {
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

// Emitter emits payloads for a single run.
type Emitter[T any] interface {
	Emit(ctx context.Context, payload T) error
}

// BoundEmitter binds a RunID to a Sink.
type BoundEmitter[T any] struct {
	RunID string
	Sink  Sink[T]
}

func (e *BoundEmitter[T]) Emit(ctx context.Context, payload T) error {
	if e == nil {
		return errors.New("emitter is nil")
	}
	if e.Sink == nil {
		return errors.New("emitter sink is nil")
	}
	return e.Sink.Emit(ctx, Message[T]{RunID: e.RunID, Payload: payload})
}
