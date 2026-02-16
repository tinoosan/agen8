package store

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func mustCreateSessionRun(t *testing.T, cfg config.Config, goal string, maxBytes int) types.Run {
	t.Helper()
	_, run, err := CreateSession(cfg, goal, maxBytes)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return run
}

func TestCreateRun(t *testing.T) {
	// Setup temporary data directory
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	goal := "Test Goal"
	maxBytes := 1024

	run := mustCreateSessionRun(t, cfg, goal, maxBytes)

	// Assert returned run fields
	if run.Goal != goal {
		t.Errorf("Expected goal %q, got %q", goal, run.Goal)
	}
	if run.MaxBytesForContext != maxBytes {
		t.Errorf("Expected maxBytes %d, got %d", maxBytes, run.MaxBytesForContext)
	}
	if run.Status != types.RunStatusRunning {
		t.Errorf("Expected status %q, got %q", types.RunStatusRunning, run.Status)
	}
	if run.StartedAt == nil {
		t.Error("Expected StartedAt to be set, got nil")
	}
	if run.RunID == "" {
		t.Error("Expected RunID to be set, got empty string")
	}
	if run.SessionID == "" {
		t.Error("Expected SessionID to be set, got empty string")
	}

	// Verify persisted content via LoadRun
	loaded, err := LoadRun(cfg, run.RunID)
	if err != nil {
		t.Fatalf("LoadRun failed: %v", err)
	}
	if loaded.RunID != run.RunID {
		t.Errorf("RunID mismatch: expected %q, got %q", run.RunID, loaded.RunID)
	}
	if loaded.Goal != run.Goal {
		t.Errorf("Goal mismatch: expected %q, got %q", run.Goal, loaded.Goal)
	}
	if loaded.Status != run.Status {
		t.Errorf("Status mismatch: expected %q, got %q", run.Status, loaded.Status)
	}
}

func TestLoadRun(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	t.Run("Success", func(t *testing.T) {
		goal := "Load Success Goal"
		run := mustCreateSessionRun(t, cfg, goal, 100)

		loaded, err := LoadRun(cfg, run.RunID)
		if err != nil {
			t.Fatalf("LoadRun failed: %v", err)
		}

		if loaded.RunID != run.RunID {
			t.Errorf("Expected RunID %q, got %q", run.RunID, loaded.RunID)
		}
		if loaded.Goal != goal {
			t.Errorf("Expected goal %q, got %q", goal, loaded.Goal)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		_, err := LoadRun(cfg, "non-existent-run")
		if err == nil {
			t.Error("Expected error for non-existent run, got nil")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected errors.Is(err, ErrNotFound) to be true, err=%v", err)
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected errors.Is(err, os.ErrNotExist) to be true, err=%v", err)
		}
	})

	t.Run("MalformedJSON", func(t *testing.T) {
		runId := "malformed-run"
		db, err := getSQLiteDB(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(
			`INSERT INTO runs (run_id, session_id, status, goal, run_json, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			runId,
			"sess-test",
			types.RunStatusRunning,
			"goal",
			"{invalid-json}",
			time.Now().UTC().Format(time.RFC3339Nano),
			time.Now().UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = LoadRun(cfg, runId)
		if err == nil {
			t.Error("Expected error for malformed JSON, got nil")
		}
	})

	t.Run("MissingRunID", func(t *testing.T) {
		runId := "missing-id-run"
		db, err := getSQLiteDB(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(
			`INSERT INTO runs (run_id, session_id, status, goal, run_json, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			runId,
			"sess-test",
			types.RunStatusRunning,
			"goal",
			`{"goal":"test"}`,
			time.Now().UTC().Format(time.RFC3339Nano),
			time.Now().UTC().Format(time.RFC3339Nano),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, err = LoadRun(cfg, runId)
		if err == nil {
			t.Error("Expected error for missing runId, got nil")
		}
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected errors.Is(err, ErrInvalid) to be true, err=%v", err)
		}
	})
}

func TestStopRun(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	t.Run("SuccessSucceeded", func(t *testing.T) {
		run := mustCreateSessionRun(t, cfg, "Stop Success", 100)

		stopped, err := StopRun(cfg, run.RunID, types.RunStatusSucceeded, "")
		if err != nil {
			t.Fatalf("StopRun failed: %v", err)
		}

		if stopped.Status != types.RunStatusSucceeded {
			t.Errorf("Expected status %q, got %q", types.RunStatusSucceeded, stopped.Status)
		}
		if stopped.FinishedAt == nil {
			t.Error("Expected FinishedAt to be set")
		}
		if stopped.Error != nil {
			t.Errorf("Expected Error to be nil, got %q", *stopped.Error)
		}

		// Verify persisted
		loaded, _ := LoadRun(cfg, run.RunID)
		if loaded.Status != types.RunStatusSucceeded {
			t.Errorf("Status expected %q, got %q", types.RunStatusSucceeded, loaded.Status)
		}
	})

	t.Run("SuccessFailed", func(t *testing.T) {
		run := mustCreateSessionRun(t, cfg, "Stop Failed Success", 100)

		errMsg := "some error occurred"
		stopped, err := StopRun(cfg, run.RunID, types.RunStatusFailed, errMsg)
		if err != nil {
			t.Fatalf("StopRun failed: %v", err)
		}

		if stopped.Status != types.RunStatusFailed {
			t.Errorf("Expected status %q, got %q", types.RunStatusFailed, stopped.Status)
		}
		if stopped.FinishedAt == nil {
			t.Error("Expected FinishedAt to be set")
		}
		if stopped.Error == nil || *stopped.Error != errMsg {
			t.Errorf("Expected Error %q, got %v", errMsg, stopped.Error)
		}
	})

	t.Run("ErrorMissingMessage", func(t *testing.T) {
		run := mustCreateSessionRun(t, cfg, "Stop Error Missing Msg", 100)

		_, err := StopRun(cfg, run.RunID, types.RunStatusFailed, "")
		if err == nil {
			t.Error("Expected error for missing error message, got nil")
		}
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected errors.Is(err, ErrInvalid) to be true, err=%v", err)
		}
	})

	t.Run("ErrorInvalidStatus", func(t *testing.T) {
		run := mustCreateSessionRun(t, cfg, "Stop Error Invalid Status", 100)

		_, err := StopRun(cfg, run.RunID, types.RunStatusRunning, "")
		if err == nil {
			t.Error("Expected error for transition to status 'running', got nil")
		}
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected errors.Is(err, ErrInvalid) to be true, err=%v", err)
		}
	})

	t.Run("ErrorAlreadyStopped", func(t *testing.T) {
		run := mustCreateSessionRun(t, cfg, "Stop Error Already Stopped", 100)

		_, err := StopRun(cfg, run.RunID, types.RunStatusSucceeded, "")
		if err != nil {
			t.Fatal(err)
		}

		// Try to stop again
		_, err = StopRun(cfg, run.RunID, types.RunStatusSucceeded, "")
		if err == nil {
			t.Error("Expected error for already stopped run, got nil")
		}
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("expected errors.Is(err, ErrConflict) to be true, err=%v", err)
		}
	})
}

