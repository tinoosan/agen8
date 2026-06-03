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

// Emitter emits payloads for a single run.
type Emitter[T any] interface {
	Emit(ctx context.Context, payload T) error
}
