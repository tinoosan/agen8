package obsidian

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDefaultVaultPath_UsesProjectDefault(t *testing.T) {
	t.Setenv("OBSIDIAN_VAULT_PATH", "")
	t.Setenv("HOME", t.TempDir())
	project := t.TempDir()
	got, err := ResolveDefaultVaultPath(project, "")
	if err != nil {
		t.Fatalf("ResolveDefaultVaultPath: %v", err)
	}
	want := filepath.Join(project, "obsidian-vault")
	if got.Host != want {
		t.Fatalf("host=%q want=%q", got.Host, want)
	}
	if got.Logical != "/project/obsidian-vault" {
		t.Fatalf("logical=%q", got.Logical)
	}
}

func TestResolveDefaultVaultPath_EnvWins(t *testing.T) {
	project := t.TempDir()
	t.Setenv("OBSIDIAN_VAULT_PATH", "/tmp/custom-vault")
	got, err := ResolveDefaultVaultPath(project, "")
	if err != nil {
		t.Fatalf("ResolveDefaultVaultPath: %v", err)
	}
	if got.Logical != "/tmp/custom-vault" {
		t.Fatalf("logical=%q", got.Logical)
	}
}

func TestResolveVaultPath_RejectsWorkspace(t *testing.T) {
	project := t.TempDir()
	t.Setenv("OBSIDIAN_ALLOW_WORKSPACE_PATH", "0")
	_, err := ResolveVaultPath(ResolveOptions{
		ExplicitPath: "/workspace/obsidian-vault",
		ProjectRoot:  project,
	})
	if err == nil {
		t.Fatalf("expected workspace path rejection")
	}
}

func TestResolveProjectVaultPath_ReadsConfig(t *testing.T) {
	project := t.TempDir()
	cfgDir := filepath.Join(project, ".agent8")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "obsidian_vault_path = \"/project/team-vault\"\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if got := ResolveProjectVaultPath(project); got != "/project/team-vault" {
		t.Fatalf("got=%q", got)
	}
}
