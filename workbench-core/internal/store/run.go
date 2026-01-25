// Package store provides functionality for persisting and retrieving workbench data.
//
// Run storage layout (implementation detail)
//
// Runs are stored in SQLite (data/workbench.db by default).
// Run-scoped directories for /scratch, /log, /memory, etc. remain on disk.
//
// # Results note
//
// The agent sees a "/results" mount in the VFS, but the default implementation is now
// virtual (ResultsStore-backed) rather than an on-disk "results/" directory.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

// CreateSubRun creates a run within an existing session.
//
// parentRunID is optional; when set, the new run is considered a "sub-agent" run.
func CreateSubRun(cfg config.Config, sessionID, parentRunID, goal string, maxBytesForContext int) (types.Run, error) {
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

// LoadRun reads a run's state from SQLite by its run ID.
// It returns an error if the row cannot be read, if the JSON is malformed,
// or if the loaded data is missing critical fields like runId.
func LoadRun(cfg config.Config, runId string) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return types.Run{}, err
	}
	var b []byte
	if err := db.QueryRow(`SELECT run_json FROM runs WHERE run_id = ?`, runId).Scan(&b); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Run{}, fmt.Errorf("run %s does not exist: %w", runId, errors.Join(ErrNotFound, os.ErrNotExist))
		}
		return types.Run{}, fmt.Errorf("error reading run %s: %w", runId, err)
	}

	var run types.Run

	if err := json.Unmarshal(b, &run); err != nil {
		return types.Run{}, fmt.Errorf("error unmarshalling run json: %w", err)
	}

	if run.RunId == "" {
		return types.Run{}, fmt.Errorf("invalid run.json: missing runId: %w", ErrInvalid)
	}
	return run, nil
}

// SaveRun persists the current state of a run to SQLite.
func SaveRun(cfg config.Config, run types.Run) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return err
	}
	b, err := types.MarshalPretty(run)
	if err != nil {
		return fmt.Errorf("error marshalling run: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	startedAt := timePtrToString(run.StartedAt)
	finishedAt := timePtrToString(run.FinishedAt)
	createdAt := startedAt
	if createdAt == "" {
		createdAt = now
	}
	_, err = db.Exec(
		`INSERT INTO runs (run_id, session_id, status, goal, run_json, started_at, finished_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET
		   session_id=excluded.session_id,
		   status=excluded.status,
		   goal=excluded.goal,
		   run_json=excluded.run_json,
		   started_at=excluded.started_at,
		   finished_at=excluded.finished_at,
		   created_at=COALESCE(runs.created_at, excluded.created_at),
		   updated_at=excluded.updated_at`,
		run.RunId,
		run.SessionID,
		string(run.Status),
		run.Goal,
		string(b),
		nullIfEmpty(startedAt),
		nullIfEmpty(finishedAt),
		createdAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("error saving run: %w", err)
	}
	return nil
}

// StopRun transitions a run to a terminal state (Done or Failed).
// It updates the Status, sets FinishedAt to the current time,
// and records an error message if the status is Failed.
// The updated state is then persisted to SQLite.
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
