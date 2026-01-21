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

// GetSessionsDir returns the path to the sessions directory given the data directory.
//
// Sessions group runs and provide shared, append-only history.
func GetSessionsDir(dataDir string) string {
	return filepath.Join(dataDir, "sessions")
}

// GetSessionDir returns the base directory for a session.
func GetSessionDir(dataDir, sessionID string) string {
	return filepath.Join(GetSessionsDir(dataDir), sessionID)
}

// GetSessionFilePath returns the path to a session's session.json file.
func GetSessionFilePath(dataDir, sessionID string) string {
	return filepath.Join(GetSessionDir(dataDir, sessionID), "session.json")
}

// GetSessionHistoryDir returns the path to a session-scoped history directory.
func GetSessionHistoryDir(dataDir, sessionID string) string {
	return filepath.Join(GetSessionDir(dataDir, sessionID), "history")
}

// GetSessionHistoryPath returns the path to the session-scoped history JSONL file.
func GetSessionHistoryPath(dataDir, sessionID string) string {
	return filepath.Join(GetSessionHistoryDir(dataDir, sessionID), "history.jsonl")
}

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

// GetScratchDir returns the path to a run's scratch directory given the data directory and run ID.
func GetScratchDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "scratch")
}

// GetArtifactDir returns the path to a run's artifact directory given the data directory and run ID.
func GetArtifactDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "artifacts")
}

// GetLogDir returns the path to a run's log directory given the data directory and run ID.
func GetLogDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "log")
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

// GetProfileDir returns the base directory for global user profile memory.
//
// Profile is shared across runs and sessions (unlike run-scoped /memory).
func GetProfileDir(dataDir string) string {
	return filepath.Join(dataDir, "profile")
}

// GetProfilePath returns the path to the committed profile markdown file.
func GetProfilePath(dataDir string) string {
	return filepath.Join(GetProfileDir(dataDir), "profile.md")
}

// GetProfileUpdatePath returns the path to the profile update staging file.
func GetProfileUpdatePath(dataDir string) string {
	return filepath.Join(GetProfileDir(dataDir), "update.md")
}

// GetProfileCommitsPath returns the path to the profile commits audit JSONL file.
func GetProfileCommitsPath(dataDir string) string {
	return filepath.Join(GetProfileDir(dataDir), "commits.jsonl")
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

// NOTE: history is session-scoped (data/sessions/<sessionId>/history/history.jsonl).
