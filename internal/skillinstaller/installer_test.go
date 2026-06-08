package skillinstaller

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallWritesCodexSkill(t *testing.T) {
	home := t.TempDir()

	result, err := Install(Options{Harness: HarnessCodex, HomeDir: home})
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}

	wantPath := filepath.Join(home, ".codex", "skills", "agen8", "SKILL.md")
	if result.Harness != HarnessCodex {
		t.Fatalf("harness = %q, want %q", result.Harness, HarnessCodex)
	}
	if result.Path != wantPath {
		t.Fatalf("path = %q, want %q", result.Path, wantPath)
	}
	content, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read installed skill: %v", err)
	}
	if string(content) != string(EmbeddedSkill()) {
		t.Fatal("installed skill content does not match embedded skill")
	}
}

func TestInstallWritesClaudeSkill(t *testing.T) {
	home := t.TempDir()

	result, err := Install(Options{Harness: "claude", HomeDir: home})
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}

	wantPath := filepath.Join(home, ".claude", "skills", "agen8", "SKILL.md")
	if result.Harness != HarnessClaudeCLI {
		t.Fatalf("harness = %q, want %q", result.Harness, HarnessClaudeCLI)
	}
	if result.Path != wantPath {
		t.Fatalf("path = %q, want %q", result.Path, wantPath)
	}
}

func TestInstallRejectsUnknownHarness(t *testing.T) {
	_, err := Install(Options{Harness: "unknown", HomeDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected unsupported harness error")
	}
}
