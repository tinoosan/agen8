package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/protocol"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

func TestSessionStart_RevivesInactiveProjectTeam(t *testing.T) {
	ctx := context.Background()
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
	sessionSvc := newTestSessionService(cfg, sessionStore)
	manifestStore := team.NewFileManifestStore(cfg)
	projectTeamSvc := NewProjectTeamService(cfg, sessionSvc, manifestStore)
	profileDir := filepath.Join(cfg.DataDir, "profiles", "general")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "profile.yaml"), []byte(
		"id: general\ndescription: Team\nteam:\n  model: openai/gpt-5-mini\n  roles:\n    - name: lead\n      coordinator: true\n      description: Lead\n      prompts:\n        systemPrompt: lead\n",
	), 0o644); err != nil {
		t.Fatalf("WriteFile(profile.yaml): %v", err)
	}
	if _, err := projectTeamSvc.RegisterTeam(ctx, ProjectTeamSummary{
		ProjectRoot:      projectRoot,
		ProjectID:        "p1",
		TeamID:           "team-alpha",
		ProfileID:        "general",
		PrimarySessionID: "",
		CoordinatorRunID: "",
		Status:           implstore.ProjectTeamStatusInactive,
	}); err != nil {
		t.Fatalf("RegisterTeam: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:               cfg,
		Run:               types.Run{},
		AllowAnyThread:    true,
		TaskService:       pkgtask.NewManager(nil, nil),
		Session:           sessionSvc,
		ManifestStore:     manifestStore,
		ProjectTeamSvc:    projectTeamSvc,
		Index:             protocol.NewIndex(0, 0),
		WorkspacePreparer: noopWorkspacePreparer{},
	})

	out, err := srv.sessionStart(ctx, protocol.SessionStartParams{
		ThreadID:    protocol.ThreadID("detached-control"),
		Profile:     "general",
		ProjectRoot: projectRoot,
		TeamID:      "team-alpha",
	})
	if err != nil {
		t.Fatalf("sessionStart: %v", err)
	}
	if got := strings.TrimSpace(out.TeamID); got != "team-alpha" {
		t.Fatalf("TeamID=%q want team-alpha", got)
	}

	teamSummary, err := projectTeamSvc.GetTeam(ctx, projectRoot, "team-alpha")
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got := strings.TrimSpace(teamSummary.Status); got != implstore.ProjectTeamStatusActive {
		t.Fatalf("status=%q want %q", got, implstore.ProjectTeamStatusActive)
	}
	if got := strings.TrimSpace(teamSummary.PrimarySessionID); got != strings.TrimSpace(out.SessionID) {
		t.Fatalf("primarySessionID=%q want %q", got, out.SessionID)
	}
	if got := strings.TrimSpace(teamSummary.CoordinatorRunID); got != strings.TrimSpace(out.PrimaryRunID) {
		t.Fatalf("coordinatorRunID=%q want %q", got, out.PrimaryRunID)
	}
	if got, _ := teamSummary.Metadata[profileFingerprintMetadataKey].(string); strings.TrimSpace(got) == "" {
		t.Fatalf("expected profile fingerprint metadata, got %+v", teamSummary.Metadata)
	}
}

func TestSessionStart_AllowsExplicitTeamIDWhenProjectTeamRecordWasDeleted(t *testing.T) {
	ctx := context.Background()
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
	sessionSvc := newTestSessionService(cfg, sessionStore)
	manifestStore := team.NewFileManifestStore(cfg)
	projectTeamSvc := NewProjectTeamService(cfg, sessionSvc, manifestStore)
	profileDir := filepath.Join(cfg.DataDir, "profiles", "general")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(profileDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "profile.yaml"), []byte(
		"id: general\ndescription: Team\nteam:\n  model: openai/gpt-5-mini\n  roles:\n    - name: lead\n      coordinator: true\n      description: Lead\n      prompts:\n        systemPrompt: lead\n",
	), 0o644); err != nil {
		t.Fatalf("WriteFile(profile.yaml): %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:               cfg,
		Run:               types.Run{},
		AllowAnyThread:    true,
		TaskService:       pkgtask.NewManager(nil, nil),
		Session:           sessionSvc,
		ManifestStore:     manifestStore,
		ProjectTeamSvc:    projectTeamSvc,
		Index:             protocol.NewIndex(0, 0),
		WorkspacePreparer: noopWorkspacePreparer{},
	})

	out, err := srv.sessionStart(ctx, protocol.SessionStartParams{
		ThreadID:    protocol.ThreadID("detached-control"),
		Profile:     "general",
		ProjectRoot: projectRoot,
		TeamID:      "team-recreated",
	})
	if err != nil {
		t.Fatalf("sessionStart: %v", err)
	}
	if got := strings.TrimSpace(out.TeamID); got != "team-recreated" {
		t.Fatalf("TeamID=%q want team-recreated", got)
	}
	teamSummary, err := projectTeamSvc.GetTeam(ctx, projectRoot, "team-recreated")
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got := strings.TrimSpace(teamSummary.PrimarySessionID); got != strings.TrimSpace(out.SessionID) {
		t.Fatalf("primarySessionID=%q want %q", got, out.SessionID)
	}
}

