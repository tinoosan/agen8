// Package store provides functionality for persisting and retrieving workbench data.
//
// Run storage layout (implementation detail)
//
// Each run is stored under:
//
//	data/runs/<runId>/
//
// With canonical files:
//   - run.json       (run metadata + state)
//   - events.jsonl   (append-only JSONL event log)
//
// And resource-backed directories:
//   - workspace/     (agent writable working directory)
//   - trace/         (mirrored event feed for agent polling)
//
// # Results note
//
// The agent sees a "/results" mount in the VFS, but the default implementation is now
// virtual (ResultsStore-backed) rather than an on-disk "results/" directory.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/jsonutil"
	"github.com/tinoosan/workbench-core/internal/types"
)

// CreateRun initializes a new run with the given goal and context limit.
// It creates a unique run ID, a corresponding directory in data/runs,
// and persists the initial run state as run.json.
func CreateRun(goal string, maxBytesForContext int) (types.Run, error) {
	run := types.NewRun(goal, maxBytesForContext)
	return run, SaveRun(run)
}

// LoadRun reads a run's state from disk by its run ID.
// It returns an error if the file cannot be read, if the JSON is malformed,
// or if the loaded data is missing critical fields like runId.
func LoadRun(runId string) (types.Run, error) {
	targetPath := fsutil.GetRunFilePath(config.DataDir, runId)
	b, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return types.Run{}, fmt.Errorf("run.json file %s does not exist: %w", targetPath, err)
		}
		return types.Run{}, fmt.Errorf("error reading run.json file %s: %w", targetPath, err)
	}

	var run types.Run

	if err := json.Unmarshal(b, &run); err != nil {
		return types.Run{}, fmt.Errorf("error unmarshalling json file %s: %w", targetPath, err)
	}

	if run.RunId == "" {
		return types.Run{}, fmt.Errorf("invalid run.json: missing runId")
	}
	return run, nil
}

// SaveRun persists the current state of a run to disk as its run.json file.
// It ensures the necessary directory structure exists before writing.
func SaveRun(run types.Run) error {
	targetPath := fsutil.GetRunFilePath(config.DataDir, run.RunId)
	err := os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return err
	}

	b, err := jsonutil.MarshalPretty(run)
	if err != nil {
		return fmt.Errorf("error marshalling run: %w", err)
	}
	if err := fsutil.WriteFileAtomic(targetPath, b, 0644); err != nil {
		return fmt.Errorf("error writing run.json file %s: %w", targetPath, err)
	}
	return nil
}

// StopRun transitions a run to a terminal state (Done or Failed).
// It updates the Status, sets FinishedAt to the current time,
// and records an error message if the status is Failed.
// The updated state is then persisted to disk.
func StopRun(runId string, status types.RunStatus, errorMsg string) (types.Run, error) {

	if status != types.StatusFailed && status != types.StatusDone {
		return types.Run{}, fmt.Errorf("error stopping run %s, invalid status '%s'", runId, status)
	}

	run, err := LoadRun(runId)
	if err != nil {
		return types.Run{}, fmt.Errorf("error stopping run: %w", err)
	}

	if run.Status == types.StatusDone || run.Status == types.StatusFailed {
		return types.Run{}, fmt.Errorf("run %s cannot be stopped due to invalid state %s", run.RunId, run.Status)
	}

	now := time.Now()

	if status == types.StatusDone {
		run.Status = status
		run.FinishedAt = &now
		run.Error = nil
	}

	if status == types.StatusFailed {

		if errorMsg == "" {
			return types.Run{}, fmt.Errorf("error stopping run, error message is required for failed runs")
		}
		run.Status = status
		run.FinishedAt = &now
		run.Error = &errorMsg
	}

	return run, SaveRun(run)
}
