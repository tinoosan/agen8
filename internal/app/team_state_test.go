package app

import (
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/services/team"
)

func TestPersistTeamManifestModel_UpdatesExistingManifest(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	manifest := team.Manifest{
		TeamID:          "team-1",
		ProfileID:       "startup_team",
		TeamModel:       "openai/gpt-5",
		CoordinatorRole: "ceo",
		CoordinatorRun:  "run-1",
		Roles: []team.RoleRecord{
			{RoleName: "ceo", RunID: "run-1", SessionID: "sess-1"},
		},
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	if err := writeTeamManifestFile(cfg, manifest); err != nil {
		t.Fatalf("writeTeamManifestFile: %v", err)
	}

	if err := persistTeamManifestModel(cfg, "team-1", "moonshotai/kimi-k2.5", "rpc.control.setModel"); err != nil {
		t.Fatalf("persistTeamManifestModel: %v", err)
	}

	loaded, err := loadExistingTeamManifest(cfg, "team-1")
	if err != nil {
		t.Fatalf("loadExistingTeamManifest: %v", err)
	}
	if loaded == nil {
		t.Fatalf("expected manifest")
	}
	if got := strings.TrimSpace(loaded.TeamModel); got != "moonshotai/kimi-k2.5" {
		t.Fatalf("team model = %q, want %q", got, "moonshotai/kimi-k2.5")
	}
	if loaded.ModelChange == nil {
		t.Fatalf("expected modelChange")
	}
	if got := strings.TrimSpace(loaded.ModelChange.Status); got != "applied" {
		t.Fatalf("modelChange.status = %q, want applied", got)
	}
}
