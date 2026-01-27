package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func TestContextConstructor_ActiveSkillInjected(t *testing.T) {
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
		SelectedSkill: "demo-skill",
		LoadSession: func(sessionID string) (types.Session, error) {
			return types.Session{SessionID: sessionID}, nil
		},
		MaxProfileBytes: 0,
		MaxMemoryBytes:  0,
		MaxTraceBytes:   0,
		MaxHistoryBytes: 0,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if !strings.Contains(out, "<active_skill>") || !strings.Contains(out, "</active_skill>") {
		t.Fatalf("expected <active_skill> section, got: %q", out)
	}
	if !strings.Contains(out, "### demo-skill") {
		t.Fatalf("expected skill header, got: %q", out)
	}
	if !strings.Contains(out, "# Instructions") {
		t.Fatalf("expected skill content, got: %q", out)
	}
}

func TestContextConstructor_ActiveSkillOmittedWhenEmpty(t *testing.T) {
	t.Parallel()

	mgr := skills.NewManager([]string{t.TempDir()})
	_ = mgr.Scan()

	sess := types.Session{SessionID: "sess-test"}
	constructor := &ContextConstructor{
		FS:            vfs.NewFS(),
		RunID:         "run-test",
		SessionID:     "sess-test",
		SkillsManager: mgr,
		LoadSession: func(sessionID string) (types.Session, error) {
			return sess, nil
		},
		MaxProfileBytes: 0,
		MaxMemoryBytes:  0,
		MaxTraceBytes:   0,
		MaxHistoryBytes: 0,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if strings.Contains(out, "<active_skill>") {
		t.Fatalf("did not expect <active_skill> section, got: %q", out)
	}
}
