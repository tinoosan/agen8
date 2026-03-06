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
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte("id: test\ndescription: x\nprompts:\n  systemPromptPath: prompt.md\nskills: [coding]\nheartbeat:\n  - name: ping\n    interval: 1m\n    goal: hello\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte("id: test\ndescription: x\nprompts:\n  systemPromptPath: prompt.md\nheartbeat:\n  - name: ping\n    interval: 1h\n    goal: hello\n"), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte("id: \ndescription: x\nprompts:\n  systemPrompt: hi\n"), 0o644); err != nil {
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
        systemPromptPath: coord.md
    - name: worker
      description: Team worker
      prompts:
        systemPromptPath: worker.md
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
        systemPromptPath: prompt.md
      allowSubagents: true
    - name: worker
      description: Worker
      prompts:
        systemPromptPath: prompt.md
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
		"id: solo\ndescription: Solo\nmodel: gpt-5\nprompts:\n  systemPromptPath: prompt.md\n",
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
		"id: researcher\nname: Stock Researcher\ndescription: Research\nmodel: gpt-5\nprompts:\n  systemPromptPath: prompt.md\n",
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
		"id: general\ndescription: General\nmodel: gpt-5\nprompts:\n  systemPrompt: hi\n",
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
        systemPromptPath: prompt.md
    - name: worker-b
      description: Worker B
      prompts:
        systemPromptPath: prompt.md
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
        systemPromptPath: prompt.md
    - name: lead
      description: Duplicate lead
      prompts:
        systemPromptPath: prompt.md
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected duplicate role validation error")
	}
}

func TestLoad_TeamProfile_RejectsReviewerNameCollision(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	raw := `
id: team-test
description: Team profile
team:
  reviewer:
    enabled: true
    name: lead
    description: Reviewer
    prompts:
      systemPromptPath: prompt.md
  roles:
    - name: lead
      coordinator: true
      description: Lead
      prompts:
        systemPromptPath: prompt.md
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected reviewer name collision validation error")
	}
}

func TestLoad_NormalizesAllowedTools(t *testing.T) {
	dir := t.TempDir()
	raw := `
id: tools-test
description: Tools profile
prompts:
  systemPrompt: hi
allowedTools: [fs_read, " fs_read ", shell_exec]
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
codeExecOnly: true
prompts:
  systemPrompt: hi
allowedTools: [fs_list, fs_read]
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
  systemPrompt: hi
codeExecRequiredImports: [requests, " requests ", pandas]
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
codeExecOnly: true
team:
  roles:
    - name: lead
      coordinator: true
      description: Team lead
      prompts:
        systemPromptPath: coord.md
    - name: worker
      description: Team worker
      codeExecOnly: false
      prompts:
        systemPromptPath: worker.md
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
  systemPromptPath: prompt.md
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
codeExecRequiredImports: [requests, " requests "]
team:
  roles:
    - name: lead
      coordinator: true
      description: Team lead
      prompts:
        systemPromptPath: coord.md
    - name: worker
      description: Team worker
      codeExecRequiredImports: [pandas, " pandas "]
      prompts:
        systemPromptPath: worker.md
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
		t.Fatalf("unexpected profile codeExecRequiredImports: %v", p.CodeExecRequiredImports)
	}
	if p.Team == nil || len(p.Team.Roles) != 2 {
		t.Fatalf("unexpected roles: %+v", p.Team)
	}
	if len(p.Team.Roles[1].CodeExecRequiredImports) != 1 || p.Team.Roles[1].CodeExecRequiredImports[0] != "pandas" {
		t.Fatalf("unexpected role codeExecRequiredImports: %v", p.Team.Roles[1].CodeExecRequiredImports)
	}
}

