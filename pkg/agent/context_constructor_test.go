package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/resources"
	"github.com/tinoosan/agen8/pkg/skills"
	"github.com/tinoosan/agen8/pkg/vfs"
)

func TestPromptBuilder_IncludesMemory(t *testing.T) {
	t.Parallel()

	fs := vfs.NewFS()
	memDir := t.TempDir()

	memRes, err := resources.NewDirResource(memDir, vfs.MountMemory)
	if err != nil {
		t.Fatalf("NewDirResource(memory): %v", err)
	}
	if err := fs.Mount(vfs.MountMemory, memRes); err != nil {
		t.Fatalf("mount memory: %v", err)
	}

	today := time.Now().Format("2006-01-02") + "-memory.md"
	if err := os.WriteFile(filepath.Join(memDir, today), []byte("remember this"), 0644); err != nil {
		t.Fatalf("write daily memory: %v", err)
	}

	constructor := &PromptBuilder{
		FS:             fs,
		MaxMemoryBytes: 1024,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if !strings.Contains(out, "## Memory") || !strings.Contains(out, "remember this") {
		t.Fatalf("expected memory section, got: %q", out)
	}
}

func TestPromptBuilder_OmitsWhenEmpty(t *testing.T) {
	t.Parallel()

	constructor := &PromptBuilder{
		FS:             vfs.NewFS(),
		MaxMemoryBytes: 1024,
	}

	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if strings.Contains(out, "## User Profile") || strings.Contains(out, "## Memory") {
		t.Fatalf("did not expect memory section, got: %q", out)
	}
}

func TestPromptBuilder_IncludesSkillScriptsManifest(t *testing.T) {
	t.Parallel()

	fs := vfs.NewFS()
	memDir := t.TempDir()
	skillsDir := t.TempDir()

	memRes, err := resources.NewDirResource(memDir, vfs.MountMemory)
	if err != nil {
		t.Fatalf("NewDirResource(memory): %v", err)
	}
	if err := fs.Mount(vfs.MountMemory, memRes); err != nil {
		t.Fatalf("mount memory: %v", err)
	}

	writeSkill := func(name string) {
		t.Helper()
		content := []byte("---\nname: " + name + "\ndescription: test\n---\n# Instructions\n")
		path := filepath.Join(skillsDir, name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("write skill: %v", err)
		}
	}
	writeScript := func(skill, script string) {
		t.Helper()
		path := filepath.Join(skillsDir, skill, "scripts", script)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir script dir: %v", err)
		}
		if err := os.WriteFile(path, []byte("ok\n"), 0o644); err != nil {
			t.Fatalf("write script: %v", err)
		}
	}

	writeSkill("market-monitoring")
	writeSkill("data-engineering")
	writeScript("market-monitoring", "price_check.sh")
	writeScript("data-engineering", "csv_validate.py")
	writeScript("data-engineering", "db_connect.py")

	mgr := skills.NewManager([]string{skillsDir})
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan skills: %v", err)
	}

	constructor := &PromptBuilder{
		FS:             fs,
		Skills:         mgr,
		MaxMemoryBytes: 1024,
	}
	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if !strings.Contains(out, "<skill_scripts>") {
		t.Fatalf("expected skill_scripts block, got: %q", out)
	}
	if !strings.Contains(out, "data-engineering: csv_validate.py, db_connect.py") {
		t.Fatalf("expected data-engineering scripts, got: %q", out)
	}
	if !strings.Contains(out, "market-monitoring: price_check.sh") {
		t.Fatalf("expected market-monitoring scripts, got: %q", out)
	}
}

func TestPromptBuilder_OmitsEmptySkillScriptsManifest(t *testing.T) {
	t.Parallel()

	fs := vfs.NewFS()
	memDir := t.TempDir()
	skillsDir := t.TempDir()

	memRes, err := resources.NewDirResource(memDir, vfs.MountMemory)
	if err != nil {
		t.Fatalf("NewDirResource(memory): %v", err)
	}
	if err := fs.Mount(vfs.MountMemory, memRes); err != nil {
		t.Fatalf("mount memory: %v", err)
	}

	skillPath := filepath.Join(skillsDir, "reporting", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(skillPath, []byte("---\nname: reporting\n---\n# Instructions\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	mgr := skills.NewManager([]string{skillsDir})
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan skills: %v", err)
	}

	constructor := &PromptBuilder{
		FS:             fs,
		Skills:         mgr,
		MaxMemoryBytes: 1024,
	}
	out, err := constructor.SystemPrompt(context.Background(), "base", 1)
	if err != nil {
		t.Fatalf("SystemPrompt: %v", err)
	}
	if strings.Contains(out, "<skill_scripts>") {
		t.Fatalf("did not expect skill_scripts block, got: %q", out)
	}
}
