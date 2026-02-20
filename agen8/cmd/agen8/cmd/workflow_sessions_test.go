package cmd

import (
	"testing"

	"github.com/tinoosan/agen8/internal/app"
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

func TestMailCommand_KeepsTasksAlias(t *testing.T) {
	if mail := tasksCmd.Use; mail != "mail" {
		t.Fatalf("use=%q want mail", mail)
	}
	found := false
	for _, alias := range tasksCmd.Aliases {
		if alias == "tasks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected tasks alias on mail command")
	}
}