func TestLoad_PromptFragments(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.md"), []byte("Base prompt."), 0o644); err != nil {
		t.Fatalf("write base.md: %v", err)
	}
	raw := `
id: frag-test
description: Fragment test
prompts:
  systemFragments:
    - path: base.md
    - inline: "Focus on data quality."
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(p.Prompts.SystemFragments) != 2 {
		t.Fatalf("expected 2 fragments, got %d", len(p.Prompts.SystemFragments))
	}
	if p.Prompts.SystemFragments[0].Path != "base.md" {
		t.Fatalf("fragment[0].Path = %q", p.Prompts.SystemFragments[0].Path)
	}
	if p.Prompts.SystemFragments[1].Inline != "Focus on data quality." {
		t.Fatalf("fragment[1].Inline = %q", p.Prompts.SystemFragments[1].Inline)
	}

	// ResolveFragments should concatenate
	text, err := ResolveFragments(dir, p.Prompts)
	if err != nil {
		t.Fatalf("ResolveFragments: %v", err)
	}
	if text != "Base prompt.\n\nFocus on data quality." {
		t.Fatalf("unexpected resolved text: %q", text)
	}
}

func TestLoad_PromptFragments_RejectsMixed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "base.md"), []byte("Base."), 0o644); err != nil {
		t.Fatalf("write base.md: %v", err)
	}
	raw := `
id: frag-mixed
description: Mixed legacy and fragments
prompts:
  systemPromptPath: base.md
  systemFragments:
    - inline: "Extra."
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatalf("expected error for mixing legacy and fragments")
	}
}

func TestEffectiveFragments(t *testing.T) {
	// Legacy systemPrompt → single inline fragment
	pc := PromptConfig{SystemPrompt: "hello"}
	frags := pc.EffectiveFragments()
	if len(frags) != 1 || frags[0].Inline != "hello" {
		t.Fatalf("legacy systemPrompt: %+v", frags)
	}

	// Legacy systemPromptPath → single path fragment
	pc2 := PromptConfig{SystemPromptPath: "prompt.md"}
	frags2 := pc2.EffectiveFragments()
	if len(frags2) != 1 || frags2[0].Path != "prompt.md" {
		t.Fatalf("legacy systemPromptPath: %+v", frags2)
	}

	// Explicit fragments take priority
	pc3 := PromptConfig{SystemFragments: []PromptFragment{{Inline: "a"}, {Path: "b.md"}}}
	frags3 := pc3.EffectiveFragments()
	if len(frags3) != 2 {
		t.Fatalf("explicit fragments: %+v", frags3)
	}

	// Empty → nil
	pc4 := PromptConfig{}
	if frags4 := pc4.EffectiveFragments(); frags4 != nil {
		t.Fatalf("empty: %+v", frags4)
	}
}

