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

func GetAgentsDir(dataDir string) string {
	return filepath.Join(dataDir, "agents")
}

func GetAgentDir(dataDir, agentID string) string {
	return filepath.Join(GetAgentsDir(dataDir), agentID)
}

func GetWorkspaceDir(dataDir, runID string) string {
	return filepath.Join(GetAgentDir(dataDir, runID), "workspace")
}

func GetArtifactDir(dataDir, runID string) string {
	return filepath.Join(GetAgentDir(dataDir, runID), "artifacts")
}

func GetLogDir(dataDir, runID string) string {
	return filepath.Join(GetAgentDir(dataDir, runID), "log")
}

func GetSkillsDir(dataDir string) string {
	return filepath.Join(dataDir, "skills")
}

func GetProfilesDir(dataDir string) string {
	return filepath.Join(dataDir, "profiles")
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
