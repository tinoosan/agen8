package app

import (
	"context"
	"testing"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestRPCServer_ProjectListTeams_UsesRegistry(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	projectTeamSvc := NewProjectTeamService(cfg, newTestSessionService(cfg, sessionStore), nil)
	if _, err := projectTeamSvc.RegisterTeam(context.Background(), ProjectTeamSummary{
		ProjectRoot:      "/tmp/project-a",
		ProjectID:        "project-a",
		TeamID:           "team-a",
		ProfileID:        "startup",
		PrimarySessionID: "sess-a",
		CoordinatorRunID: "run-a",
		Status:           "active",
	}); err != nil {
		t.Fatalf("RegisterTeam: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:            cfg,
		Run:            types.Run{},
		TaskService:    pkgtask.NewManager(nil, nil),
		Session:        newTestSessionService(cfg, sessionStore),
		ProjectTeamSvc: projectTeamSvc,
		Index:          protocol.NewIndex(0, 0),
	})

	out, err := srv.projectListTeams(context.Background(), protocol.ProjectListTeamsParams{ProjectRoot: "/tmp/project-a"})
	if err != nil {
		t.Fatalf("projectListTeams: %v", err)
	}
	if len(out.Teams) != 1 || out.Teams[0].TeamID != "team-a" {
		t.Fatalf("teams=%+v", out.Teams)
	}
}
