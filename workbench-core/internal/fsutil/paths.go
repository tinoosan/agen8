package fsutil

import "path/filepath"

// GetRunFilePath returns the path to a run's run.json file given the data directory and run ID.
func GetRunFilePath(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "run.json")
}

// GetEventFilePath returns the path to a run's event.jsonl file given the data directory and run ID.
func GetEventFilePath(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "events.jsonl")
}
