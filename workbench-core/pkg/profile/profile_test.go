package profile

import (
	"os"
	"path/filepath"
	"strings"
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

func TestLoad_TeamProfile_Valid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coord.md"), []byte("coord prompt"), 0o644); err != nil {
		t.Fatalf("write coord prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.md"), []byte("worker prompt"), 0o644); err != nil {
		t.Fatalf("write worker prompt: %v", err)
	}
	raw := `
id: team-test
description: Team profile
team:
  roles:
    - name: lead
      coordinator: true
      description: Team lead
      prompts:
        system_prompt_path: coord.md
    - name: worker
      description: Team worker
      prompts:
        system_prompt_path: worker.md
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Team == nil {
		t.Fatalf("expected team config")
	}
	if len(p.Team.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(p.Team.Roles))
	}
}

func TestLoad_TeamProfile_MissingCoordinator(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	raw := `
id: team-test
description: Team profile
team:
  roles:
    - name: worker-a
      description: Worker A
      prompts:
        system_prompt_path: prompt.md
    - name: worker-b
      description: Worker B
      prompts:
        system_prompt_path: prompt.md
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected missing coordinator error")
	}
}

func TestLoad_TeamProfile_DuplicateRoleNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	raw := `
id: team-test
description: Team profile
team:
  roles:
    - name: lead
      coordinator: true
      description: Lead
      prompts:
        system_prompt_path: prompt.md
    - name: lead
      description: Duplicate lead
      prompts:
        system_prompt_path: prompt.md
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected duplicate role validation error")
	}
}
