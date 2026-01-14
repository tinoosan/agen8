// Package fsutil provides filesystem path helpers for workbench data layout.
//
// All run data is stored under:
//
//	<dataDir>/runs/<runId>/
//
// This package centralizes path construction so the layout is consistent
// and easy to change.
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

// GetRunDir returns the base directory for a run given the data directory and run ID.
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

// GetToolManifestPath returns the path to a tool's manifest.json given the tools directory and tool ID.
func GetToolManifestPath(toolsDir, toolID string) string {
	return filepath.Join(toolsDir, toolID, "manifest.json")
}

// GetResultsDir returns the path to workbench results directory given the data directory and run ID.
func GetResultsDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "results")
}

// GetAgentDir returns the base directory for cross-run agent state.
func GetAgentDir(dataDir string) string {
	return filepath.Join(dataDir, "agent")
}

// GetAgentMemoryPath returns the path to the persistent cross-run memory markdown file.
func GetAgentMemoryPath(dataDir string) string {
	return filepath.Join(GetAgentDir(dataDir), "memory.md")
}

// GetAgentMemoryUpdatePath returns the path to the per-turn memory update staging file.
//
// The host ingests this file after each agent turn and appends it to memory.md.
func GetAgentMemoryUpdatePath(dataDir string) string {
	return filepath.Join(GetAgentDir(dataDir), "update.md")
}

// GetRunMemoryDir returns the path to a run-scoped memory directory.
//
// This is where per-run agent memory state can live (separate from global history).
func GetRunMemoryDir(dataDir, runId string) string {
	return filepath.Join(GetRunDir(dataDir, runId), "memory")
}

// GetRunMemoryPath returns the path to a run-scoped memory markdown file.
func GetRunMemoryPath(dataDir, runId string) string {
	return filepath.Join(GetRunMemoryDir(dataDir, runId), "memory.md")
}

// GetRunMemoryUpdatePath returns the path to a run-scoped memory update staging file.
func GetRunMemoryUpdatePath(dataDir, runId string) string {
	return filepath.Join(GetRunMemoryDir(dataDir, runId), "update.md")
}

// GetRunHistoryDir returns the path to a run-scoped history directory.
//
// History is an immutable, append-only log of raw interactions between:
// - users
// - agents
// - the environment/host
//
// It is separate from /trace (which is a curated event feed for agents) and from
// /memory (which is curated, host-governed long-term notes).
func GetRunHistoryDir(dataDir, runId string) string {
	return filepath.Join(GetRunDir(dataDir, runId), "history")
}

// GetRunHistoryPath returns the path to the run-scoped history JSONL file.
func GetRunHistoryPath(dataDir, runId string) string {
	return filepath.Join(GetRunHistoryDir(dataDir, runId), "history.jsonl")
}
