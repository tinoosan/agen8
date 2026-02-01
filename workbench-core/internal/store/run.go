// Package store provides functionality for persisting and retrieving workbench data.
//
// Run storage layout (implementation detail)
//
// Runs are stored in SQLite (data/workbench.db by default).
// Run-scoped directories for /workspace, /log, etc. remain on disk.
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
)

// LoadRun reads a run's state from SQLite by its run ID.
// It returns an error if the row cannot be read, if the JSON is malformed,
// or if the loaded data is missing critical fields like runID.
func LoadRun(cfg config.Config, runID string) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return types.Run{}, err
	}
	var b []byte
	if err := db.QueryRow(`SELECT run_json FROM runs WHERE run_id = ?`, runID).Scan(&b); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Run{}, fmt.Errorf("run %s does not exist: %w", runID, errors.Join(ErrNotFound, os.ErrNotExist))
		}
		return types.Run{}, fmt.Errorf("error reading run %s: %w", runID, err)
	}

	var run types.Run

	if err := json.Unmarshal(b, &run); err != nil {
		return types.Run{}, fmt.Errorf("error unmarshalling run json: %w", err)
	}

	if run.RunID == "" {
		return types.Run{}, fmt.Errorf("invalid run.json: missing runID: %w", ErrInvalid)
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
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
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
	_, err = tx.Exec(
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
		run.RunID,
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
	return tx.Commit()
}

// ReopenRun transitions a terminal run back to StatusRunning so it can be continued.
// It clears FinishedAt and Error for terminal runs. If already running, it is a no-op.
func ReopenRun(cfg config.Config, runID string) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	run, err := LoadRun(cfg, runID)
	if err != nil {
		return types.Run{}, err
	}
	if run.Status == types.StatusRunning {
		return run, nil
	}
	run.Status = types.StatusRunning
	run.FinishedAt = nil
	run.Error = nil
	return run, SaveRun(cfg, run)
}

// StopRun transitions a run to a terminal state (Done or Failed).
// It updates the Status, sets FinishedAt to the current time,
// and records an error message if the status is Failed.
// The updated state is then persisted to SQLite.
func StopRun(cfg config.Config, runID string, status types.RunStatus, errorMsg string) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}

	if status != types.StatusFailed && status != types.StatusDone && status != types.StatusCanceled {
		return types.Run{}, fmt.Errorf("error stopping run %s, invalid status '%s': %w", runID, status, ErrInvalid)
	}

	run, err := LoadRun(cfg, runID)
	if err != nil {
		return types.Run{}, fmt.Errorf("error stopping run: %w", err)
	}

	if run.Status == types.StatusDone || run.Status == types.StatusFailed || run.Status == types.StatusCanceled {
		return types.Run{}, fmt.Errorf("run %s cannot be stopped due to invalid state %s: %w", run.RunID, run.Status, ErrConflict)
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

// LatestRunningRun returns the most recently created run that is still StatusRunning.
// If no running run exists, it returns ErrNotFound.
func LatestRunningRun(cfg config.Config) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return types.Run{}, err
	}
	var raw string
	if err := db.QueryRow(
		`SELECT run_json FROM runs WHERE status = ? ORDER BY created_at DESC LIMIT 1`,
		string(types.StatusRunning),
	).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Run{}, ErrNotFound
		}
		return types.Run{}, err
	}
	var run types.Run
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		return types.Run{}, err
	}
	if run.RunID == "" {
		return types.Run{}, ErrInvalid
	}
	return run, nil
}

// LatestRun returns the most recently created run (any status).
func LatestRun(cfg config.Config) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return types.Run{}, err
	}
	var raw string
	if err := db.QueryRow(
		`SELECT run_json FROM runs ORDER BY created_at DESC LIMIT 1`,
	).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Run{}, ErrNotFound
		}
		return types.Run{}, err
	}
	var run types.Run
	if err := json.Unmarshal([]byte(raw), &run); err != nil {
		return types.Run{}, err
	}
	if run.RunID == "" {
		return types.Run{}, ErrInvalid
	}
	return run, nil
}
