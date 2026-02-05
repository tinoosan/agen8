package emit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingEmitter struct {
	mu  sync.Mutex
	got []int
}

func (r *recordingEmitter) Emit(_ context.Context, payload int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, payload)
	return nil
}

func TestOrderedEmitter_Serializes(t *testing.T) {
	inner := &recordingEmitter{}
	o := NewOrderedEmitter[int](inner)
	t.Cleanup(func() { o.Close() })

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = o.Emit(ctx, i)
		}()
	}
	wg.Wait()

	inner.mu.Lock()
	defer inner.mu.Unlock()
	if len(inner.got) != 50 {
		t.Fatalf("len(got)=%d want 50", len(inner.got))
	}
}

func TestOrderedEmitter_CloseDrops(t *testing.T) {
	inner := &recordingEmitter{}
	o := NewOrderedEmitter[int](inner)
	o.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := o.Emit(ctx, 1); !errors.Is(err, ErrDropped) {
		t.Fatalf("err=%v want ErrDropped", err)
	}
}
