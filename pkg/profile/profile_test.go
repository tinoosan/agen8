package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if len(p.Heartbeat.Jobs) != 1 || p.Heartbeat.Jobs[0].Name != "ping" {
		t.Fatalf("unexpected heartbeat: %+v", p.Heartbeat)
	}
}

func TestLoad_HeartbeatHourInterval(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte("id: test\ndescription: x\nprompts:\n  system_prompt_path: prompt.md\nheartbeat:\n  - name: ping\n    interval: 1h\n    goal: hello\n"), 0o644); err != nil {
		t.Fatalf("write profile.yaml: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := p.Heartbeat.Jobs[0].Interval, time.Hour; got != want {
		t.Fatalf("heartbeat interval = %v want %v", got, want)
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

func TestLoad_TeamProfile_AllowSubagents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	raw := `
id: team-allow
description: Team with allow_subagents
team:
  roles:
    - name: lead
      coordinator: true
      description: Lead
      prompts:
        system_prompt_path: prompt.md
      allow_subagents: true
    - name: worker
      description: Worker
      prompts:
        system_prompt_path: prompt.md
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	lead := p.Team.Roles[0]
	if !lead.AllowSubagents {
		t.Fatalf("lead.AllowSubagents = false, want true")
	}
	worker := p.Team.Roles[1]
	if worker.AllowSubagents {
		t.Fatalf("worker.AllowSubagents = true, want false (default)")
	}
}

func TestProfile_RolesForSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(
		"id: solo\ndescription: Solo\nmodel: gpt-5\nprompts:\n  system_prompt_path: prompt.md\n",
	), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	roles, err := p.RolesForSession()
	if err != nil {
		t.Fatalf("RolesForSession: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("standalone: expected 1 role, got %d", len(roles))
	}
	if roles[0].Name != "agent" || !roles[0].Coordinator || !roles[0].AllowSubagents {
		t.Fatalf("synthetic role: %+v", roles[0])
	}
	if roles[0].Model != "gpt-5" {
		t.Fatalf("role model = %q want gpt-5", roles[0].Model)
	}
}

func TestProfile_RolesForSession_WithName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(
		"id: researcher\nname: Stock Researcher\ndescription: Research\nmodel: gpt-5\nprompts:\n  system_prompt_path: prompt.md\n",
	), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	roles, err := p.RolesForSession()
	if err != nil {
		t.Fatalf("RolesForSession: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("standalone: expected 1 role, got %d", len(roles))
	}
	if roles[0].Name != "Stock Researcher" {
		t.Fatalf("synthetic role name = %q want Stock Researcher", roles[0].Name)
	}
}

func TestResolveByRef(t *testing.T) {
	profilesDir := t.TempDir()
	generalDir := filepath.Join(profilesDir, "general")
	if err := os.MkdirAll(generalDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(generalDir, "profile.yaml"), []byte(
		"id: general\ndescription: General\nmodel: gpt-5\nprompts:\n  system_prompt: hi\n",
	), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	// By name under profilesDir
	p, dir, err := ResolveByRef(profilesDir, "general")
	if err != nil {
		t.Fatalf("ResolveByRef(general): %v", err)
	}
	if p == nil || p.ID != "general" {
		t.Fatalf("profile = %+v", p)
	}
	if dir != generalDir {
		t.Fatalf("dir = %q want %q", dir, generalDir)
	}

	// Empty defaults to general
	p2, _, err := ResolveByRef(profilesDir, "")
	if err != nil {
		t.Fatalf("ResolveByRef(empty): %v", err)
	}
	if p2 == nil || p2.ID != "general" {
		t.Fatalf("profile = %+v", p2)
	}

	// Direct path
	p3, dir3, err := ResolveByRef(profilesDir, generalDir)
	if err != nil {
		t.Fatalf("ResolveByRef(path): %v", err)
	}
	if p3 == nil || dir3 != generalDir {
		t.Fatalf("profile=%+v dir=%q", p3, dir3)
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

func TestLoad_TeamProfile_RejectsMultipleReviewers(t *testing.T) {
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
      reviewer: true
      description: Lead
      prompts:
        system_prompt_path: prompt.md
    - name: qa
      reviewer: true
      description: QA
      prompts:
        system_prompt_path: prompt.md
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected multiple reviewer validation error")
	}
}

func TestLoad_NormalizesAllowedTools(t *testing.T) {
	dir := t.TempDir()
	raw := `
id: tools-test
description: Tools profile
prompts:
  system_prompt: hi
allowed_tools: [fs_read, " fs_read ", shell_exec]
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := len(p.AllowedTools), 2; got != want {
		t.Fatalf("allowed_tools len=%d want=%d (%v)", got, want, p.AllowedTools)
	}
	if p.AllowedTools[0] != "fs_read" || p.AllowedTools[1] != "shell_exec" {
		t.Fatalf("unexpected allowed_tools order/content: %v", p.AllowedTools)
	}
}

func TestLoad_ParsesCodeExecOnly(t *testing.T) {
	dir := t.TempDir()
	raw := `
id: code-exec-only
description: Code exec profile
code_exec_only: true
prompts:
  system_prompt: hi
allowed_tools: [fs_list, fs_read]
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !p.CodeExecOnly {
		t.Fatalf("expected code_exec_only to be true")
	}
}

func TestLoad_NormalizesCodeExecRequiredImports(t *testing.T) {
	dir := t.TempDir()
	raw := `
id: code-exec-imports
description: Code exec imports
prompts:
  system_prompt: hi
code_exec_required_imports: [requests, " requests ", pandas]
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := len(p.CodeExecRequiredImports), 2; got != want {
		t.Fatalf("code_exec_required_imports len=%d want=%d (%v)", got, want, p.CodeExecRequiredImports)
	}
	if p.CodeExecRequiredImports[0] != "requests" || p.CodeExecRequiredImports[1] != "pandas" {
		t.Fatalf("unexpected code_exec_required_imports order/content: %v", p.CodeExecRequiredImports)
	}
}

func TestLoad_TeamRoleCodeExecOnlyOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coord.md"), []byte("coord"), 0o644); err != nil {
		t.Fatalf("write coord prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.md"), []byte("worker"), 0o644); err != nil {
		t.Fatalf("write worker prompt: %v", err)
	}
	raw := `
id: team-code-exec
description: Team profile
code_exec_only: true
team:
  roles:
    - name: lead
      coordinator: true
      description: Team lead
      prompts:
        system_prompt_path: coord.md
    - name: worker
      description: Team worker
      code_exec_only: false
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
	if !p.CodeExecOnly {
		t.Fatalf("expected profile code_exec_only true")
	}
	if p.Team == nil || len(p.Team.Roles) != 2 {
		t.Fatalf("unexpected roles: %+v", p.Team)
	}
	if p.Team.Roles[0].CodeExecOnly != nil {
		t.Fatalf("expected nil override for first role")
	}
	if p.Team.Roles[1].CodeExecOnly == nil || *p.Team.Roles[1].CodeExecOnly {
		t.Fatalf("expected worker override false")
	}
}

func TestEffectiveHeartbeats_Disabled(t *testing.T) {
	disabled := false
	p := &Profile{
		Heartbeat: HeartbeatConfig{
			Enabled: &disabled,
			Jobs:    []HeartbeatJob{{Name: "ping", Interval: time.Minute, Goal: "hi"}},
		},
	}
	if got := p.EffectiveHeartbeats(); len(got) != 0 {
		t.Fatalf("heartbeat_enabled=false: expected no heartbeats, got %d", len(got))
	}
}

func TestEffectiveHeartbeats_Enabled(t *testing.T) {
	enabled := true
	p := &Profile{
		Heartbeat: HeartbeatConfig{
			Enabled: &enabled,
			Jobs:    []HeartbeatJob{{Name: "ping", Interval: time.Minute, Goal: "hi"}},
		},
	}
	if got := p.EffectiveHeartbeats(); len(got) != 1 {
		t.Fatalf("heartbeat_enabled=true: expected 1 heartbeat, got %d", len(got))
	}
}

func TestEffectiveHeartbeats_UnsetDefaultsToEnabled(t *testing.T) {
	p := &Profile{
		Heartbeat: HeartbeatConfig{
			Jobs: []HeartbeatJob{{Name: "ping", Interval: time.Minute, Goal: "hi"}},
		},
	}
	if got := p.EffectiveHeartbeats(); len(got) != 1 {
		t.Fatalf("heartbeat_enabled unset: expected 1 heartbeat (default on), got %d", len(got))
	}
}

func TestLoad_HeartbeatEnabledFalse(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("# hi\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	raw := `
id: test
description: x
prompts:
  system_prompt_path: prompt.md
