// Package store provides functionality for persisting and retrieving workbench data.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinoosan/workbench-core/internal/types"
)

var DataDir = "data"

// CreateRun initializes a new run with the given goal and context limit.
// It creates a unique run ID, a corresponding directory in data/runs,
// and persists the initial run state as run.json.
func CreateRun(goal string, maxBytesForContext int) (types.Run, error) {
	run := types.NewRun(goal, maxBytesForContext)
	//fmt.Printf("run: %v", run)
	targetPath := runFilePath(run.RunId)
	err := os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		return types.Run{}, err
	}

	b, err := json.MarshalIndent(run, "", "\t")
	if err != nil {
		return types.Run{}, err
	}

	if err := WriteFileAtomic(targetPath, b, 0644); err != nil {
		return types.Run{}, err
	}
	return run, nil
}

// LoadRun reads a run's state from disk by its run ID.
// It returns an error if the file cannot be read, if the JSON is malformed,
// or if the loaded data is missing critical fields like runId.
func LoadRun(runId string) (types.Run, error) {
	targetPath := runFilePath(runId)
	b, err := os.ReadFile(targetPath)
	if err != nil {
		return types.Run{}, fmt.Errorf("error reading run.json file %s: %w", targetPath, err)
	}

	if errors.Is(err, os.ErrNotExist) {
		return types.Run{}, fmt.Errorf("run.json file %s does not exist", targetPath)
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

// runFilePath returns the absolute or relative path to a run's run.json file.
func runFilePath(runId string) string {
	return filepath.Join(DataDir, "runs", runId, "run.json")
}
