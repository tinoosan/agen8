package app

import (
	"os"
	"path/filepath"
	"strings"
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
	if got := ctx.Config.DefaultMode; got != "multi-agent" {
		t.Fatalf("default mode=%q (team normalizes to multi-agent)", got)
	}
	if got := ctx.Config.DefaultTeamProfile; got != "startup_team" {
		t.Fatalf("default team profile=%q", got)
	}
	if got := ctx.Config.RPCEndpoint; got != "127.0.0.1:7777" {
		t.Fatalf("rpc endpoint=%q", got)
	}
	if !strings.Contains(ctx.ProjectDir, ".agen8") {
		t.Fatalf("project dir=%q; expected .agen8", ctx.ProjectDir)
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

func TestSaveProjectConfig_PersistsObsidianFields(t *testing.T) {
	base := t.TempDir()
	if _, err := InitProject(base, ProjectConfig{}); err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	ctx, err := SaveProjectConfig(base, ProjectConfig{
		ProjectID:         "p1",
		DefaultMode:       "standalone",
		ObsidianVaultPath: "/project/obsidian-vault",
		ObsidianEnabled:   true,
	})
	if err != nil {
		t.Fatalf("SaveProjectConfig: %v", err)
	}
	if got := ctx.Config.ObsidianVaultPath; got != "/project/obsidian-vault" {
		t.Fatalf("obsidian vault path=%q", got)
	}
	if !ctx.Config.ObsidianEnabled {
		t.Fatalf("expected obsidian enabled")
	}
}

func TestNormalizeProjectConfig_ModeValues(t *testing.T) {
	base := t.TempDir()
	tests := []struct {
		input string
		want  string
	}{
		{"team", "multi-agent"},
		{"multi-agent", "multi-agent"},
		{"standalone", "single-agent"},
		{"single-agent", "single-agent"},
		{"", "single-agent"},
		{"invalid", "single-agent"},
	}
	for _, tc := range tests {
		cfg := ProjectConfig{DefaultMode: tc.input}
		norm := normalizeProjectConfig(cfg, base)
		if got := norm.DefaultMode; got != tc.want {
			t.Errorf("normalizeProjectConfig(DefaultMode=%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestInitProject_MigratesLegacyDotAgent8(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, ".agent8")
	if err := os.MkdirAll(filepath.Join(legacy, "profiles"), 0o755); err != nil {
		t.Fatalf("mkdir legacy: %v", err)
	}
	legacyCfg := strings.Join([]string{
		`project_id = "legacy-project"`,
		`default_mode = "team"`,
		`rpc_endpoint = "127.0.0.1:7999"`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(legacy, "config.toml"), []byte(legacyCfg), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "state.json"), []byte(`{"active_session_id":"sess-legacy"}`), 0o644); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	ctx, err := InitProject(base, ProjectConfig{})
	if err != nil {
		t.Fatalf("InitProject: %v", err)
	}
	if !ctx.Exists {
		t.Fatalf("expected initialized context")
	}
	if got := strings.TrimSpace(ctx.Config.ProjectID); got != "legacy-project" {
		t.Fatalf("project id=%q", got)
	}
	if got := strings.TrimSpace(ctx.State.ActiveSessionID); got != "sess-legacy" {
		t.Fatalf("active session=%q", got)
	}
	if _, err := os.Stat(filepath.Join(base, ".agen8", "MIGRATED_FROM_AGENT8")); err != nil {
		t.Fatalf("expected migration marker: %v", err)
	}
}
