package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/types"
)

func TestCreateRun(t *testing.T) {
	// Setup temporary data directory
	tmpDir := t.TempDir()
	oldDataDir := DataDir
	DataDir = tmpDir
	defer func() { DataDir = oldDataDir }()

	goal := "Test Goal"
	maxBytes := 1024

	run, err := CreateRun(goal, maxBytes)
	if err != nil {
		t.Fatalf("CreateRun failed: %v", err)
	}

	// Assert returned run fields
	if run.Goal != goal {
		t.Errorf("Expected goal %q, got %q", goal, run.Goal)
	}
	if run.MaxBytesForContext != maxBytes {
		t.Errorf("Expected maxBytes %d, got %d", maxBytes, run.MaxBytesForContext)
	}
	if run.Status != types.StatusRunning {
		t.Errorf("Expected status %q, got %q", types.StatusRunning, run.Status)
	}
	if run.StartedAt == nil {
		t.Error("Expected StartedAt to be set, got nil")
	}
	if run.RunId == "" {
		t.Error("Expected RunId to be set, got empty string")
	}

	// Verify run.json creation
	runDir := filepath.Join(tmpDir, "runs", run.RunId)
	jsonPath := filepath.Join(runDir, "run.json")

	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Fatalf("run.json was not created at %s", jsonPath)
	}

	// Verify JSON content
	b, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("Failed to read run.json: %v", err)
	}

	var savedRun types.Run
	if err := json.Unmarshal(b, &savedRun); err != nil {
		t.Fatalf("Failed to unmarshal run.json: %v", err)
	}

	if savedRun.RunId != run.RunId {
		t.Errorf("JSON RunId mismatch: expected %q, got %q", run.RunId, savedRun.RunId)
	}
	if savedRun.Goal != run.Goal {
		t.Errorf("JSON Goal mismatch: expected %q, got %q", run.Goal, savedRun.Goal)
	}
	if savedRun.Status != run.Status {
		t.Errorf("JSON Status mismatch: expected %q, got %q", run.Status, savedRun.Status)
	}

	// Verify omitempty fields are absent from JSON
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("Failed to unmarshal raw JSON: %v", err)
	}

	if _, exists := raw["finishedAt"]; exists {
		t.Error("finishedAt should be omitted from JSON when nil")
	}
	if _, exists := raw["error"]; exists {
		t.Error("error should be omitted from JSON when nil")
	}
}
