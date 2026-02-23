package app

import (
	"context"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/profile"
	"github.com/tinoosan/agen8/pkg/services/team"
	"github.com/tinoosan/agen8/pkg/types"
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

func TestStoreBuilder_Validate_AcceptsStandaloneProfile(t *testing.T) {
	prof := &profile.Profile{
		ID:          "general",
		Description: "Standalone",
		Model:       "openai/gpt-5-mini",
		Prompts:     profile.PromptConfig{SystemPrompt: "You are helpful."},
		Team:        nil,
	}
	b := StoreBuilder{req: &teamRunRequest{prof: prof}}
	if err := b.Validate(); err != nil {
		t.Fatalf("Validate with standalone profile: %v", err)
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
