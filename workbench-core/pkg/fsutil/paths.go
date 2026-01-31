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

func GetRunFilePath(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "run.json")
}

func GetEventFilePath(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "events.jsonl")
}

func GetRunDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId)
}

func GetWorkspaceDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "workspace")
}

func GetArtifactDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "artifacts")
}

func GetLogDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "log")
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

func GetToolManifestPath(toolsDir, toolID string) string {
	return filepath.Join(toolsDir, toolID, "manifest.json")
}

func GetResultsDir(dataDir, runId string) string {
	return filepath.Join(dataDir, "runs", runId, "results")
}

func GetAgentDir(dataDir string) string {
	return filepath.Join(dataDir, "agent")
}

func GetProfileDir(dataDir string) string {
	return filepath.Join(dataDir, "profile")
}

func GetProfilePath(dataDir string) string {
	return filepath.Join(GetProfileDir(dataDir), "profile.md")
}

func GetProfileUpdatePath(dataDir string) string {
	return filepath.Join(GetProfileDir(dataDir), "update.md")
}

func GetProfileCommitsPath(dataDir string) string {
	return filepath.Join(GetProfileDir(dataDir), "commits.jsonl")
}

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
