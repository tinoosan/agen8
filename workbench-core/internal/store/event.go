package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nxadm/tail"
	"github.com/tinoosan/workbench-core/internal/types"
)

type TailedEvent struct {
	Event      types.Event
	NextOffset int64
}

// AppendEvent records a new event for the specified run.
// It validates inputs, ensures the run exists, and appends the event to the run's event log.
func AppendEvent(runId, eventType, message string, data map[string]string) error {

	if runId == "" {
		return fmt.Errorf("error appending event, runId cannot be blank")
	}

	if eventType == "" {
		return fmt.Errorf("error appending event, eventType cannot be blank")
	}

	if message == "" {
		return fmt.Errorf("error appending event, message cannot be blank")
	}

	targetPath := GetEventFilePath(runId)
	runFilePath := GetRunFilePath(runId)

	// We check if a run exists before we attempt to create an event in reference to it
	_, err := os.Stat(runFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("cannot append event, %s does not exist: %w", runFilePath, err)
		}
		return fmt.Errorf("cannot append event, error reading run.json file %s: %w", runFilePath, err)
	}

	event := types.NewEvent(runId, eventType, message, data)

	err = os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return err
	}

	b, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error marshalling event: %w", err)
	}

	b = append(b, '\n')

	f, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("error appending event for run %s: %w", runId, err)
	}

	defer f.Close()

	_, err = f.Write(b)
	if err != nil {
		return fmt.Errorf("error writing event for run %s: %w", runId, err)
	}

	//f.Sync()
	return nil

}

// ListEvents retrieves all recorded events for a given run ID.
// It reads from the run's JSONL event log, validates each entry, and returns them in order.
func ListEvents(runId string) ([]types.Event, int64, error) {
	targetPath := GetEventFilePath(runId)
	events := make([]types.Event, 0)
	f, err := os.Open(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return events, 0, nil
		}
		return nil, 0, fmt.Errorf("error opening %s: %w", targetPath, err)
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		var event types.Event
		lineNum++
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, 0, fmt.Errorf("error reading event at line %d in %s: %w", lineNum, targetPath, err)
		}

		if event.RunId != runId {
			return nil, 0, fmt.Errorf("error reading event at line %d in %s: runId mismatch", lineNum, targetPath)
		}

		if event.EventId == "" {
			return nil, 0, fmt.Errorf("error reading event at line %d in %s: eventId cannot be blank", lineNum, targetPath)
		}

		if event.Timestamp.IsZero() {
			return nil, 0, fmt.Errorf("error reading event at line %d in %s: timestamp cannot be zero", lineNum, targetPath)
		}

		if event.Type == "" {
			return nil, 0, fmt.Errorf("error reading event at line %d in %s: type cannot be blank", lineNum, targetPath)
		}

		if event.Message == "" {
			return nil, 0, fmt.Errorf("error reading event at line %d in %s: message cannot be blank", lineNum, targetPath)
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("error scanning %s at line %d: %w", targetPath, lineNum+1, err)
	}

	info, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("error getting offset for %s: %w", targetPath, err)
	}

	nextOffset := info.Size()
	return events, nextOffset, nil
}

// TailEvents streams events for a given run from a specific byte offset.
// It follows the file and sends new events as they are appended.
// Each TailedEvent includes the NextOffset for resuming after refresh.
// The caller should cancel the context to stop tailing.
// Both returned channels are closed when the function exits.
func TailEvents(ctx context.Context, runId string, fromOffset int64) (<-chan TailedEvent, <-chan error) {
	eventCh := make(chan TailedEvent)
	errCh := make(chan error, 1)

	targetPath := GetEventFilePath(runId)
	currentOffset := fromOffset

	go func() {
		defer close(eventCh)
		defer close(errCh)

		// Validate offset is non-negative (always, regardless of file existence)
		if fromOffset < 0 {
			errCh <- fmt.Errorf("fromOffset cannot be negative")
			return
		}

		// Validate offset against file size if file exists
		info, err := os.Stat(targetPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				errCh <- fmt.Errorf("error getting file info for %s: %w", targetPath, err)
				return
			}
			// If file doesn't exist, tail will create a watcher and wait for it
		} else {
			if fromOffset > info.Size() {
				errCh <- fmt.Errorf("fromOffset %d exceeds file size %d", fromOffset, info.Size())
				return
			}
		}

		t, err := tail.TailFile(targetPath, tail.Config{
			Location:      &tail.SeekInfo{Offset: fromOffset, Whence: io.SeekStart},
			Follow:        true,
			ReOpen:        true,
			Poll:          true,
			Logger:        tail.DiscardingLogger,
			CompleteLines: true,
		})
		if err != nil {
			errCh <- fmt.Errorf("error tailing file %s: %w", targetPath, err)
			return
		}
		defer t.Cleanup()

		for {
			select {
			case <-ctx.Done():
				t.Stop()
				return
			case line, ok := <-t.Lines:
				if !ok {
					return
				}
				if line.Err != nil {
					errCh <- fmt.Errorf("error reading line: %w", line.Err)
					return
				}

				// Update offset: line length + 1 for newline
				currentOffset += int64(len(line.Text)) + 1

				// Skip empty lines
				text := strings.TrimSpace(line.Text)
				if text == "" {
					continue
				}

				var event types.Event
				if err := json.Unmarshal([]byte(text), &event); err != nil {
					errCh <- fmt.Errorf("error unmarshalling event: %w", err)
					return
				}

				// Validate runId matches
				if event.RunId != runId {
					errCh <- fmt.Errorf("runId mismatch: expected %s, got %s", runId, event.RunId)
					return
				}

				eventCh <- TailedEvent{
					Event:      event,
					NextOffset: currentOffset,
				}
			}
		}
	}()

	return eventCh, errCh
}
