package fsutil

import (
	"path/filepath"
	"time"
)

func GetSQLitePath(dataDir string) string {
	return filepath.Join(dataDir, "workbench.db")
}

func GetSessionsDir(dataDir string) string {
	return filepath.Join(dataDir, "sessions")
}

func GetSessionDir(dataDir, sessionID string) string {
	return filepath.Join(GetSessionsDir(dataDir), sessionID)
}

func GetSessionFilePath(dataDir, sessionID string) string {
	return filepath.Join(GetSessionDir(dataDir, sessionID), "session.json")
}

func GetSessionHistoryDir(dataDir, sessionID string) string {
	return filepath.Join(GetSessionDir(dataDir, sessionID), "history")
}

func GetSessionHistoryPath(dataDir, sessionID string) string {
	return filepath.Join(GetSessionHistoryDir(dataDir, sessionID), "history.jsonl")
}

func GetRunFilePath(dataDir, runID string) string {
	return filepath.Join(dataDir, "runs", runID, "run.json")
}

func GetEventFilePath(dataDir, runID string) string {
	return filepath.Join(dataDir, "runs", runID, "events.jsonl")
}

func GetRunDir(dataDir, runID string) string {
	return filepath.Join(dataDir, "runs", runID)
}

func GetWorkspaceDir(dataDir, runID string) string {
	return filepath.Join(dataDir, "runs", runID, "workspace")
}

func GetArtifactDir(dataDir, runID string) string {
	return filepath.Join(dataDir, "runs", runID, "artifacts")
}

func GetLogDir(dataDir, runID string) string {
	return filepath.Join(dataDir, "runs", runID, "log")
}

func GetToolsDir(dataDir string) string {
	return filepath.Join(dataDir, "tools")
}

func GetSkillsDir(dataDir string) string {
	return filepath.Join(dataDir, "skills")
}

func GetRolesDir(dataDir string) string {
	return filepath.Join(dataDir, "roles")
}

func GetProfilesDir(dataDir string) string {
	return filepath.Join(dataDir, "profiles")
}

func GetToolManifestPath(toolsDir, toolID string) string {
	return filepath.Join(toolsDir, toolID, "manifest.json")
}

func GetResultsDir(dataDir, runID string) string {
	return filepath.Join(dataDir, "runs", runID, "results")
}

func GetAgentDir(dataDir string) string {
	return filepath.Join(dataDir, "agent")
}

func GetUserProfileDir(dataDir string) string {
	return filepath.Join(dataDir, "user_profile")
}

func GetUserProfilePath(dataDir string) string {
	return filepath.Join(GetUserProfileDir(dataDir), "user_profile.md")
}

func GetUserProfileUpdatePath(dataDir string) string {
	return filepath.Join(GetUserProfileDir(dataDir), "update.md")
}

func GetUserProfileCommitsPath(dataDir string) string {
	return filepath.Join(GetUserProfileDir(dataDir), "commits.jsonl")
}

// Deprecated: legacy naming; use GetUserProfileDir.
func GetProfileDir(dataDir string) string { return GetUserProfileDir(dataDir) }

// Deprecated: legacy naming; use GetUserProfilePath.
func GetProfilePath(dataDir string) string { return GetUserProfilePath(dataDir) }

// Deprecated: legacy naming; use GetUserProfileUpdatePath.
func GetProfileUpdatePath(dataDir string) string { return GetUserProfileUpdatePath(dataDir) }

// Deprecated: legacy naming; use GetUserProfileCommitsPath.
func GetProfileCommitsPath(dataDir string) string { return GetUserProfileCommitsPath(dataDir) }

func GetAgentMemoryPath(dataDir string) string {
	return filepath.Join(GetAgentDir(dataDir), "memory.md")
}

func GetAgentMemoryUpdatePath(dataDir string) string {
	return filepath.Join(GetAgentDir(dataDir), "update.md")
}

func GetMemoryDir(dataDir string) string {
	return filepath.Join(dataDir, "memory")
}

func GetMemoryMasterPath(dataDir string) string {
	return filepath.Join(GetMemoryDir(dataDir), "MEMORY.MD")
}

func GetDailyMemoryPath(dataDir string, date time.Time) string {
	return filepath.Join(GetMemoryDir(dataDir), date.Format("2006-01-02")+"-memory.md")
}
