package store

import (
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
)

func TestCreateSessionAndAddRun(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	s, err := CreateSession("test session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if s.SessionID == "" {
		t.Fatalf("expected sessionId")
	}

	// Add a run and ensure it becomes current.
	updated, err := AddRunToSession(s.SessionID, "run-1")
	if err != nil {
		t.Fatalf("AddRunToSession: %v", err)
	}
	if updated.CurrentRunID != "run-1" {
		t.Fatalf("currentRunId=%q", updated.CurrentRunID)
	}
	if len(updated.Runs) != 1 || updated.Runs[0] != "run-1" {
		t.Fatalf("runs=%v", updated.Runs)
	}

	loaded, err := LoadSession(s.SessionID)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if loaded.CurrentRunID != "run-1" {
		t.Fatalf("loaded currentRunId=%q", loaded.CurrentRunID)
	}
}

func TestRecordTurnInSession_UpdatesGoalAndSummary(t *testing.T) {
	tmp := t.TempDir()
	old := config.DataDir
	config.DataDir = tmp
	defer func() { config.DataDir = old }()

	s, err := CreateSession("t")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	updated, err := RecordTurnInSession(s.SessionID, "run-1", "do the thing", "done")
	if err != nil {
		t.Fatalf("RecordTurnInSession: %v", err)
	}
	if updated.CurrentGoal == "" {
		t.Fatalf("expected currentGoal to be set")
	}
	if updated.Summary == "" {
		t.Fatalf("expected summary to be set")
	}
	if updated.UpdatedAt == nil {
		t.Fatalf("expected updatedAt to be set")
	}
}
