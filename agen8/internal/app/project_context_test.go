package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitProjectAndLoadContext(t *testing.T) {
	base := t.TempDir()
	ctx, err := InitProject(base, ProjectConfig{
		DefaultProfile:     "software_dev",
		DefaultMode:        "team",
		DefaultTeamProfile: "startup_team",
		RPCEndpoint:        "127.0.0.1:7777",
	})
	if err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	if !ctx.Exists {
		t.Fatalf("expected initialized context")
	}
	if got := ctx.Config.DefaultProfile; got != "software_dev" {
		t.Fatalf("default profile=%q", got)
	}
	if got := ctx.Config.DefaultMode; got != "team" {
		t.Fatalf("default mode=%q", got)
	}
	if got := ctx.Config.DefaultTeamProfile; got != "startup_team" {
		t.Fatalf("default team profile=%q", got)
	}
	if got := ctx.Config.RPCEndpoint; got != "127.0.0.1:7777" {
		t.Fatalf("rpc endpoint=%q", got)
	}
}

func TestFindProjectRoot_WalksParents(t *testing.T) {
	base := t.TempDir()
	if _, err := InitProject(base, ProjectConfig{}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	child := filepath.Join(base, "a", "b", "c")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdirAll: %v", err)
	}
	root, ok, err := FindProjectRoot(child)
	if err != nil {
		t.Fatalf("FindProjectRoot: %v", err)
	}
	if !ok {
		t.Fatalf("expected project root")
	}
	if root != base {
		t.Fatalf("root=%q want=%q", root, base)
	}
}

func TestSetActiveSessionUpdatesState(t *testing.T) {
	base := t.TempDir()
	if _, err := InitProject(base, ProjectConfig{}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	ctx, err := SetActiveSession(base, ProjectState{
		ActiveSessionID: "sess-1",
		ActiveTeamID:    "team-1",
		ActiveRunID:     "run-1",
		LastCommand:     "attach",
	})
	if err != nil {
		t.Fatalf("SetActiveSession: %v", err)
	}
	if got := ctx.State.ActiveSessionID; got != "sess-1" {
		t.Fatalf("active session=%q", got)
	}
	if got := ctx.State.ActiveTeamID; got != "team-1" {
		t.Fatalf("active team=%q", got)
	}
	if got := ctx.State.ActiveRunID; got != "run-1" {
		t.Fatalf("active run=%q", got)
	}
	if got := ctx.State.LastCommand; got != "attach" {
		t.Fatalf("last command=%q", got)
	}
	if ctx.State.LastAttachedAt == "" {
		t.Fatalf("expected LastAttachedAt")
	}
}