func TestLoad_PromptFragments_RelativeTraversal(t *testing.T) {
	// Create parent/shared/safety.md and parent/profile/profile.yaml
	parent := t.TempDir()
	shared := filepath.Join(parent, "shared")
	profDir := filepath.Join(parent, "profile")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatalf("mkdir shared: %v", err)
	}
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatalf("mkdir profDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(shared, "safety.md"), []byte("Safety rules."), 0o644); err != nil {
		t.Fatalf("write safety.md: %v", err)
	}
	raw := `
id: traversal-test
description: Test relative traversal
prompts:
  systemFragments:
    - path: ../shared/safety.md
    - inline: "Local context."
`
	if err := os.WriteFile(filepath.Join(profDir, "profile.yaml"), []byte(strings.TrimSpace(raw)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(profDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	text, err := ResolveFragments(profDir, p.Prompts)
	if err != nil {
		t.Fatalf("ResolveFragments: %v", err)
	}
	if text != "Safety rules.\n\nLocal context." {
		t.Fatalf("unexpected resolved text: %q", text)
	}
}

func TestLoad_BackwardCompat(t *testing.T) {
	// Ensure existing profiles with legacy systemPromptPath still load
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "prompt.md"), []byte("Legacy prompt."), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "profile.yaml"), []byte(
		"id: legacy\ndescription: Legacy\nprompts:\n  systemPromptPath: prompt.md\n",
	), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Prompts.SystemPromptPath != "prompt.md" {
		t.Fatalf("expected systemPromptPath=prompt.md, got %q", p.Prompts.SystemPromptPath)
	}
	text, err := ResolveFragments(dir, p.Prompts)
	if err != nil {
		t.Fatalf("ResolveFragments: %v", err)
	}
	if text != "Legacy prompt." {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestLoad_RoleRef_Resolves(t *testing.T) {
	parent := t.TempDir()
	baseDir := filepath.Join(parent, "base", "roles")
	profDir := filepath.Join(parent, "myprofile")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatalf("mkdir profile: %v", err)
	}
	// Base role with prompt path relative to its own directory
	basePromptDir := filepath.Join(parent, "base", "prompts")
	if err := os.MkdirAll(basePromptDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(basePromptDir, "worker.md"), []byte("Worker prompt."), 0o644); err != nil {
		t.Fatalf("write worker.md: %v", err)
	}

	baseRole := `codeExecOnly: true
allowedTools: [pipe, http_fetch, browser]
prompts:
  systemFragments:
    - path: ../prompts/worker.md
`
	if err := os.WriteFile(filepath.Join(baseDir, "worker-web.yaml"), []byte(baseRole), 0o644); err != nil {
		t.Fatalf("write base role: %v", err)
	}

	// Coordinator prompt
	if err := os.WriteFile(filepath.Join(profDir, "coord.md"), []byte("Coord prompt."), 0o644); err != nil {
		t.Fatalf("write coord.md: %v", err)
	}

	profileYAML := `
id: roleref-test
description: Test roleRef resolution
team:
  roles:
    - name: lead
      coordinator: true
      description: Team lead
      prompts:
        systemPromptPath: coord.md
    - roleRef: ../base/roles/worker-web.yaml
      name: researcher
      description: Researches stuff
      skills: [planning]
`
	if err := os.WriteFile(filepath.Join(profDir, "profile.yaml"), []byte(strings.TrimSpace(profileYAML)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(profDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(p.Team.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(p.Team.Roles))
	}
	worker := p.Team.Roles[1]
	if worker.Name != "researcher" {
		t.Fatalf("role name = %q, want researcher", worker.Name)
	}
	if worker.Description != "Researches stuff" {
		t.Fatalf("role description = %q", worker.Description)
	}
	if worker.CodeExecOnly == nil || !*worker.CodeExecOnly {
		t.Fatalf("expected codeExecOnly from base")
	}
	if len(worker.AllowedTools) != 3 || worker.AllowedTools[0] != "pipe" {
		t.Fatalf("allowedTools = %v", worker.AllowedTools)
	}
	if len(worker.Skills) != 1 || worker.Skills[0] != "planning" {
		t.Fatalf("skills = %v (should be inline override)", worker.Skills)
	}
}

func TestLoad_RoleRef_Override(t *testing.T) {
	parent := t.TempDir()
	baseDir := filepath.Join(parent, "base")
	profDir := filepath.Join(parent, "profile")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(profDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	baseRole := `name: base-worker
description: Base worker
coordinator: true
subagentModel: moonshotai/kimi-k2.5
codeExecOnly: true
allowedTools: [pipe, http_fetch]
prompts:
  systemPrompt: base prompt
heartbeat:
  enabled: false
  jobs:
    - name: check-in
      interval: 10m
      goal: "Check in"
`
	if err := os.WriteFile(filepath.Join(baseDir, "coord.yaml"), []byte(baseRole), 0o644); err != nil {
		t.Fatalf("write base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profDir, "lead.md"), []byte("Lead prompt."), 0o644); err != nil {
		t.Fatalf("write lead.md: %v", err)
	}

	profileYAML := `
id: override-test
description: Override test
team:
  roles:
    - roleRef: ../base/coord.yaml
      name: lead
      description: Team lead
      model: gpt-5-mini
      prompts:
        systemPromptPath: lead.md
`
	if err := os.WriteFile(filepath.Join(profDir, "profile.yaml"), []byte(strings.TrimSpace(profileYAML)+"\n"), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	p, err := Load(profDir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	role := p.Team.Roles[0]
	// Inline overrides
	if role.Name != "lead" {
		t.Fatalf("name = %q", role.Name)
	}
	if role.Description != "Team lead" {
		t.Fatalf("description = %q", role.Description)
	}
	if role.Model != "gpt-5-mini" {
		t.Fatalf("model = %q (should be override)", role.Model)
	}
	if role.Prompts.SystemPromptPath != "lead.md" {
		t.Fatalf("prompts = %+v (should be override)", role.Prompts)
	}
	// Inherited from base
	if !role.Coordinator {
		t.Fatalf("coordinator should be inherited from base")
	}
	if role.SubagentModel != "moonshotai/kimi-k2.5" {
		t.Fatalf("subagentModel = %q (should be inherited)", role.SubagentModel)
	}
	if role.CodeExecOnly == nil || !*role.CodeExecOnly {
		t.Fatalf("codeExecOnly should be inherited")
	}
}
