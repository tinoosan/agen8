package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func TestContextConstructor_SkillsMetadataInjected(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	skillDir := filepath.Join(baseDir, "demo-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	skillContent := "---\nname: Demo Skill\ndescription: Demo\n---\n# Instructions\nDo demo.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	mgr := skills.NewManager([]string{baseDir})
	if err := mgr.Scan(); err != nil {
		t.Fatalf("skills scan: %v", err)
	}

	constructor := &ContextConstructor{
		FS:            vfs.NewFS(),
		RunID:         "run-test",
		SessionID:     "sess-test",
		SkillsManager: mgr,
		MaxProfileBytes: 0,
		MaxMemoryBytes:  0,
		MaxTraceBytes:   0,
		MaxHistoryBytes: 0,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if !strings.Contains(out, "<available_skills>") || !strings.Contains(out, "</available_skills>") {
		t.Fatalf("expected <available_skills> section, got: %q", out)
	}
	if !strings.Contains(out, "Demo Skill") || !strings.Contains(out, "demo-skill") {
		t.Fatalf("expected skill metadata, got: %q", out)
	}
	if strings.Contains(out, "# Instructions") || strings.Contains(out, "Do demo.") {
		t.Fatalf("did not expect full skill content, got: %q", out)
	}
}

func TestContextConstructor_SkillsOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	mgr := skills.NewManager([]string{t.TempDir()})
	_ = mgr.Scan()

	constructor := &ContextConstructor{
		FS:            vfs.NewFS(),
		RunID:         "run-test",
		SessionID:     "sess-test",
		SkillsManager: mgr,
		MaxProfileBytes: 0,
		MaxMemoryBytes:  0,
		MaxTraceBytes:   0,
		MaxHistoryBytes: 0,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if strings.Contains(out, "<available_skills>") {
		t.Fatalf("did not expect <available_skills> section, got: %q", out)
	}
}