func TestListRunsByStatus(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	t.Run("Empty", func(t *testing.T) {
		runs, err := ListRunsByStatus(cfg, []string{types.RunStatusRunning, types.RunStatusPaused})
		if err != nil {
			t.Fatalf("ListRunsByStatus: %v", err)
		}
		if len(runs) != 0 {
			t.Fatalf("expected 0 runs, got %d", len(runs))
		}
	})

	t.Run("ByStatus", func(t *testing.T) {
		r1 := mustCreateSessionRun(t, cfg, "running one", 100)
		_, r2, _ := CreateSession(cfg, "running two", 200)
		_ = SaveRun(cfg, r2)
		_, r3, _ := CreateSession(cfg, "paused run", 200)
		r3.Status = types.RunStatusPaused
		_ = SaveRun(cfg, r3)
		_, r4, _ := CreateSession(cfg, "succeeded run", 200)
		r4.Status = types.RunStatusSucceeded
		_ = SaveRun(cfg, r4)

		running, err := ListRunsByStatus(cfg, []string{types.RunStatusRunning})
		if err != nil {
			t.Fatalf("ListRunsByStatus(running): %v", err)
		}
		if len(running) != 2 {
			t.Fatalf("expected 2 running runs, got %d", len(running))
		}
		seen := map[string]bool{}
		for _, r := range running {
			seen[r.RunID] = true
			if r.Status != types.RunStatusRunning {
				t.Errorf("run %s has status %q", r.RunID, r.Status)
			}
		}
		if !seen[r1.RunID] || !seen[r2.RunID] {
			t.Errorf("expected r1 and r2 in running list, got %v", seen)
		}

		active, err := ListRunsByStatus(cfg, []string{types.RunStatusRunning, types.RunStatusPaused})
		if err != nil {
			t.Fatalf("ListRunsByStatus(running,paused): %v", err)
		}
		if len(active) != 3 {
			t.Fatalf("expected 3 active runs, got %d", len(active))
		}
	})
}
