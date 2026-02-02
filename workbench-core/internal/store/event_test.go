package store

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
)

func TestEventStore(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := CreateSession(cfg, "Event Test Run", 1024)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	t.Run("AppendEventWritesOneLine", func(t *testing.T) {
		err := AppendEvent(cfg, run.RunID, "test_event", "hello world", map[string]string{"foo": "bar"})
		if err != nil {
			t.Fatalf("AppendEvent failed: %v", err)
		}

		tracePath := filepath.Join(cfg.DataDir, "agents", run.RunID, "log", "events.jsonl")
		f, err := os.Open(tracePath)
		if err != nil {
			t.Fatalf("Failed to open event file: %v", err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		lineCount := 0
		for scanner.Scan() {
			lineCount++
		}

		if lineCount != 1 {
			t.Errorf("Expected 1 line in trace event file, got %d", lineCount)
		}
	})

	t.Run("AppendEvent_NonexistentRun_IsErrNotFound", func(t *testing.T) {
		err := AppendEvent(cfg, "run-does-not-exist", "test_event", "hello world", nil)
		if err == nil {
			t.Fatalf("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected errors.Is(err, ErrNotFound) to be true, err=%v", err)
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected errors.Is(err, os.ErrNotExist) to be true, err=%v", err)
		}
	})

	t.Run("ListEventsReturnsInOrder", func(t *testing.T) {
		// Append a second event
		err := AppendEvent(cfg, run.RunID, "second_event", "second message", nil)
		if err != nil {
			t.Fatalf("Failed to append second event: %v", err)
		}

		events, offset, err := ListEvents(cfg, run.RunID)
		if err != nil {
			t.Fatalf("ListEvents failed: %v", err)
		}

		if len(events) != 2 {
			t.Fatalf("Expected 2 events, got %d", len(events))
		}

		if events[0].Type != "test_event" || events[0].Message != "hello world" {
			t.Errorf("First event mismatch: %+v", events[0])
		}

		if events[1].Type != "second_event" || events[1].Message != "second message" {
			t.Errorf("Second event mismatch: %+v", events[1])
		}

		if offset < int64(len(events)) {
			t.Errorf("Expected offset >= %d, got %d", len(events), offset)
		}
	})

	t.Run("TailEventsReturnsFromOffset", func(t *testing.T) {
		// We already have 2 events from previous tests
		// Get offset before adding more
		_, offset, err := ListEvents(cfg, run.RunID)
		if err != nil {
			t.Fatalf("ListEvents failed: %v", err)
		}

		// Add 2 more events
		AppendEvent(cfg, run.RunID, "third_event", "third message", nil)
		AppendEvent(cfg, run.RunID, "fourth_event", "fourth message", nil)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		eventCh, errCh := TailEvents(cfg, ctx, run.RunID, offset)

		var tailedEvents []TailedEvent
		done := false
		for !done {
			select {
			case te, ok := <-eventCh:
				if !ok {
					done = true
					break
				}
				tailedEvents = append(tailedEvents, te)
				if len(tailedEvents) >= 2 {
					cancel()
				}
			case err := <-errCh:
				if err != nil {
					t.Fatalf("TailEvents failed: %v", err)
				}
			case <-ctx.Done():
				done = true
			}
		}

		if len(tailedEvents) != 2 {
			t.Fatalf("Expected 2 events, got %d", len(tailedEvents))
		}

		if tailedEvents[0].Event.Type != "third_event" || tailedEvents[1].Event.Type != "fourth_event" {
			t.Errorf("TailEvents returned wrong events: %+v", tailedEvents)
		}

		// Verify NextOffset is increasing
		if tailedEvents[1].NextOffset <= tailedEvents[0].NextOffset {
			t.Errorf("Expected NextOffset to increase, got %d then %d", tailedEvents[0].NextOffset, tailedEvents[1].NextOffset)
		}
	})

	t.Run("TailEventsReceivesNewEvents", func(t *testing.T) {
		// Get current offset
		_, offset, err := ListEvents(cfg, run.RunID)
		if err != nil {
			t.Fatalf("ListEvents failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		eventCh, errCh := TailEvents(cfg, ctx, run.RunID, offset)

		// Append a new event after starting to tail
		// Give the tail library time to set up file watching
		go func() {
			time.Sleep(500 * time.Millisecond)
			AppendEvent(cfg, run.RunID, "new_event", "new message", nil)
		}()

		var tailedEvents []TailedEvent
		done := false
		for !done {
			select {
			case te, ok := <-eventCh:
				if !ok {
					done = true
					break
				}
				tailedEvents = append(tailedEvents, te)
				cancel()
			case err := <-errCh:
				if err != nil {
					t.Fatalf("TailEvents failed: %v", err)
				}
			case <-ctx.Done():
				done = true
			}
		}

		if len(tailedEvents) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(tailedEvents))
		}

		if tailedEvents[0].Event.Type != "new_event" {
			t.Errorf("Expected new_event, got %s", tailedEvents[0].Event.Type)
		}
	})
}
