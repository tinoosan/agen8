package store

import (
	"bufio"
	"os"
	"testing"
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

		filePath := GetEventFilePath(run.RunId)
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

		events, err := ListEvents(run.RunId)
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
	})
}
