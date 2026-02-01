package agentevents

import (
	"context"
	"fmt"
	"sync"

	"github.com/tinoosan/workbench-core/pkg/events"
)

type EmitFunc func(ctx context.Context, ev events.Event) error

type Writer struct {
	emit EmitFunc

	ch   chan workItem
	once sync.Once
}

type workItem struct {
	ctx  context.Context
	ev   events.Event
	done chan error
}

func NewWriter(emit EmitFunc) *Writer {
	return &Writer{
		emit: emit,
		ch:   make(chan workItem, 128),
	}
}

func (w *Writer) Start() {
	if w == nil {
		return
	}
	w.once.Do(func() {
		go func() {
			for item := range w.ch {
				err := error(nil)
				if w.emit == nil {
					err = fmt.Errorf("emit func is nil")
				} else {
					err = w.emit(item.ctx, item.ev)
				}
				item.done <- err
				close(item.done)
			}
		}()
	})
}

func (w *Writer) Close() {
	if w == nil {
		return
	}
	w.Start()
	close(w.ch)
}

// Emit writes an event and blocks until it is persisted/emitted by the underlying sink.
func (w *Writer) Emit(ctx context.Context, ev events.Event) error {
	if w == nil {
		return fmt.Errorf("writer is nil")
	}
	w.Start()
	done := make(chan error, 1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case w.ch <- workItem{ctx: ctx, ev: ev, done: done}:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

