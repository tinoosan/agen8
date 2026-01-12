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

func GetRunDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId)
}

// GetWorkspaceDir returns the path to a run's workspace directory given the data directory and run ID.
func GetWorkspaceDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "workspace")
}

// GetArtifactDir returns the path to a run's artifact directory given the data directory and run ID.
func GetArtifactDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "artifacts")
}

// GetTraceDir returns the path to a run's trace directory given the data directory and run ID.
func GetTraceDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "trace")
}

// GetToolsDir returns the path to workbench tools directory given the data directory.
func GetToolsDir(dataDir string) string {
	return filepath.Join(dataDir, "tools")
}

// GetResultsDir returns the path to workbench results directory given the data directory and run ID.
func GetResultsDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "results")
}
