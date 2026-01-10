package store

import (
	"bufio"
	"context"
	"os"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/internal/fsutil"
)

func TestEventStore(t *testing.T) {
	tmpDir := t.TempDir()
	oldDataDir := DataDir
	DataDir = tmpDir
	defer func() { DataDir = oldDataDir }()

	run, err := CreateRun("Event Test Run", 1024)
	if err != nil {
		t.Fatalf("Failed to create run: %v", err)
	}

	t.Run("AppendEventWritesOneLine", func(t *testing.T) {
		err := AppendEvent(run.RunId, "test_event", "hello world", map[string]string{"foo": "bar"})
		if err != nil {
			t.Fatalf("AppendEvent failed: %v", err)
		}

		filePath := fsutil.GetEventFilePath(DataDir, run.RunId)
		f, err := os.Open(filePath)
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
			t.Errorf("Expected 1 line in event file, got %d", lineCount)
		}
	})

	t.Run("ListEventsReturnsInOrder", func(t *testing.T) {
		// Append a second event
		err := AppendEvent(run.RunId, "second_event", "second message", nil)
		if err != nil {
			t.Fatalf("Failed to append second event: %v", err)
		}

		events, offset, err := ListEvents(run.RunId)
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

		// Verify offset matches file size
		filePath := fsutil.GetEventFilePath(DataDir, run.RunId)
		info, err := os.Stat(filePath)
		if err != nil {
			t.Fatalf("Failed to stat event file: %v", err)
		}

		if offset != info.Size() {
			t.Errorf("Expected offset %d (file size), got %d", info.Size(), offset)
		}
	})

	t.Run("TailEventsReturnsFromOffset", func(t *testing.T) {
		// We already have 2 events from previous tests
		// Get offset before adding more
		filePath := fsutil.GetEventFilePath(DataDir, run.RunId)
		info, _ := os.Stat(filePath)
		offset := info.Size()

		// Add 2 more events
		AppendEvent(run.RunId, "third_event", "third message", nil)
		AppendEvent(run.RunId, "fourth_event", "fourth message", nil)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		eventCh, errCh := TailEvents(ctx, run.RunId, offset)

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
		filePath := fsutil.GetEventFilePath(DataDir, run.RunId)
		info, _ := os.Stat(filePath)
		offset := info.Size()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		eventCh, errCh := TailEvents(ctx, run.RunId, offset)

		// Append a new event after starting to tail
		// Give the tail library time to set up file watching
		go func() {
			time.Sleep(500 * time.Millisecond)
			AppendEvent(run.RunId, "new_event", "new message", nil)
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

	t.Run("TailEventsWaitsForCompleteLines", func(t *testing.T) {
		// Get current offset
		filePath := fsutil.GetEventFilePath(DataDir, run.RunId)
		info, _ := os.Stat(filePath)
		offset := info.Size()

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		eventCh, errCh := TailEvents(ctx, run.RunId, offset)

		// Write an incomplete line (no newline)
		go func() {
			time.Sleep(300 * time.Millisecond)
			f, _ := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
			f.WriteString(`{"eventId":"partial","runId":"` + run.RunId + `"`)
			f.Close()

			// Wait, then complete the line
			time.Sleep(500 * time.Millisecond)
			f, _ = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
			f.WriteString(`,"type":"complete_test","message":"test","timestamp":"2026-01-10T12:00:00Z"}` + "\n")
			f.Close()
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

		// Should receive exactly 1 complete event, not a partial
		if len(tailedEvents) != 1 {
			t.Fatalf("Expected 1 complete event, got %d", len(tailedEvents))
		}

		if tailedEvents[0].Event.Type != "complete_test" {
			t.Errorf("Expected complete_test, got %s", tailedEvents[0].Event.Type)
		}
	})
}
