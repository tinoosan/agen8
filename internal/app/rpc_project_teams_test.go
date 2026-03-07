package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/services/session"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

type testRuntimeState struct {
	diff protocol.ProjectDiffResult
}

func (t testRuntimeState) GetRunState(context.Context, string, string) (protocol.RuntimeRunState, error) {
	return protocol.RuntimeRunState{}, nil
}

func (t testRuntimeState) GetSessionState(context.Context, string) ([]protocol.RuntimeRunState, error) {
	return nil, nil
}

func (t testRuntimeState) DiffProject(context.Context, string) (protocol.ProjectDiffResult, error) {
	return t.diff, nil
}

func (t testRuntimeState) ApplyProject(context.Context, string) (protocol.ProjectDiffResult, error) {
	return t.diff, nil
}

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

func TestRPCServer_ProjectDeleteTeams_RemovesAllTeamsForProject(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot): %v", err)
	}
	if _, err := InitProject(projectRoot, ProjectConfig{ProjectID: "p1"}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	sessionStore, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		t.Fatalf("NewSQLiteSessionStore: %v", err)
	}
	sessionSvc := session.NewManager(cfg, sessionStore, nil)
	manifestStore := team.NewFileManifestStore(cfg)
	projectTeamSvc := NewProjectTeamService(cfg, sessionSvc, manifestStore)

	createTeam := func(teamID, sessionID, runID string) {
		sess := types.NewSession("goal")
		sess.SessionID = sessionID
		sess.ProjectRoot = projectRoot
		sess.TeamID = teamID
		sess.Profile = "startup"
		sess.CurrentRunID = runID
		sess.Runs = []string{runID}
		run := types.NewRun("goal", 8*1024, sessionID)
		run.RunID = runID
		if err := sessionSvc.SaveSession(context.Background(), sess); err != nil {
			t.Fatalf("SaveSession(%s): %v", teamID, err)
		}
		if err := sessionSvc.SaveRun(context.Background(), run); err != nil {
			t.Fatalf("SaveRun(%s): %v", teamID, err)
		}
		if err := manifestStore.Save(context.Background(), team.BuildManifest(
			teamID, "startup", "lead", runID, "openai/gpt-5-mini",
			[]team.RoleRecord{{RoleName: "lead", RunID: runID, SessionID: sessionID}}, nil, "",
		)); err != nil {
			t.Fatalf("SaveManifest(%s): %v", teamID, err)
		}
		if _, err := projectTeamSvc.RegisterTeam(context.Background(), ProjectTeamSummary{
			ProjectRoot:      projectRoot,
			ProjectID:        "p1",
			TeamID:           teamID,
			ProfileID:        "startup",
			PrimarySessionID: sessionID,
			CoordinatorRunID: runID,
			Status:           "active",
		}); err != nil {
			t.Fatalf("RegisterTeam(%s): %v", teamID, err)
		}
	}
	createTeam("team-a", "sess-a", "run-a")
	createTeam("team-b", "sess-b", "run-b")
	if _, err := SetActiveSession(projectRoot, ProjectState{
		ActiveSessionID: "sess-a",
		ActiveTeamID:    "team-a",
		ActiveRunID:     "run-a",
		LastCommand:     "team.start",
	}); err != nil {
		t.Fatalf("SetActiveSession: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:            cfg,
		Run:            types.Run{},
		TaskService:    pkgtask.NewManager(nil, nil),
		Session:        sessionSvc,
		ProjectTeamSvc: projectTeamSvc,
		ManifestStore:  manifestStore,
		Index:          protocol.NewIndex(0, 0),
	})
	out, err := srv.projectDeleteTeams(context.Background(), protocol.ProjectDeleteTeamsParams{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatalf("projectDeleteTeams: %v", err)
	}
	if len(out.DeletedTeamIDs) != 2 {
		t.Fatalf("DeletedTeamIDs=%v", out.DeletedTeamIDs)
	}
	listed, err := projectTeamSvc.ListTeams(context.Background(), projectRoot)
	if err != nil {
		t.Fatalf("ListTeams: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("listed teams=%v want empty", listed)
	}
	projectCtx, err := LoadProjectContext(projectRoot)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}
	if projectCtx.State.ActiveTeamID != "" || projectCtx.State.ActiveSessionID != "" {
		t.Fatalf("project state not cleared: %+v", projectCtx.State)
	}
}

func TestRPCServer_ProjectDiff_UsesRuntimeState(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	projectRoot := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(projectRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(projectRoot): %v", err)
	}
	if _, err := InitProject(projectRoot, ProjectConfig{ProjectID: "p1"}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg: cfg,
		Run: types.Run{},
		RuntimeState: testRuntimeState{
			diff: protocol.ProjectDiffResult{
				ProjectRoot: projectRoot,
				ProjectID:   "p1",
				Converged:   false,
				Status:      "drifting",
				Actions: []protocol.ProjectReconcileAction{{
					Action:  "spawn",
					Profile: "dev_team",
				}},
			},
		},
		Index: protocol.NewIndex(0, 0),
	})

	out, err := srv.projectDiff(context.Background(), protocol.ProjectDiffParams{ProjectRoot: projectRoot})
	if err != nil {
		t.Fatalf("projectDiff: %v", err)
	}
	if out.ProjectID != "p1" || out.Status != "drifting" || len(out.Actions) != 1 {
		t.Fatalf("unexpected diff: %+v", out)
	}
}
