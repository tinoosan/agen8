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

	if got, want := GetTeamDir(dataDir, teamID), filepath.Join(dataDir, "teams", teamID); got != want {
		t.Fatalf("GetTeamDir = %q, want %q", got, want)
	}
	if got, want := GetTeamWorkspaceDir(dataDir, teamID), filepath.Join(dataDir, "teams", teamID, "workspace"); got != want {
		t.Fatalf("GetTeamWorkspaceDir = %q, want %q", got, want)
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

	// Child run: workspace is under parent's workspace/subagents/<childRunID>
	run.ParentRunID = "run-parent"
	run.RunID = "run-child"
	want := filepath.Join(GetWorkspaceDir(dataDir, "run-parent"), "subagents", "run-child")
	if got := GetWorkspaceDirForRun(dataDir, run); got != want {
		t.Fatalf("GetWorkspaceDirForRun(child) = %q, want %q", got, want)
	}
}
