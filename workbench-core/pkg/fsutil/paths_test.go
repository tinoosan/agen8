package fsutil

import (
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestGetAgentsSkillsDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	gotHome, err := GetAgentsHomeDir()
	if err != nil {
		t.Fatalf("GetAgentsHomeDir: %v", err)
	}
	if want := filepath.Join(home, ".agents"); gotHome != want {
		t.Fatalf("GetAgentsHomeDir = %q, want %q", gotHome, want)
	}

	gotSkills, err := GetAgentsSkillsDir()
	if err != nil {
		t.Fatalf("GetAgentsSkillsDir: %v", err)
	}
	if want := filepath.Join(home, ".agents", "skills"); gotSkills != want {
		t.Fatalf("GetAgentsSkillsDir = %q, want %q", gotSkills, want)
	}
}

func TestGetAgentsHomeDir_EmptyHome(t *testing.T) {
	t.Setenv("HOME", "")
	if _, err := GetAgentsHomeDir(); err == nil {
		t.Fatalf("expected error when HOME is empty")
	}
}

func TestGetTeamPaths(t *testing.T) {
	dataDir := "/tmp/workbench-data"
	teamID := "team-abc"
	role := "backend-engineer"

	if got, want := GetTeamDir(dataDir, teamID), filepath.Join(dataDir, "teams", teamID); got != want {
		t.Fatalf("GetTeamDir = %q, want %q", got, want)
	}
	if got, want := GetTeamWorkspaceDir(dataDir, teamID), filepath.Join(dataDir, "teams", teamID, "workspace"); got != want {
		t.Fatalf("GetTeamWorkspaceDir = %q, want %q", got, want)
	}
	if got, want := GetTeamRoleWorkspaceDir(dataDir, teamID, role), filepath.Join(dataDir, "teams", teamID, "workspace", role); got != want {
		t.Fatalf("GetTeamRoleWorkspaceDir = %q, want %q", got, want)
	}
	if got, want := GetTeamTasksDir(dataDir, teamID), filepath.Join(dataDir, "teams", teamID, "tasks"); got != want {
		t.Fatalf("GetTeamTasksDir = %q, want %q", got, want)
	}
	if got, want := GetTeamRoleTasksDir(dataDir, teamID, role), filepath.Join(dataDir, "teams", teamID, "tasks", role); got != want {
		t.Fatalf("GetTeamRoleTasksDir = %q, want %q", got, want)
	}
	if got, want := GetTeamLogPath(dataDir, teamID), filepath.Join(dataDir, "teams", teamID, "daemon.log"); got != want {
		t.Fatalf("GetTeamLogPath = %q, want %q", got, want)
	}
}

func TestGetSubagentsDirAndRunDir(t *testing.T) {
	dataDir := "/var/data/workbench"
	parentRunID := "run-parent-1"
	childRunID := "run-child-1"

	subagentsDir := GetSubagentsDir(dataDir, parentRunID)
	wantSubagents := filepath.Join(dataDir, "agents", parentRunID, "subagents")
	if subagentsDir != wantSubagents {
		t.Fatalf("GetSubagentsDir = %q, want %q", subagentsDir, wantSubagents)
	}

	childRunDir := GetSubagentRunDir(dataDir, parentRunID, childRunID)
	wantChild := filepath.Join(wantSubagents, childRunID)
	if childRunDir != wantChild {
		t.Fatalf("GetSubagentRunDir = %q, want %q", childRunDir, wantChild)
	}

	if got := GetLogDirFromRunDir(childRunDir); got != filepath.Join(childRunDir, "log") {
		t.Fatalf("GetLogDirFromRunDir = %q", got)
	}
}

func TestGetRunDir(t *testing.T) {
	dataDir := "/var/data/workbench"

	// Top-level run: no ParentRunID
	run := types.Run{RunID: "run-1", ParentRunID: ""}
	if got, want := GetRunDir(dataDir, run), GetAgentDir(dataDir, "run-1"); got != want {
		t.Fatalf("GetRunDir(top-level) = %q, want %q", got, want)
	}

	// Child run: has ParentRunID
	run.ParentRunID = "run-parent"
	run.RunID = "run-child"
	if got, want := GetRunDir(dataDir, run), GetSubagentRunDir(dataDir, "run-parent", "run-child"); got != want {
		t.Fatalf("GetRunDir(child) = %q, want %q", got, want)
	}
}

func TestGetWorkspaceDirForRun(t *testing.T) {
	dataDir := "/var/data/workbench"

	// Top-level run: workspace is under agent dir
	run := types.Run{RunID: "run-1", ParentRunID: ""}
	if got, want := GetWorkspaceDirForRun(dataDir, run), GetWorkspaceDir(dataDir, "run-1"); got != want {
		t.Fatalf("GetWorkspaceDirForRun(top-level) = %q, want %q", got, want)
	}

	// Child run: workspace is under child run dir (parentRun/subagents/childID/workspace)
	run.ParentRunID = "run-parent"
	run.RunID = "run-child"
	want := filepath.Join(GetSubagentRunDir(dataDir, "run-parent", "run-child"), "workspace")
	if got := GetWorkspaceDirForRun(dataDir, run); got != want {
		t.Fatalf("GetWorkspaceDirForRun(child) = %q, want %q", got, want)
	}
}

func TestGetSubagentDeliverablesDir(t *testing.T) {
	dataDir := "/var/data/workbench"
	parentRunID := "run-parent"
	childRunID := "run-child"
	got := GetSubagentDeliverablesDir(dataDir, parentRunID, childRunID)
	want := filepath.Join(GetDeliverablesDir(dataDir, parentRunID), "subagents", childRunID)
	if got != want {
		t.Fatalf("GetSubagentDeliverablesDir = %q, want %q", got, want)
	}
}

func TestGetDeliverablesDir(t *testing.T) {
	dataDir := "/var/data/workbench"
	runID := "run-1"
	got := GetDeliverablesDir(dataDir, runID)
	want := filepath.Join(GetAgentDir(dataDir, runID), "deliverables")
	if got != want {
		t.Fatalf("GetDeliverablesDir = %q, want %q", got, want)
	}
}

func TestGetTasksDir(t *testing.T) {
	dataDir := "/var/data/workbench"
	runID := "run-1"
	got := GetTasksDir(dataDir, runID)
	want := filepath.Join(GetAgentDir(dataDir, runID), "tasks")
	if got != want {
		t.Fatalf("GetTasksDir = %q, want %q", got, want)
	}
}

func TestGetSubagentTasksDir(t *testing.T) {
	dataDir := "/var/data/workbench"
	parentRunID := "run-parent"
	childRunID := "run-child"
	got := GetSubagentTasksDir(dataDir, parentRunID, childRunID)
	want := filepath.Join(GetTasksDir(dataDir, parentRunID), "subagents", childRunID)
	if got != want {
		t.Fatalf("GetSubagentTasksDir = %q, want %q", got, want)
	}
}
