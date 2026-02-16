// Package store provides functionality for persisting and retrieving workbench data.
//
// Run storage layout (implementation detail)
//
// Runs are stored in SQLite (data/workbench.db by default).
// Run-scoped directories for /workspace, /log, etc. remain on disk.
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
	"github.com/tinoosan/workbench-core/pkg/timeutil"
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
	startedAt := timeutil.FormatRFC3339Nano(run.StartedAt)
	finishedAt := timeutil.FormatRFC3339Nano(run.FinishedAt)
	createdAt := startedAt
	if createdAt == "" {
		createdAt = now
	}
	_, err = tx.Exec(
		`INSERT INTO runs (run_id, session_id, status, goal, run_json, started_at, finished_at, created_at, updated_at, parent_run_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(run_id) DO UPDATE SET
		   session_id=excluded.session_id,
		   status=excluded.status,
		   goal=excluded.goal,
		   run_json=excluded.run_json,
		   started_at=excluded.started_at,
		   finished_at=excluded.finished_at,
		   created_at=COALESCE(runs.created_at, excluded.created_at),
		   updated_at=excluded.updated_at,
		   parent_run_id=excluded.parent_run_id`,
		run.RunID,
		run.SessionID,
		run.Status,
		run.Goal,
		string(b),
		nullIfEmpty(startedAt),
		nullIfEmpty(finishedAt),
		createdAt,
		now,
		strings.TrimSpace(run.ParentRunID),
	)
	if err != nil {
		return fmt.Errorf("error saving run: %w", err)
	}
	return tx.Commit()
}

// ReopenRun transitions a terminal run back to RunStatusRunning so it can be continued.
// It clears FinishedAt and Error for terminal runs. If already running, it is a no-op.
func ReopenRun(cfg config.Config, runID string) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}
	run, err := LoadRun(cfg, runID)
	if err != nil {
		return types.Run{}, err
	}
	if run.Status == types.RunStatusRunning {
		return run, nil
	}
	run.Status = types.RunStatusRunning
	run.FinishedAt = nil
	run.Error = nil
	return run, SaveRun(cfg, run)
}

// StopRun transitions a run to a terminal state (Succeeded, Failed, or Canceled).
// It updates the Status, sets FinishedAt to the current time,
// and records an error message if the status is Failed.
// The updated state is then persisted to SQLite.
func StopRun(cfg config.Config, runID string, status string, errorMsg string) (types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return types.Run{}, err
	}

	if status != types.RunStatusFailed && status != types.RunStatusSucceeded && status != types.RunStatusCanceled {
		return types.Run{}, fmt.Errorf("error stopping run %s, invalid status '%s': %w", runID, status, ErrInvalid)
	}

	run, err := LoadRun(cfg, runID)
	if err != nil {
		return types.Run{}, fmt.Errorf("error stopping run: %w", err)
	}

	if run.Status == types.RunStatusSucceeded || run.Status == types.RunStatusFailed || run.Status == types.RunStatusCanceled {
		return types.Run{}, fmt.Errorf("run %s cannot be stopped due to invalid state %s: %w", run.RunID, run.Status, ErrConflict)
	}

	now := time.Now()

	if status == types.RunStatusSucceeded {
		run.Status = status
		run.FinishedAt = &now
		run.Error = nil
	}

	if status == types.RunStatusFailed {

		if errorMsg == "" {
			return types.Run{}, fmt.Errorf("error stopping run, error message is required for failed runs: %w", ErrInvalid)
		}
		run.Status = status
		run.FinishedAt = &now
		run.Error = &errorMsg
	}

	if status == types.RunStatusCanceled {
		if strings.TrimSpace(errorMsg) == "" {
			errorMsg = "canceled"
		}
		run.Status = status
		run.FinishedAt = &now
		run.Error = &errorMsg
	}

	return run, SaveRun(cfg, run)
}

// LatestRunningRun returns the most recently created run that is still RunStatusRunning.
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
		types.RunStatusRunning,
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

// ListRunsByStatus returns all runs whose status is in the given list (e.g. running, paused).
// Used by the daemon supervisor to discover active runs without scanning sessions.
func ListRunsByStatus(cfg config.Config, statuses []string) ([]types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return nil, nil
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	placeholders := make([]string, len(statuses))
	args := make([]any, len(statuses))
	for i, s := range statuses {
		placeholders[i] = "?"
		args[i] = strings.TrimSpace(s)
	}
	query := `SELECT run_json FROM runs WHERE status IN (` + strings.Join(placeholders, ",") + `) ORDER BY created_at`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runs by status: %w", err)
	}
	defer rows.Close()
	var out []types.Run
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var run types.Run
		if err := json.Unmarshal([]byte(raw), &run); err != nil {
			continue
		}
		if run.RunID != "" {
			out = append(out, run)
		}
	}
	return out, rows.Err()
}

// ListChildRuns returns all runs whose parent_run_id matches the given parentRunID.
func ListChildRuns(cfg config.Config, parentRunID string) ([]types.Run, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	parentRunID = strings.TrimSpace(parentRunID)
	if parentRunID == "" {
		return nil, fmt.Errorf("parentRunID cannot be blank")
	}
	db, err := getSQLiteDB(cfg)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(`SELECT run_json FROM runs WHERE parent_run_id = ? ORDER BY created_at`, parentRunID)
	if err != nil {
		return nil, fmt.Errorf("error listing child runs: %w", err)
	}
	defer rows.Close()

	var out []types.Run
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var run types.Run
		if err := json.Unmarshal([]byte(raw), &run); err != nil {
			continue
		}
		if run.RunID != "" {
			out = append(out, run)
		}
	}
	return out, rows.Err()
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