heartbeat:
  enabled: false
  jobs:
    - name: ping
      interval: 1m
      goal: hello
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(p.Heartbeat.Jobs) != 1 {
		t.Fatalf("expected heartbeat entries preserved: %+v", p.Heartbeat)
	}
	if got := p.EffectiveHeartbeats(); len(got) != 0 {
		t.Fatalf("heartbeat_enabled=false: expected EffectiveHeartbeats empty, got %d", len(got))
	}
}

func TestLoad_TeamRoleCodeExecRequiredImports(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coord.md"), []byte("coord"), 0o644); err != nil {
		t.Fatalf("write coord prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.md"), []byte("worker"), 0o644); err != nil {
		t.Fatalf("write worker prompt: %v", err)
	}
	raw := `
id: team-code-exec-imports
description: Team profile
code_exec_required_imports: [requests, " requests "]
team:
  roles:
    - name: lead
      coordinator: true
      description: Team lead
      prompts:
        system_prompt_path: coord.md
    - name: worker
      description: Team worker
      code_exec_required_imports: [pandas, " pandas "]
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
	if got, want := len(p.CodeExecRequiredImports), 1; got != want {
		t.Fatalf("profile code_exec_required_imports len=%d want=%d (%v)", got, want, p.CodeExecRequiredImports)
	}
	if p.CodeExecRequiredImports[0] != "requests" {
		t.Fatalf("unexpected profile code_exec_required_imports: %v", p.CodeExecRequiredImports)
	}
	if p.Team == nil || len(p.Team.Roles) != 2 {
		t.Fatalf("unexpected roles: %+v", p.Team)
	}
	if len(p.Team.Roles[1].CodeExecRequiredImports) != 1 || p.Team.Roles[1].CodeExecRequiredImports[0] != "pandas" {
		t.Fatalf("unexpected role code_exec_required_imports: %v", p.Team.Roles[1].CodeExecRequiredImports)
	}
}
