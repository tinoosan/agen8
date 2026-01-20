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
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/validate"
)

// CreateRun initializes a new run with the given goal and context limit.
// It creates a unique run ID, a corresponding directory in data/runs,
// and persists the initial run state as run.json.
func CreateRun(cfg config.Config, goal string, maxBytesForContext int) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	// Runs always belong to a session. For now (until a CLI/session loader exists),
	// CreateRun creates a new session implicitly.
	sess, err := CreateSession(cfg, goal)
	if err != nil {
		return types.Run{}, err
	}
	return CreateRunInSession(cfg, sess.SessionID, "", goal, maxBytesForContext)
}

// CreateRunInSession creates a run within an existing session.
//
// parentRunID is optional; when set, the new run is considered a "sub-agent" run.
func CreateRunInSession(cfg config.Config, sessionID, parentRunID, goal string, maxBytesForContext int) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return types.Run{}, err
	}
	run := types.NewRun(goal, maxBytesForContext, sessionID, parentRunID)
	if err := SaveRun(cfg, run); err != nil {
		return types.Run{}, err
	}
	if _, err := AddRunToSession(cfg, sessionID, run.RunId); err != nil {
		return types.Run{}, err
	}
	return run, nil
}

// LoadRun reads a run's state from disk by its run ID.
// It returns an error if the file cannot be read, if the JSON is malformed,
// or if the loaded data is missing critical fields like runId.
func LoadRun(cfg config.Config, runId string) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	targetPath := fsutil.GetRunFilePath(cfg.DataDir, runId)
	b, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return types.Run{}, fmt.Errorf("run.json file %s does not exist: %w", targetPath, errors.Join(ErrNotFound, err))
		}
		return types.Run{}, fmt.Errorf("error reading run.json file %s: %w", targetPath, err)
	}

	var run types.Run

	if err := json.Unmarshal(b, &run); err != nil {
		return types.Run{}, fmt.Errorf("error unmarshalling json file %s: %w", targetPath, err)
	}

	if run.RunId == "" {
		return types.Run{}, fmt.Errorf("invalid run.json: missing runId: %w", ErrInvalid)
	}
	return run, nil
}

// SaveRun persists the current state of a run to disk as its run.json file.
// It ensures the necessary directory structure exists before writing.
func SaveRun(cfg config.Config, run types.Run) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	targetPath := fsutil.GetRunFilePath(cfg.DataDir, run.RunId)
	err := os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return err
	}

	b, err := types.MarshalPretty(run)
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
func StopRun(cfg config.Config, runId string, status types.RunStatus, errorMsg string) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}

	if status != types.StatusFailed && status != types.StatusDone && status != types.StatusCanceled {
		return types.Run{}, fmt.Errorf("error stopping run %s, invalid status '%s': %w", runId, status, ErrInvalid)
	}

	run, err := LoadRun(cfg, runId)
	if err != nil {
		return types.Run{}, fmt.Errorf("error stopping run: %w", err)
	}

	if run.Status == types.StatusDone || run.Status == types.StatusFailed || run.Status == types.StatusCanceled {
		return types.Run{}, fmt.Errorf("run %s cannot be stopped due to invalid state %s: %w", run.RunId, run.Status, ErrConflict)
	}

	now := time.Now()

	if status == types.StatusDone {
		run.Status = status
		run.FinishedAt = &now
		run.Error = nil
	}

	if status == types.StatusFailed {

		if errorMsg == "" {
			return types.Run{}, fmt.Errorf("error stopping run, error message is required for failed runs: %w", ErrInvalid)
		}
		run.Status = status
		run.FinishedAt = &now
		run.Error = &errorMsg
	}

	if status == types.StatusCanceled {
		if strings.TrimSpace(errorMsg) == "" {
			errorMsg = "canceled"
		}
		run.Status = status
		run.FinishedAt = &now
		run.Error = &errorMsg
	}

	return run, SaveRun(cfg, run)
}
