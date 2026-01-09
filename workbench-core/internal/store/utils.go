package store

import "path/filepath"

// GetRunFilePath returns the absolute or relative path to a run's run.json file.
func GetRunFilePath(runId string) string {
	return filepath.Join(DataDir, "runs", runId, "run.json")
}


// GetEventFilePath returns the absolute or relative path to a run's event.jsonl file.
func GetEventFilePath(runId string) string {
	return filepath.Join(DataDir, "runs", runId, "event.jsonl")
}

