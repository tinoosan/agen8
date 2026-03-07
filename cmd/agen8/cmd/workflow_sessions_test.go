package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/app"
	authpkg "github.com/tinoosan/agen8/pkg/auth"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/protocol"
)

func TestAttachCommand_RequiresSessionID(t *testing.T) {
	prev := attachSessionID
	attachSessionID = ""
	t.Cleanup(func() { attachSessionID = prev })

	if err := attachCmd.RunE(attachCmd, nil); err == nil {
		t.Fatalf("expected error when no session id is provided")
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
	origRequire := requireRuntimeAuthReadyFn
	requireRuntimeAuthReadyFn = func(context.Context, config.Config) error {
		return fmt.Errorf("run `agen8 auth login --provider chatgpt_account`")
	}
	t.Cleanup(func() { requireRuntimeAuthReadyFn = origRequire })

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
	origRequire := requireRuntimeAuthReadyFn
	requireRuntimeAuthReadyFn = func(context.Context, config.Config) error {
		return fmt.Errorf("run `agen8 auth login --provider chatgpt_account`")
	}
	t.Cleanup(func() { requireRuntimeAuthReadyFn = origRequire })

	t.Setenv(authpkg.EnvAuthProvider, authpkg.ProviderChatGPTAccount)
	err := runCoordinatorForSession(coordinatorCmd, "sess-123")
	if err == nil {
		t.Fatalf("expected login-required error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "auth login") {
		t.Fatalf("expected relogin guidance, got: %v", err)
	}
}

func TestTeamStartCommand_RequiresProfileRef(t *testing.T) {
	if err := teamStartCmd.Args(teamStartCmd, nil); err == nil {
		t.Fatalf("expected missing profile-ref error")
	}
}

func TestListProfileRefs_ReturnsOnlyProfileDirectories(t *testing.T) {
	base := t.TempDir()
	profilesDir := filepath.Join(base, "profiles")
	if err := os.MkdirAll(filepath.Join(profilesDir, "startup_team"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "startup_team", "profile.yaml"), []byte("id: startup_team\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(profilesDir, "empty_dir"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	got := listProfileRefs(profilesDir)
	if len(got) != 1 || got[0] != "startup_team" {
		t.Fatalf("listProfileRefs=%v", got)
	}
}

func TestTeamList_ShowsSharedAndProjectProfiles(t *testing.T) {
	base := t.TempDir()
	prevDataDir := dataDir
	prevWorkDir := workDir
	dataDir = base
	workDir = base
	t.Cleanup(func() {
		dataDir = prevDataDir
		workDir = prevWorkDir
	})

	if _, err := app.InitProject(base, app.ProjectConfig{}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	sharedDir := filepath.Join(fsutil.GetProfilesDir(base), "startup_team")
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("MkdirAll shared: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "profile.yaml"), []byte("id: startup_team\n"), 0o644); err != nil {
		t.Fatalf("WriteFile shared: %v", err)
	}
	projectDir := filepath.Join(base, ".agen8", "profiles", "delivery_team")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "profile.yaml"), []byte("id: delivery_team\n"), 0o644); err != nil {
		t.Fatalf("WriteFile project: %v", err)
	}

	buf := new(strings.Builder)
	teamListCmd.SetOut(buf)
	teamListCmd.SetErr(buf)
	teamListCmd.SetContext(context.Background())
	t.Cleanup(func() {
		teamListCmd.SetOut(nil)
		teamListCmd.SetErr(nil)
		teamListCmd.SetContext(context.Background())
	})
	if err := teamListCmd.RunE(teamListCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"startup_team", "shared-profile", "delivery_team", "project-profile"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "DEFAULT") {
		t.Fatalf("unexpected default column in output:\n%s", out)
	}
}

func TestProjectInitHelp_HidesProfileDefaultFlags(t *testing.T) {
	buf := new(strings.Builder)
	projectInitCmd.SetOut(buf)
	projectInitCmd.SetErr(buf)
	t.Cleanup(func() {
		projectInitCmd.SetOut(nil)
		projectInitCmd.SetErr(nil)
	})
	if err := projectInitCmd.Help(); err != nil {
		t.Fatalf("Help: %v", err)
	}
	helpText := buf.String()
	for _, unwanted := range []string{"--profile ", "--team-profile", "\n      --mode "} {
		if strings.Contains(helpText, unwanted) {
			t.Fatalf("help unexpectedly contains %q:\n%s", unwanted, helpText)
		}
	}
}

func TestProjectStatus_DoesNotShowDefaultFields(t *testing.T) {
	base := t.TempDir()
	prevDataDir := dataDir
	prevWorkDir := workDir
	dataDir = base
	workDir = base
	t.Cleanup(func() {
		dataDir = prevDataDir
		workDir = prevWorkDir
	})

	if _, err := app.InitProject(base, app.ProjectConfig{
		DefaultProfile:     "general",
		DefaultTeamProfile: "startup_team",
	}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	buf := new(strings.Builder)
	projectStatusCmd.SetOut(buf)
	projectStatusCmd.SetErr(buf)
	projectStatusCmd.SetContext(context.Background())
	t.Cleanup(func() {
		projectStatusCmd.SetOut(nil)
		projectStatusCmd.SetErr(nil)
		projectStatusCmd.SetContext(context.Background())
	})
	if err := projectStatusCmd.RunE(projectStatusCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	out := buf.String()
	for _, unwanted := range []string{"default_team", "default_profile"} {
		if strings.Contains(out, unwanted) {
			t.Fatalf("status unexpectedly contains %q:\n%s", unwanted, out)
		}
	}
}

func TestProjectApply_RequiresInitializedProject(t *testing.T) {
	base := t.TempDir()
	prevWorkDir := workDir
	workDir = base
	t.Cleanup(func() { workDir = prevWorkDir })

	err := projectApplyCmd.RunE(projectApplyCmd, nil)
	if err == nil {
		t.Fatalf("expected error for uninitialized project")
	}
	if !strings.Contains(err.Error(), "project is not initialized") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProjectApply_PrintsAppliedDiff(t *testing.T) {
	base := t.TempDir()
	prevDataDir := dataDir
	prevWorkDir := workDir
	prevApply := rpcProjectApplyFn
	dataDir = base
	workDir = base
	rpcProjectApplyFn = func(_ context.Context, projectRoot string) (protocol.ProjectDiffResult, error) {
		return protocol.ProjectDiffResult{
			ProjectRoot: projectRoot,
			ProjectID:   "p1",
			Converged:   false,
			Status:      "reconciling",
			Actions: []protocol.ProjectReconcileAction{{
				Action:  "spawn",
				Profile: "dev_team",
				Reason:  "desired team is missing",
			}},
		}, nil
	}
	t.Cleanup(func() {
		dataDir = prevDataDir
		workDir = prevWorkDir
		rpcProjectApplyFn = prevApply
	})

	if _, err := app.InitProject(base, app.ProjectConfig{ProjectID: "p1"}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}

	buf := new(strings.Builder)
	projectApplyCmd.SetOut(buf)
	projectApplyCmd.SetErr(buf)
	projectApplyCmd.SetContext(context.Background())
	t.Cleanup(func() {
		projectApplyCmd.SetOut(nil)
		projectApplyCmd.SetErr(nil)
		projectApplyCmd.SetContext(context.Background())
	})
	if err := projectApplyCmd.RunE(projectApplyCmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	out := buf.String()
	for _, want := range []string{
		"applied=true",
		"project_id=p1",
		"desired_converged=false",
		"desired_status=reconciling",
		"action=spawn profile=dev_team team=- reason=desired team is missing",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}
