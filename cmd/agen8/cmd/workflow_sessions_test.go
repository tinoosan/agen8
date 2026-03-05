package cmd

import (
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/app"
	authpkg "github.com/tinoosan/agen8/pkg/auth"
)

func TestAttachCommand_RequiresSessionID(t *testing.T) {
	prev := attachSessionID
	attachSessionID = ""
	t.Cleanup(func() { attachSessionID = prev })

	if err := attachCmd.RunE(attachCmd, nil); err == nil {
		t.Fatalf("expected error when no session id is provided")
	}
}

func TestProjectModeDefault(t *testing.T) {
	tests := []struct {
		configMode string
		want       string
	}{
		{"team", "multi-agent"},
		{"multi-agent", "multi-agent"},
		{"standalone", "single-agent"},
		{"single-agent", "single-agent"},
		{"", "single-agent"},
	}
	for _, tc := range tests {
		ctx := app.ProjectContext{
			Exists: true,
			Config: app.ProjectConfig{DefaultMode: tc.configMode},
		}
		if got := projectModeDefault(ctx); got != tc.want {
			t.Errorf("projectModeDefault(DefaultMode=%q)=%q want %q", tc.configMode, got, tc.want)
		}
	}
	// No project
	if got := projectModeDefault(app.ProjectContext{}); got != "single-agent" {
		t.Errorf("projectModeDefault(no project)=%q want single-agent", got)
	}
}

func TestProjectProfileDefault(t *testing.T) {
	ctx := app.ProjectContext{
		Exists: true,
		Config: app.ProjectConfig{
			DefaultProfile:     "general",
			DefaultTeamProfile: "startup_team",
		},
	}
	if got := projectProfileDefault(ctx, "single-agent"); got != "general" {
		t.Errorf("projectProfileDefault(single-agent)=%q want general", got)
	}
	if got := projectProfileDefault(ctx, "multi-agent"); got != "startup_team" {
		t.Errorf("projectProfileDefault(multi-agent)=%q want startup_team", got)
	}
	if got := projectProfileDefault(ctx, "team"); got != "startup_team" {
		t.Errorf("projectProfileDefault(team)=%q want startup_team", got)
	}
	ctx.Config.DefaultTeamProfile = ""
	if got := projectProfileDefault(ctx, "multi-agent"); got != "general" {
		t.Errorf("projectProfileDefault(multi-agent, no team profile)=%q want general", got)
	}
}

func TestResolveSessionDeleteTarget_UsesProjectActiveSession(t *testing.T) {
	base := t.TempDir()
	prevWD := workDir
	workDir = base
	t.Cleanup(func() { workDir = prevWD })

	if _, err := app.InitProject(base, app.ProjectConfig{}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	if _, err := app.SetActiveSession(base, app.ProjectState{
		ActiveSessionID: "sess-active",
		LastCommand:     "attach",
	}); err != nil {
		t.Fatalf("SetActiveSession: %v", err)
	}

	if got := resolveSessionDeleteTarget(nil); got != "sess-active" {
		t.Fatalf("resolveSessionDeleteTarget()=%q want %q", got, "sess-active")
	}
}

func TestMailCommand_RemovesTasksAlias(t *testing.T) {
	if mail := tasksCmd.Use; mail != "mail" {
		t.Fatalf("use=%q want mail", mail)
	}
	for _, alias := range tasksCmd.Aliases {
		if alias == "tasks" {
			t.Fatalf("unexpected tasks alias on mail command")
		}
	}
}

func TestRunNewSessionFlow_RequiresChatGPTLogin(t *testing.T) {
	prevDataDir := dataDir
	dataDir = t.TempDir()
	t.Cleanup(func() { dataDir = prevDataDir })

	t.Setenv(authpkg.EnvAuthProvider, authpkg.ProviderChatGPTAccount)
	err := runNewSessionFlow(newCmd, false)
	if err == nil {
		t.Fatalf("expected login-required error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "auth login") {
		t.Fatalf("expected relogin guidance, got: %v", err)
	}
}

func TestRunCoordinatorForSession_RequiresChatGPTLogin(t *testing.T) {
	prevDataDir := dataDir
	dataDir = t.TempDir()
	t.Cleanup(func() { dataDir = prevDataDir })

	t.Setenv(authpkg.EnvAuthProvider, authpkg.ProviderChatGPTAccount)
	err := runCoordinatorForSession(coordinatorCmd, "sess-123")
	if err == nil {
		t.Fatalf("expected login-required error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "auth login") {
		t.Fatalf("expected relogin guidance, got: %v", err)
	}
}
