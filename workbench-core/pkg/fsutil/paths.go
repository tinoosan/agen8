package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GetSQLitePath(dataDir string) string {
	return filepath.Join(dataDir, "workbench.db")
}

func GetRPCSocketPath(dataDir string) string {
	return filepath.Join(dataDir, "rpc.sock")
}

func GetSessionsDir(dataDir string) string {
	return filepath.Join(dataDir, "sessions")
}

func GetSessionDir(dataDir, sessionID string) string {
	return filepath.Join(GetSessionsDir(dataDir), sessionID)
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

func GetTeamDir(dataDir, teamID string) string {
	return filepath.Join(dataDir, "teams", teamID)
}

func GetTeamWorkspaceDir(dataDir, teamID string) string {
	return filepath.Join(GetTeamDir(dataDir, teamID), "workspace")
}

func GetTeamLogPath(dataDir, teamID string) string {
	return filepath.Join(GetTeamDir(dataDir, teamID), "daemon.log")
}

func GetWorkspaceDir(dataDir, runID string) string {
	return filepath.Join(GetAgentDir(dataDir, runID), "workspace")
}

func GetLogDir(dataDir, runID string) string {
	return filepath.Join(GetAgentDir(dataDir, runID), "log")
}

func GetSkillsDir(dataDir string) string {
	return filepath.Join(dataDir, "skills")
}

func GetAgentsHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return filepath.Join(home, ".agents"), nil
}

func GetAgentsSkillsDir() (string, error) {
	agentsHome, err := GetAgentsHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(agentsHome, "skills"), nil
}

func GetProfilesDir(dataDir string) string {
	return filepath.Join(dataDir, "profiles")
}

func GetMemoryDir(dataDir string) string {
	return filepath.Join(dataDir, "memory")
}
