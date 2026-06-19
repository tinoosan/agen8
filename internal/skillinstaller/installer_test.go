package skillinstaller

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestInstallWritesCodexSkillTree(t *testing.T) {
	home := t.TempDir()

	result, err := Install(Options{Harness: HarnessCodex, HomeDir: home})
	if err != nil {
		t.Fatalf("install skills: %v", err)
	}

	wantRoot := filepath.Join(home, ".codex", "skills")
	if result.Harness != HarnessCodex {
		t.Fatalf("harness = %q, want %q", result.Harness, HarnessCodex)
	}
	if result.Root != wantRoot {
		t.Fatalf("root = %q, want %q", result.Root, wantRoot)
	}
	if len(result.Skills) == 0 {
		t.Fatal("no skills installed")
	}
	// Every reported skill is written and matches its embedded source.
	for _, skill := range result.Skills {
		content, err := os.ReadFile(filepath.Join(wantRoot, skill, "SKILL.md"))
		if err != nil {
			t.Fatalf("read installed skill %s: %v", skill, err)
		}
		embeddedContent, err := embedded.ReadFile(embeddedRoot + "/" + skill + "/SKILL.md")
		if err != nil {
			t.Fatalf("read embedded skill %s: %v", skill, err)
		}
		if string(content) != string(embeddedContent) {
			t.Fatalf("installed skill %s does not match embedded source", skill)
		}
	}
	// The core agen8 skill must always be present.
	if !slices.Contains(result.Skills, "agen8") {
		t.Fatalf("skills = %v, want it to include agen8", result.Skills)
	}
	if !slices.Contains(result.Skills, "agen8-coordination") {
		t.Fatalf("skills = %v, want it to include agen8-coordination", result.Skills)
	}
}

func TestInstallWritesClaudeSkillTree(t *testing.T) {
	home := t.TempDir()

	result, err := Install(Options{Harness: "claude", HomeDir: home})
	if err != nil {
		t.Fatalf("install skills: %v", err)
	}

	if result.Harness != HarnessClaudeCLI {
		t.Fatalf("harness = %q, want %q", result.Harness, HarnessClaudeCLI)
	}
	wantRoot := filepath.Join(home, ".claude", "skills")
	if result.Root != wantRoot {
		t.Fatalf("root = %q, want %q", result.Root, wantRoot)
	}
	if _, err := os.Stat(filepath.Join(wantRoot, "agen8", "SKILL.md")); err != nil {
		t.Fatalf("core agen8 skill not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wantRoot, "agen8-coordination", "SKILL.md")); err != nil {
		t.Fatalf("coordination skill not installed: %v", err)
	}
}

func TestInstallRejectsUnknownHarness(t *testing.T) {
	_, err := Install(Options{Harness: "unknown", HomeDir: t.TempDir()})
	if err == nil {
		t.Fatal("expected unsupported harness error")
	}
}
