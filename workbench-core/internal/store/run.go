// Package store provides functionality for persisting and retrieving workbench data.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/internal/types"
)

// CreateRun initializes a new run with the given goal and context limit.
// It creates a unique run ID, a corresponding directory in data/runs,
// and persists the initial run state as run.json.
func CreateRun(goal string, maxBytesForContext int) (types.Run, error) {
	runId := "run-" + uuid.NewString()
	runDir := filepath.Join("data", "runs", runId)
	targetPath := filepath.Join(runDir, "run.json")
	err := os.Mkdir(runDir, 0755)
	if err != nil {
		return types.Run{}, err
	}

	run := types.Run{
		RunId:              runId,
		Goal:               goal,
		Status:             types.StatusRunning,
		StartedAt:          time.Now(),
		MaxBytesForContext: maxBytesForContext,
		Err:                nil,
	}

	b, err := json.Marshal(run)
	if err != nil {
		return types.Run{}, err
	}

	if err := WriteFileAtomic(targetPath, b, 0755); err != nil {
		return types.Run{}, err
	}
	return run, nil
}

// WriteFileAtomic writes data to targetpath using an atomic rename operation.
// This ensures that the target file is either fully written or not written at all,
// preventing partial writes or corrupted files in case of a crash or power failure.
func WriteFileAtomic(targetpath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetpath)

	// Create a temporary file in the same directory as the target path.
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpName := tmpFile.Name()

	defer func() {
		_ = os.Remove(tmpName)
	}()

	// Write the data to the temporary file.
	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}

	// Sync ensuring data is physically written to disk.
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}

	// Atomically rename the temporary file to the target path.
	if err := os.Rename(tmpName, targetpath); err != nil {
		return fmt.Errorf("rename temp to target: %w", err)
	}

	return nil
}
