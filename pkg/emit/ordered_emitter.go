package emit

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

type orderedItem[T any] struct {
	ctx     context.Context
	payload T
	done    chan error
}

// OrderedEmitter serializes calls to an underlying emitter.
//
// Emit blocks until the underlying emitter has processed the payload.
// Close prevents new emissions and drains any queued work.
type OrderedEmitter[T any] struct {
	Inner Emitter[T]

	once sync.Once
	mu   sync.Mutex
	ch   chan orderedItem[T]

	closed     atomic.Bool
	closeOnce  sync.Once
	workerDone chan struct{}
}

// NewOrderedEmitter creates an OrderedEmitter with a default queue size.
func NewOrderedEmitter[T any](inner Emitter[T]) *OrderedEmitter[T] {
	return &OrderedEmitter[T]{Inner: inner}
}

func (o *OrderedEmitter[T]) start() {
	o.once.Do(func() {
		o.ch = make(chan orderedItem[T], 128)
		o.workerDone = make(chan struct{})
		go func() {
			defer close(o.workerDone)
			for item := range o.ch {
				err := error(nil)
				if o.Inner == nil {
					err = errors.New("inner emitter is nil")
				} else {
					err = o.Inner.Emit(item.ctx, item.payload)
				}
				item.done <- err
				close(item.done)
			}
		}()
	})
}

func (o *OrderedEmitter[T]) Emit(ctx context.Context, payload T) error {
	if o == nil {
		return fmt.Errorf("ordered emitter is nil")
	}
	if o.closed.Load() {
		return ErrDropped
	}
	o.start()
	if o.Inner == nil {
		return fmt.Errorf("ordered emitter inner is nil")
	}

	done := make(chan error, 1)
	item := orderedItem[T]{ctx: ctx, payload: payload, done: done}

	o.mu.Lock()
	if o.closed.Load() {
		o.mu.Unlock()
		return ErrDropped
	}
	select {
	case <-ctx.Done():
		o.mu.Unlock()
		return ctx.Err()
	case o.ch <- item:
		o.mu.Unlock()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (o *OrderedEmitter[T]) Close() {
	if o == nil {
		return
	}
	// start is idempotent; it initializes ch/workerDone even if Close is called first.
	o.start()
	o.closeOnce.Do(func() {
		o.mu.Lock()
		o.closed.Store(true)
		close(o.ch)
		o.mu.Unlock()
	})
	<-o.workerDone
}
