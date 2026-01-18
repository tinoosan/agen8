package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskTraceStore_EventsSince_CursorAdvancesDeterministically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	ev1 := map[string]any{
		"eventId":   "event-1",
		"runId":     "run-1",
		"timestamp": time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		"type":      "a",
		"message":   "m1",
		"data":      map[string]string{"k": "v"},
	}
	ev2 := map[string]any{
		"eventId":   "event-2",
		"runId":     "run-1",
		"timestamp": time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC),
		"type":      "b",
		"message":   "m2",
	}

	b1, _ := json.Marshal(ev1)
	b2, _ := json.Marshal(ev2)
	content := append(append(append(b1, '\n'), b2...), '\n')
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	s := DiskTraceStore{DiskStore: DiskStore{Dir: dir}}
	batch1, err := s.EventsSince(context.Background(), TraceCursorFromInt64(0), TraceSinceOptions{MaxBytes: 1024, Limit: 10})
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	after1, err := TraceCursorToInt64(batch1.CursorAfter)
	if err != nil {
		t.Fatalf("TraceCursorToInt64: %v", err)
	}
	if after1 != int64(len(content)) {
		t.Fatalf("cursorAfter=%d want %d", after1, len(content))
	}
	if len(batch1.Events) != 2 {
		t.Fatalf("events=%d", len(batch1.Events))
	}

	// Calling again from cursorAfter should return no new events and keep cursor stable.
	batch2, err := s.EventsSince(context.Background(), batch1.CursorAfter, TraceSinceOptions{MaxBytes: 1024, Limit: 10})
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	if batch2.CursorAfter != batch1.CursorAfter {
		t.Fatalf("cursorAfter changed: %q -> %q", batch1.CursorAfter, batch2.CursorAfter)
	}
	if len(batch2.Events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(batch2.Events))
	}
}

func TestTraceCursorToInt64_Invalid_IsErrInvalid(t *testing.T) {
	_, err := TraceCursorToInt64(TraceCursor("not-a-number"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected errors.Is(err, ErrInvalid) to be true, err=%v", err)
	}
}
