package events

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

func TestMultiSink_FanoutOrder(t *testing.T) {
	var calls []string
	s1 := SinkFunc(func(ctx context.Context, runID string, event Event) error {
		calls = append(calls, "s1:"+event.Type)
		return nil
	})
	s2 := SinkFunc(func(ctx context.Context, runID string, event Event) error {
		calls = append(calls, "s2:"+event.Type)
		return nil
	})

	m := MultiSink{s1, s2}
	if err := m.Emit(context.Background(), "run-1", Event{Type: "t", Message: "m"}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	if got := strings.Join(calls, ","); got != "s1:t,s2:t" {
		t.Fatalf("unexpected call order: %s", got)
	}
}

func TestConsoleSink_JSONShape(t *testing.T) {
	var buf bytes.Buffer
	oldOut := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
	})

	s := ConsoleSink{}
	if err := s.Emit(context.Background(), "run-1", Event{
		Type:    "x",
		Message: "hello",
		Data:    map[string]string{"k": "v"},
	}); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	got := strings.TrimSpace(buf.String())
	if got != `{"type":"x","message":"hello","data":{"k":"v"}}` {
		t.Fatalf("unexpected console json: %s", got)
	}
}

// SinkFunc adapts a function to the Sink interface.
type SinkFunc func(ctx context.Context, runID string, event Event) error

func (f SinkFunc) Emit(ctx context.Context, runID string, event Event) error {
	return f(ctx, runID, event)
}
