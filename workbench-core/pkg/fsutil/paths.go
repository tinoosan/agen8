package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/types"
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

func GetTeamTasksDir(dataDir, teamID string) string {
	return filepath.Join(GetTeamDir(dataDir, teamID), "tasks")
}

// sanitizeRoleForPath returns a safe path segment for a role name (no path separators, no "..").
// Allows alphanumeric and "-_"; empty or invalid becomes "default".
func sanitizeRoleForPath(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range role {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "default"
	}
	return s
}

// GetTeamRoleWorkspaceDir returns the per-role workspace dir under the team: dataDir/teams/teamID/workspace/<role>.
func GetTeamRoleWorkspaceDir(dataDir, teamID, role string) string {
	return filepath.Join(GetTeamWorkspaceDir(dataDir, teamID), sanitizeRoleForPath(role))
}

func GetTeamRoleTasksDir(dataDir, teamID, role string) string {
	return filepath.Join(GetTeamTasksDir(dataDir, teamID), sanitizeRoleForPath(role))
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

// GetSubagentsDir returns the directory under a parent run where child run dirs live.
func GetSubagentsDir(dataDir, parentRunID string) string {
	return filepath.Join(GetAgentDir(dataDir, parentRunID), "subagents")
}

// GetSubagentRunDir returns the root directory for a child run (under the parent's subagents dir).
func GetSubagentRunDir(dataDir, parentRunID, childRunID string) string {
	return filepath.Join(GetSubagentsDir(dataDir, parentRunID), childRunID)
}

// GetSubagentLabel returns a stable human-readable standalone subagent label.
// Spawn indexes are 1-based; invalid indexes return "subagent-unknown".
func GetSubagentLabel(spawnIndex int) string {
	if spawnIndex <= 0 {
		return "subagent-unknown"
	}
	return fmt.Sprintf("subagent-%d", spawnIndex)
}

// GetStandaloneSubagentWorkspaceDir returns the parent-visible workspace directory
// for a standalone child run.
func GetStandaloneSubagentWorkspaceDir(dataDir, parentRunID string, spawnIndex int) string {
	return filepath.Join(GetWorkspaceDir(dataDir, parentRunID), GetSubagentLabel(spawnIndex))
}

// GetStandaloneSubagentTasksDir returns the parent-visible tasks directory
// for a standalone child run.
func GetStandaloneSubagentTasksDir(dataDir, parentRunID string, spawnIndex int) string {
	return filepath.Join(GetTasksDir(dataDir, parentRunID), GetSubagentLabel(spawnIndex))
}

// GetStandaloneSubagentPlanDir returns the parent-visible plan directory
// for a standalone child run.
func GetStandaloneSubagentPlanDir(dataDir, parentRunID string, spawnIndex int) string {
	return filepath.Join(GetAgentDir(dataDir, parentRunID), "plan", GetSubagentLabel(spawnIndex))
}

// GetRunDir returns the root directory for a run. Child runs (with ParentRunID set) live under the parent's subagents dir.
func GetRunDir(dataDir string, run types.Run) string {
	if strings.TrimSpace(run.ParentRunID) != "" {
		return GetSubagentRunDir(dataDir, run.ParentRunID, run.RunID)
	}
	return GetAgentDir(dataDir, run.RunID)
}

// GetWorkspaceDirForRun returns the workspace directory for a run. Subagents (with ParentRunID set)
// use a workspace under the child run dir: parentRun/subagents/childID/workspace.
func GetWorkspaceDirForRun(dataDir string, run types.Run) string {
	if strings.TrimSpace(run.ParentRunID) != "" {
		runDir := GetSubagentRunDir(dataDir, run.ParentRunID, run.RunID)
		return filepath.Join(runDir, "workspace")
	}
	return GetWorkspaceDir(dataDir, run.RunID)
}

// GetDeliverablesDir returns the run-level deliverables directory (same structure as tasks),
// not under workspace. Parent sees own deliverables at /deliverables and subagent at /deliverables/subagents/<childRunID>/.
func GetDeliverablesDir(dataDir, runID string) string {
	return filepath.Join(GetAgentDir(dataDir, runID), "deliverables")
}

// GetSubagentDeliverablesDir returns the on-disk path under the parent's run-level deliverables
// tree where a child run's deliverables are stored. The child writes to /deliverables (mounted here);
// the parent sees them at /deliverables/subagents/<childRunID>/.
func GetSubagentDeliverablesDir(dataDir, parentRunID, childRunID string) string {
	return filepath.Join(GetDeliverablesDir(dataDir, parentRunID), "subagents", childRunID)
}

// GetTasksDir returns the run-level task output directory (summaries, etc.), not under workspace.
func GetTasksDir(dataDir, runID string) string {
	return filepath.Join(GetAgentDir(dataDir, runID), "tasks")
}

// GetSubagentTasksDir returns the task output directory for a child run (under the parent's tasks tree).
func GetSubagentTasksDir(dataDir, parentRunID, childRunID string) string {
	return filepath.Join(GetTasksDir(dataDir, parentRunID), "subagents", childRunID)
}

// GetLogDirFromRunDir returns the log directory given a run's root dir.
func GetLogDirFromRunDir(runDir string) string {
	return filepath.Join(runDir, "log")
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
