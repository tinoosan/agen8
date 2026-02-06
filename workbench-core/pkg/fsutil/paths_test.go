package fsutil

import (
	"path/filepath"
	"testing"
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
