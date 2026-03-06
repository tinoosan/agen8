package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/agen8/pkg/config"
)

func TestResolveRoleAllowSubagents_StandaloneProfile(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	// Seed a standalone profile (no team block)
	profDir := filepath.Join(cfg.DataDir, "profiles", "general")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "prompt.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "profile.yaml"), []byte(`
id: general
description: General
prompts:
  systemPromptPath: prompt.md
`), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// Standalone profile (no team): should allow subagents
	if !ResolveRoleAllowSubagents(cfg, "general", "") {
		t.Fatalf("standalone profile should allow subagents")
	}
}

func TestResolveRoleAllowSubagents_TeamProfile(t *testing.T) {
	cfg := config.Config{DataDir: t.TempDir()}
	profDir := filepath.Join(cfg.DataDir, "profiles", "team-test")
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "prompt.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "profile.yaml"), []byte(`
id: team-test
description: Team
team:
  roles:
    - name: ceo
      coordinator: true
      description: CEO
      prompts:
        systemPromptPath: prompt.md
      allowSubagents: true
    - name: worker
      description: Worker
      prompts:
        systemPromptPath: prompt.md
`), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	if !ResolveRoleAllowSubagents(cfg, "team-test", "ceo") {
		t.Fatalf("ceo with allowSubagents: true should allow")
	}
	if ResolveRoleAllowSubagents(cfg, "team-test", "worker") {
		t.Fatalf("worker without allow_subagents should not allow")
	}
	if ResolveRoleAllowSubagents(cfg, "team-test", "unknown") {
		t.Fatalf("unknown role should not allow")
	}
}
