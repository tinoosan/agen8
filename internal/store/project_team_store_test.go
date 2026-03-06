package store

import (
	"context"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
)

func TestProjectTeamStore_CRUD(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	ctx := context.Background()

	record, err := UpsertProjectTeam(ctx, cfg, ProjectTeamRecord{
		ProjectRoot:      "/tmp/project-a",
		ProjectID:        "project-a",
		TeamID:           "team-alpha",
		ProfileID:        "startup",
		PrimarySessionID: "sess-1",
		CoordinatorRunID: "run-1",
		Status:           ProjectTeamStatusActive,
	})
	if err != nil {
		t.Fatalf("UpsertProjectTeam: %v", err)
	}
	if got := record.TeamID; got != "team-alpha" {
		t.Fatalf("teamID=%q want team-alpha", got)
	}

	loaded, err := LoadProjectTeam(ctx, cfg, "/tmp/project-a", "team-alpha")
	if err != nil {
		t.Fatalf("LoadProjectTeam: %v", err)
	}
	if loaded.PrimarySessionID != "sess-1" {
		t.Fatalf("primarySessionID=%q want sess-1", loaded.PrimarySessionID)
	}

	listed, err := ListProjectTeams(ctx, cfg, "/tmp/project-a")
	if err != nil {
		t.Fatalf("ListProjectTeams: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("len(listed)=%d want 1", len(listed))
	}

	if err := DeleteProjectTeam(ctx, cfg, "/tmp/project-a", "team-alpha"); err != nil {
		t.Fatalf("DeleteProjectTeam: %v", err)
	}
	if _, err := LoadProjectTeam(ctx, cfg, "/tmp/project-a", "team-alpha"); err == nil {
		t.Fatalf("expected deleted project team lookup to fail")
	}
}
