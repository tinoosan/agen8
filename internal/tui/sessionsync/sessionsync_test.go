package sessionsync

import (
	"testing"

	"github.com/tinoosan/agen8/internal/app"
)

func TestResolveActiveSessionID(t *testing.T) {
	root := t.TempDir()
	if _, err := app.InitProject(root, app.ProjectConfig{}); err != nil {
		t.Fatalf("init project: %v", err)
	}
	if _, err := app.SetActiveSession(root, app.ProjectState{ActiveSessionID: "sess-123"}); err != nil {
		t.Fatalf("set active: %v", err)
	}
	got, err := ResolveActiveSessionID(root)
	if err != nil {
		t.Fatalf("resolve active session: %v", err)
	}
	if got != "sess-123" {
		t.Fatalf("session id = %q, want sess-123", got)
	}
}

func TestResolveActiveSessionID_NoActive(t *testing.T) {
	root := t.TempDir()
	if _, err := app.InitProject(root, app.ProjectConfig{}); err != nil {
		t.Fatalf("init project: %v", err)
	}
	if _, err := ResolveActiveSessionID(root); err == nil {
		t.Fatalf("expected error when active session is missing")
	}
}
