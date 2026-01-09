package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinoosan/workbench-core/internal/types"
)

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

func ListEvents(runId string) ([]types.Event, error) {
	targetPath := GetEventFilePath(runId)
	f, err := os.Open(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("error opening %s, does not exist: %w", targetPath, err)
		}
		return nil, fmt.Errorf("error opening %s: %w", targetPath, err)
	}

	defer f.Close()

	events := make([]types.Event, 0)

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		var event types.Event
		lineNum++
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("error reading event at line %d in %s: %w", lineNum, targetPath, err)
		}

		if event.RunId != runId {
			return nil, fmt.Errorf("error reading event at line %d in %s: runId mismatch", lineNum, targetPath)
		}

		if event.EventId == "" {
			return nil, fmt.Errorf("error reading event at line %d in %s: eventId cannot be blank", lineNum, targetPath)
		}

		if event.Timestamp.IsZero() {
			return nil, fmt.Errorf("error reading event at line %d in %s: timestamp cannot be zero", lineNum, targetPath)
		}

		if event.Type == "" {
			return nil, fmt.Errorf("error reading event at line %d in %s: type cannot be blank", lineNum, targetPath)
		}

		if event.Message == "" {
			return nil, fmt.Errorf("error reading event at line %d in %s: message cannot be blank", lineNum, targetPath)
		}
		events = append(events, event)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning %s at line %d: %w", targetPath, lineNum+1, err)
	}
	return events, nil
}
