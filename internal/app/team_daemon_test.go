package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
)

type sessionLoaderStub struct {
	sessions map[string]types.Session
}

func (s sessionLoaderStub) LoadSession(_ context.Context, sessionID string) (types.Session, error) {
	if sess, ok := s.sessions[sessionID]; ok {
		return sess, nil
	}
	return types.Session{}, context.Canceled
}

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

func TestRunAsTeam_RequiresProfile(t *testing.T) {
	err := runAsTeam(context.Background(), config.Config{}, nil, "", "", 0, 0, RunChatOptions{}, false)
	if err == nil {
		t.Fatalf("expected profile required error")
	}
}

func TestRunAsTeam_GoalBootstrapIsRejected(t *testing.T) {
	prof := &profile.Profile{
		ID:          "general",
		Description: "Standalone",
		Model:       "openai/gpt-5-mini",
	}
	err := runAsTeam(context.Background(), config.Config{}, prof, "", "seed this goal", 8*1024, time.Second, RunChatOptions{}, false)
	if !errors.Is(err, errDaemonGoalBootstrapUnsupported) {
		t.Fatalf("expected errDaemonGoalBootstrapUnsupported, got: %v", err)
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected not supported message, got: %q", err.Error())
	}
}

func TestResolveTeamModelFromProfile_Standalone(t *testing.T) {
	prof := &profile.Profile{
		ID:          "general",
		Description: "Standalone",
		Model:       "openai/gpt-5-mini",
		Team:        nil,
	}
	model := resolveTeamModelFromProfile(nil, prof, RunChatOptions{})
	if model != "openai/gpt-5-mini" {
		t.Fatalf("model=%q want openai/gpt-5-mini", model)
	}
}

func TestBuildRoleRuntimeProfile_UsesRoleScopedSkillsOnly(t *testing.T) {
	enabled := true
	role := profile.RoleConfig{
		Name:                    "backend",
		Description:             "Backend role",
		Skills:                  []string{"coding", "data-engineering"},
		CodeExecOnly:            &enabled,
		CodeExecRequiredImports: []string{"pandas"},
		AllowedTools:            []string{"task_create"},
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
	if len(got.CodeExecRequiredImports) != 1 || got.CodeExecRequiredImports[0] != "pandas" {
		t.Fatalf("unexpected code_exec required imports: %v", got.CodeExecRequiredImports)
	}
}

func TestResolveWebhookRoutingContextFromRuns_PrefersCoordinatorTeamRun(t *testing.T) {
	now := time.Now().UTC()
	older := now.Add(-1 * time.Hour)
	runs := []types.Run{
		{
			RunID:       "run-standalone",
			SessionID:   "sess-standalone",
			ParentRunID: "",
			StartedAt:   &older,
			Runtime:     &types.RunRuntimeConfig{},
		},
		{
			RunID:       "run-ceo",
			SessionID:   "sess-team",
			ParentRunID: "",
			StartedAt:   &now,
			Runtime: &types.RunRuntimeConfig{
				TeamID: "team-1",
				Role:   "ceo",
			},
		},
	}
	roleSet := map[string]struct{}{"ceo": {}, "cto": {}}
	ctx, err := resolveWebhookRoutingContextFromRuns(context.Background(), runs, roleSet, "ceo", sessionLoaderStub{})
	if err != nil {
		t.Fatalf("resolve webhook route: %v", err)
	}
	if ctx.run.RunID != "run-ceo" {
		t.Fatalf("selected run=%q want run-ceo", ctx.run.RunID)
	}
	if ctx.teamID != "team-1" {
		t.Fatalf("teamID=%q want team-1", ctx.teamID)
	}
	if ctx.coordinatorRole != "ceo" {
		t.Fatalf("coordinatorRole=%q want ceo", ctx.coordinatorRole)
	}
	if _, ok := ctx.validRoles["cto"]; !ok {
		t.Fatalf("validRoles missing cto: %#v", ctx.validRoles)
	}
}

func TestResolveWebhookRoutingContextFromRuns_UsesSessionTeamIDFallback(t *testing.T) {
	now := time.Now().UTC()
	runs := []types.Run{
		{
			RunID:       "run-coordinator",
			SessionID:   "sess-1",
			ParentRunID: "",
			StartedAt:   &now,
			Runtime: &types.RunRuntimeConfig{
				Role: "ceo",
			},
		},
	}
	loader := sessionLoaderStub{
		sessions: map[string]types.Session{
			"sess-1": {SessionID: "sess-1", TeamID: "team-from-session"},
		},
	}
	ctx, err := resolveWebhookRoutingContextFromRuns(context.Background(), runs, map[string]struct{}{"ceo": {}}, "ceo", loader)
	if err != nil {
		t.Fatalf("resolve webhook route: %v", err)
	}
	if ctx.teamID != "team-from-session" {
		t.Fatalf("teamID=%q want team-from-session", ctx.teamID)
	}
	if ctx.coordinatorRole != "ceo" {
		t.Fatalf("coordinatorRole=%q want ceo", ctx.coordinatorRole)
	}
}

func TestResolveWebhookRoutingContextFromRuns_NoRootRunErrors(t *testing.T) {
	now := time.Now().UTC()
	runs := []types.Run{
		{
			RunID:       "run-child",
			SessionID:   "sess-1",
			ParentRunID: "run-parent",
			StartedAt:   &now,
			Runtime: &types.RunRuntimeConfig{
				TeamID: "team-1",
				Role:   "ceo",
			},
		},
	}
	_, err := resolveWebhookRoutingContextFromRuns(context.Background(), runs, nil, "", sessionLoaderStub{})
	if err == nil {
		t.Fatalf("expected error when no root run is available")
	}
}
