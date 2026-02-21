package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestEventStore(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := CreateSession(cfg, "Event Test Run", 1024)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	t.Run("AppendEvent_DoesNotWriteEventsJSONL", func(t *testing.T) {
		err := AppendEvent(context.Background(), cfg, types.EventRecord{
			RunID:     run.RunID,
			Type:      "test_event",
			Message:   "hello world",
			Data:      map[string]string{"foo": "bar"},
			Origin:    "",
			Store:     nil,
			Console:   nil,
			History:   nil,
			StoreData: nil,
		})
		if err != nil {
			t.Fatalf("AppendEvent failed: %v", err)
		}

		tracePath := filepath.Join(cfg.DataDir, "agents", run.RunID, "log", "events.jsonl")
		if _, err := os.Stat(tracePath); err == nil {
			t.Fatalf("expected %s to not be created", tracePath)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected os.ErrNotExist, got %v", err)
		}
	})

	t.Run("AppendEvent_NonexistentRun_IsErrNotFound", func(t *testing.T) {
		err := AppendEvent(context.Background(), cfg, types.EventRecord{
			RunID:   "run-does-not-exist",
			Type:    "test_event",
			Message: "hello world",
		})
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
		err := AppendEvent(context.Background(), cfg, types.EventRecord{
			RunID:   run.RunID,
			Type:    "second_event",
			Message: "second message",
		})
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
		_ = AppendEvent(context.Background(), cfg, types.EventRecord{RunID: run.RunID, Type: "third_event", Message: "third message"})
		_ = AppendEvent(context.Background(), cfg, types.EventRecord{RunID: run.RunID, Type: "fourth_event", Message: "fourth message"})

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
			_ = AppendEvent(context.Background(), cfg, types.EventRecord{RunID: run.RunID, Type: "new_event", Message: "new message"})
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

	t.Run("ListEventsPaginated_CursorPagination", func(t *testing.T) {
		_, runPaged, err := CreateSession(cfg, "Event Paged Run", 1024)
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		// Append 10 events.
		for i := 0; i < 10; i++ {
			if err := AppendEvent(context.Background(), cfg, types.EventRecord{
				RunID:   runPaged.RunID,
				Type:    "paged",
				Message: fmt.Sprintf("event %d", i),
			}); err != nil {
				t.Fatalf("AppendEvent %d: %v", i, err)
			}
		}

		filter := EventFilter{RunID: runPaged.RunID, Types: []string{"paged"}, Limit: 4}
		b1, cursor, err := ListEventsPaginated(cfg, filter)
		if err != nil {
			t.Fatalf("ListEventsPaginated: %v", err)
		}
		if len(b1) != 4 {
			t.Fatalf("expected 4 events in batch 1, got %d", len(b1))
		}
		if cursor == 0 {
			t.Fatalf("expected non-zero cursor")
		}

		filter.AfterSeq = cursor
		b2, cursor, err := ListEventsPaginated(cfg, filter)
		if err != nil {
			t.Fatalf("ListEventsPaginated batch 2: %v", err)
		}
		if len(b2) != 4 {
			t.Fatalf("expected 4 events in batch 2, got %d", len(b2))
		}

		filter.AfterSeq = cursor
		b3, _, err := ListEventsPaginated(cfg, filter)
		if err != nil {
			t.Fatalf("ListEventsPaginated batch 3: %v", err)
		}
		if len(b3) != 2 {
			t.Fatalf("expected 2 events in batch 3, got %d", len(b3))
		}

		// Verify total retrieved (only those with type "paged"; other test events exist too).
		total := len(b1) + len(b2) + len(b3)
		if total != 10 {
			t.Fatalf("expected 10 total events, got %d", total)
		}
	})

	t.Run("ListEventsPaginated_TypeFilterAndCount", func(t *testing.T) {
		_, run2, err := CreateSession(cfg, "Event Filter Run", 1024)
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		AppendEvent(context.Background(), cfg, types.EventRecord{RunID: run2.RunID, Type: "info", Message: "info 1"})
		AppendEvent(context.Background(), cfg, types.EventRecord{RunID: run2.RunID, Type: "error", Message: "error 1"})
		AppendEvent(context.Background(), cfg, types.EventRecord{RunID: run2.RunID, Type: "info", Message: "info 2"})
		AppendEvent(context.Background(), cfg, types.EventRecord{RunID: run2.RunID, Type: "warning", Message: "warning 1"})

		filter := EventFilter{RunID: run2.RunID, Types: []string{"info"}, Limit: 100}
		evs, _, err := ListEventsPaginated(cfg, filter)
		if err != nil {
			t.Fatalf("ListEventsPaginated: %v", err)
		}
		if len(evs) != 2 {
			t.Fatalf("expected 2 info events, got %d", len(evs))
		}
		for _, e := range evs {
			if e.Type != "info" {
				t.Fatalf("expected type 'info', got %q", e.Type)
			}
		}

		count, err := CountEvents(cfg, EventFilter{RunID: run2.RunID})
		if err != nil {
			t.Fatalf("CountEvents: %v", err)
		}
		if count != 4 {
			t.Fatalf("expected 4 events, got %d", count)
		}
	})

	t.Run("GetLatestEventSeq", func(t *testing.T) {
		_, run3, err := CreateSession(cfg, "Event Seq Run", 1024)
		if err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}

		seq, err := GetLatestEventSeq(cfg, run3.RunID)
		if err != nil {
			t.Fatalf("GetLatestEventSeq: %v", err)
		}
		if seq != 0 {
			t.Fatalf("expected seq 0 for no events, got %d", seq)
		}

		for i := 0; i < 3; i++ {
			_ = AppendEvent(context.Background(), cfg, types.EventRecord{
				RunID:   run3.RunID,
				Type:    "test",
				Message: fmt.Sprintf("event %d", i),
			})
		}

		seq2, err := GetLatestEventSeq(cfg, run3.RunID)
		if err != nil {
			t.Fatalf("GetLatestEventSeq: %v", err)
		}
		if seq2 <= 0 {
			t.Fatalf("expected non-zero seq after adding events")
		}
	})

	t.Run("ListEventsPaginated_EmptyRunID", func(t *testing.T) {
		_, _, err := ListEventsPaginated(cfg, EventFilter{})
		if err == nil {
			t.Fatalf("expected error for empty runID")
		}
	})
}