func TestRPCSessionDelete_PreservesActiveTeamSelection(t *testing.T) {
	ctx := context.Background()
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
	sessionSvc := newTestSessionService(cfg, sessionStore)
	manifestStore := team.NewFileManifestStore(cfg)
	projectTeamSvc := NewProjectTeamService(cfg, sessionSvc, manifestStore)

	sess := types.NewSession("goal")
	sess.ProjectRoot = projectRoot
	sess.TeamID = "team-alpha"
	sess.Profile = "general"
	run := types.NewRun("goal", 8*1024, sess.SessionID)
	sess.CurrentRunID = run.RunID
	sess.Runs = []string{run.RunID}
	if err := sessionSvc.SaveSession(ctx, sess); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if err := sessionSvc.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if _, err := projectTeamSvc.RegisterTeam(ctx, ProjectTeamSummary{
		ProjectRoot:      projectRoot,
		ProjectID:        "p1",
		TeamID:           sess.TeamID,
		ProfileID:        "general",
		PrimarySessionID: sess.SessionID,
		CoordinatorRunID: run.RunID,
		Status:           implstore.ProjectTeamStatusActive,
	}); err != nil {
		t.Fatalf("RegisterTeam: %v", err)
	}
	if _, err := SetActiveSession(projectRoot, ProjectState{
		ActiveSessionID: sess.SessionID,
		ActiveTeamID:    sess.TeamID,
		ActiveRunID:     run.RunID,
		LastCommand:     "team.start",
	}); err != nil {
		t.Fatalf("SetActiveSession: %v", err)
	}

	srv := NewRPCServer(RPCServerConfig{
		Cfg:            cfg,
		Run:            types.Run{},
		TaskService:    pkgtask.NewManager(nil, nil),
		Session:        sessionSvc,
		ManifestStore:  manifestStore,
		ProjectTeamSvc: projectTeamSvc,
		Index:          protocol.NewIndex(0, 0),
	})

	if _, err := srv.sessionDelete(ctx, protocol.SessionDeleteParams{SessionID: sess.SessionID}); err != nil {
		t.Fatalf("sessionDelete: %v", err)
	}

	projectCtx, err := LoadProjectContext(projectRoot)
	if err != nil {
		t.Fatalf("LoadProjectContext: %v", err)
	}
	if got := strings.TrimSpace(projectCtx.State.ActiveTeamID); got != sess.TeamID {
		t.Fatalf("ActiveTeamID=%q want %q", got, sess.TeamID)
	}
	if projectCtx.State.ActiveSessionID != "" {
		t.Fatalf("ActiveSessionID=%q want empty", projectCtx.State.ActiveSessionID)
	}
	if projectCtx.State.ActiveRunID != "" {
		t.Fatalf("ActiveRunID=%q want empty", projectCtx.State.ActiveRunID)
	}

	teamSummary, err := projectTeamSvc.GetTeam(ctx, projectRoot, sess.TeamID)
	if err != nil {
		t.Fatalf("GetTeam: %v", err)
	}
	if got := strings.TrimSpace(teamSummary.Status); got != implstore.ProjectTeamStatusInactive {
		t.Fatalf("status=%q want %q", got, implstore.ProjectTeamStatusInactive)
	}
}

type noopWorkspacePreparer struct{}

func (noopWorkspacePreparer) PrepareTeamWorkspace(context.Context, string) error {
	return nil
}
