package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	pkgagent "github.com/tinoosan/workbench-core/pkg/services/agent"
	"github.com/tinoosan/workbench-core/pkg/services/team"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestTeamIsIdle_IgnoresHeartbeatTasks(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(t.TempDir()))
	if err != nil {
		t.Fatalf("new sqlite task store: %v", err)
	}
	now := time.Now().UTC()
	ctx := context.Background()

	if err := store.CreateTask(ctx, types.Task{
		TaskID:     "heartbeat-1",
		SessionID:  "sess-1",
		RunID:      "run-1",
		TeamID:     "team-1",
		TaskKind:   state.TaskKindHeartbeat,
		Status:     types.TaskStatusPending,
		Goal:       "heartbeat",
		CreatedAt:  &now,
		AssignedTo: "run-1",
	}); err != nil {
		t.Fatalf("create heartbeat task: %v", err)
	}

	if !team.IsTeamIdle(ctx, store, "team-1") {
		t.Fatalf("expected team to be idle with only heartbeat tasks")
	}
}

func TestTeamIsIdle_BlocksOnNonHeartbeatTasks(t *testing.T) {
	store, err := state.NewSQLiteTaskStore(fsutil.GetSQLitePath(t.TempDir()))
	if err != nil {
		t.Fatalf("new sqlite task store: %v", err)
	}
	now := time.Now().UTC()
	ctx := context.Background()

	if err := store.CreateTask(ctx, types.Task{
		TaskID:     "task-1",
		SessionID:  "sess-1",
		RunID:      "run-1",
		TeamID:     "team-1",
		TaskKind:   state.TaskKindCallback,
		Status:     types.TaskStatusPending,
		Goal:       "regular work",
		CreatedAt:  &now,
		AssignedTo: "run-1",
	}); err != nil {
		t.Fatalf("create non-heartbeat task: %v", err)
	}

	if team.IsTeamIdle(ctx, store, "team-1") {
		t.Fatalf("expected team to be non-idle with regular pending tasks")
	}
}

func TestBuildTeamRPCServerConfig_AcceptsRoleSessionThread(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	memStore := store.NewMemorySessionStore()
	_ = memStore.SaveSession(context.Background(), types.Session{SessionID: "coord-sess"})
	_ = memStore.SaveSession(context.Background(), types.Session{SessionID: "worker-sess"})
	sessionSvc := newTestSessionService(cfg, memStore)
	runtimes := []teamRoleRuntime{
		{run: types.Run{SessionID: "worker-sess", RunID: "worker-run"}},
	}
	controllers := make([]team.RoleRunController, len(runtimes))
	for i := range runtimes {
		controllers[i] = &teamRoleRunControllerAdapter{rt: &runtimes[i]}
	}
	teamCtrl := team.NewController(team.ControllerConfig{
		SessionService: sessionSvc,
		Runtimes:       controllers,
	})
	srvCfg := buildTeamRPCServerConfig(
		RPCServerConfig{},
		cfg,
		RunChatOptions{},
		types.Run{SessionID: "coord-sess", RunID: "coord-run"},
		nil,
		sessionSvc,
		runtimes,
		teamCtrl,
		nil,
	)

	if srvCfg.AgentService == nil {
		t.Fatal("AgentService not set")
	}
	err := srvCfg.AgentService.Pause(context.Background(), "", "worker-sess")
	if err == nil {
		t.Fatal("expected error for empty runID")
	}
	var se *pkgagent.ServiceError
	if errors.As(err, &se) {
		if se.Code != protocol.CodeInvalidParams {
			t.Fatalf("service error code=%d want=%d", se.Code, protocol.CodeInvalidParams)
		}
		return
	}
	pErr, ok := err.(*protocol.ProtocolError)
	if ok && pErr.Code == protocol.CodeInvalidParams {
		return
	}
	t.Fatalf("expected InvalidParams error, got %T: %v", err, err)
}

func TestBuildTeamRPCServerConfig_RejectsUnknownThread(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	memStore := store.NewMemorySessionStore()
	_ = memStore.SaveSession(context.Background(), types.Session{SessionID: "coord-sess"})
	sessionSvc := newTestSessionService(cfg, memStore)
	teamCtrl := team.NewController(team.ControllerConfig{
		SessionService: sessionSvc,
		Runtimes:       nil,
	})
	srvCfg := buildTeamRPCServerConfig(
		RPCServerConfig{},
		cfg,
		RunChatOptions{},
		types.Run{SessionID: "coord-sess", RunID: "coord-run"},
		nil,
		sessionSvc,
		nil,
		teamCtrl,
		nil,
	)

	_, err := srvCfg.ControlSetModel(context.Background(), "missing-thread", "", "openai/gpt-5-mini")
	pErr, ok := err.(*protocol.ProtocolError)
	if !ok {
		t.Fatalf("expected protocol error, got %T", err)
	}
	if pErr.Code != protocol.CodeThreadNotFound {
		t.Fatalf("protocol code=%d want=%d", pErr.Code, protocol.CodeThreadNotFound)
	}
}

func TestResolveTeamModel_FallsBackToRoleModel(t *testing.T) {
	model := resolveTeamModel(nil, &profile.TeamConfig{
		Roles: []profile.RoleConfig{
			{Name: "pm", Model: "openai/gpt-5-mini"},
		},
	}, RunChatOptions{})
	if model != "openai/gpt-5-mini" {
		t.Fatalf("model=%q", model)
	}
}

func TestResolveRoleModel_UsesRoleOverride(t *testing.T) {
	model := resolveRoleModel(profile.RoleConfig{Model: "openai/gpt-5-nano"}, "openai/gpt-5")
	if model != "openai/gpt-5-nano" {
		t.Fatalf("model=%q", model)
	}
}

func TestBuildRoleRuntimeProfile_UsesRoleScopedSkillsOnly(t *testing.T) {
	enabled := true
	role := profile.RoleConfig{
		Name:         "backend",
		Description:  "Backend role",
		Skills:       []string{"coding", "data-engineering"},
		CodeExecOnly: &enabled,
		AllowedTools: []string{"task_create"},
	}
	got := buildRoleRuntimeProfile(role)
	if got == nil {
		t.Fatalf("expected profile")
	}
	if got.ID != "backend" {
		t.Fatalf("id=%q", got.ID)
	}
	if len(got.Skills) != 2 || got.Skills[0] != "coding" || got.Skills[1] != "data-engineering" {
		t.Fatalf("unexpected skills: %v", got.Skills)
	}
	if len(got.AllowedTools) != 1 || got.AllowedTools[0] != "task_create" {
		t.Fatalf("unexpected allowed tools: %v", got.AllowedTools)
	}
	if !got.CodeExecOnly {
		t.Fatalf("expected code_exec_only copied from role override")
	}
}
