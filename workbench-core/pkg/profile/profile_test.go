package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte("id: test\ndescription: x\nprompts:\n  system_prompt_path: prompt.md\nskills: [coding]\nheartbeat:\n  - name: ping\n    interval: 1m\n    goal: hello\n"), 0o644); err != nil {
		t.Fatalf("write profile.yaml: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.ID != "test" {
		t.Fatalf("unexpected id: %q", p.ID)
	}
	if len(p.Heartbeat) != 1 || p.Heartbeat[0].Name != "ping" {
		t.Fatalf("unexpected heartbeat: %+v", p.Heartbeat)
	}
}

func TestLoad_DefaultsPromptMD(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte("id: test\ndescription: x\nprompts: {}\n"), 0o644); err != nil {
		t.Fatalf("write profile.yaml: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Prompts.SystemPromptPath != "prompt.md" {
		t.Fatalf("expected prompt.md default, got %q", p.Prompts.SystemPromptPath)
	}
}

func TestLoad_Invalid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte("id: \ndescription: x\nprompts:\n  system_prompt: hi\n"), 0o644); err != nil {
		t.Fatalf("write profile.yaml: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected error")
	}
}

