package builtins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
)

func TestBuiltinShellInvoker_AllowsSkillsMountCwd(t *testing.T) {
	projectDir := t.TempDir()
	skillsDir := t.TempDir()
	scriptDir := filepath.Join(skillsDir, "data_engineering", "scripts")
	if err := writeExecutable(scriptDir, "echo_json.sh", "#!/usr/bin/env bash\necho '{\"ok\":true}'\n"); err != nil {
		t.Fatalf("write script: %v", err)
	}

	inv := NewBuiltinShellInvoker(projectDir, nil, "project")
	inv.MountRoots["skills"] = skillsDir

	req := toolReq(t, shellExecInput{
		Argv: []string{"bash", "-c", "./echo_json.sh"},
		Cwd:  "/skills/data_engineering/scripts",
	})
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	var out shellExecOutput
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", out.ExitCode, out.Stderr)
	}
	if !strings.Contains(out.Stdout, `{"ok":true}`) {
		t.Fatalf("unexpected stdout: %q", out.Stdout)
	}
}

func TestBuiltinShellInvoker_AllowsWorkspaceMountCwd(t *testing.T) {
	projectDir := t.TempDir()
	workspaceDir := t.TempDir()

	inv := NewBuiltinShellInvoker(projectDir, nil, "project")
	inv.MountRoots["workspace"] = workspaceDir

	req := toolReq(t, shellExecInput{
		Argv: []string{"bash", "-c", "pwd"},
		Cwd:  "/workspace",
	})
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	var out shellExecOutput
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", out.ExitCode, out.Stderr)
	}
	if !strings.Contains(out.Stdout, "/workspace") {
		t.Fatalf("expected translated /workspace path in stdout, got %q", out.Stdout)
	}
}

func TestBuiltinShellInvoker_NormalizesDuplicateScriptsPrefix(t *testing.T) {
	projectDir := t.TempDir()
	skillsDir := t.TempDir()
	scriptDir := filepath.Join(skillsDir, "data_engineering", "scripts")
	if err := writeExecutable(scriptDir, "hello.py", "#!/usr/bin/env python3\nprint('ok')\n"); err != nil {
		t.Fatalf("write script: %v", err)
	}

	inv := NewBuiltinShellInvoker(projectDir, nil, "project")
	inv.MountRoots["skills"] = skillsDir

	req := toolReq(t, shellExecInput{
		Argv: []string{"bash", "-c", "python3 scripts/hello.py"},
		Cwd:  "/skills/data_engineering/scripts",
	})
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	var out shellExecOutput
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", out.ExitCode, out.Stderr)
	}
	if !out.ScriptPathNormalized {
		t.Fatalf("expected normalization to be true")
	}
	if out.ScriptAntiPattern != "duplicate_scripts_prefix" {
		t.Fatalf("unexpected anti pattern: %q", out.ScriptAntiPattern)
	}
}

func TestBuiltinShellInvoker_NormalizesAbsoluteSkillsScriptPath(t *testing.T) {
	projectDir := t.TempDir()
	skillsDir := t.TempDir()
	scriptDir := filepath.Join(skillsDir, "data_engineering", "scripts")
	if err := writeExecutable(scriptDir, "hello.py", "#!/usr/bin/env python3\nprint('ok')\n"); err != nil {
		t.Fatalf("write script: %v", err)
	}

	inv := NewBuiltinShellInvoker(projectDir, nil, "project")
	inv.MountRoots["skills"] = skillsDir

	req := toolReq(t, shellExecInput{
		Argv: []string{"bash", "-c", "python3 /skills/data_engineering/scripts/hello.py"},
		Cwd:  "/skills/data_engineering/scripts",
	})
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	var out shellExecOutput
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", out.ExitCode, out.Stderr)
	}
	if !out.ScriptPathNormalized {
		t.Fatalf("expected normalization to be true")
	}
	if out.ScriptAntiPattern != "absolute_skills_path" {
		t.Fatalf("unexpected anti pattern: %q", out.ScriptAntiPattern)
	}
}

func TestBuiltinShellInvoker_ProvidesScriptHintOnFailure(t *testing.T) {
	projectDir := t.TempDir()
	skillsDir := t.TempDir()
	scriptDir := filepath.Join(skillsDir, "data_engineering", "scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("mkdir script dir: %v", err)
	}

	inv := NewBuiltinShellInvoker(projectDir, nil, "project")
	inv.MountRoots["skills"] = skillsDir
	inv.EnableScriptPathMitigation = false

	req := toolReq(t, shellExecInput{
		Argv: []string{"bash", "-c", "python3 scripts/missing.py"},
		Cwd:  "/skills/data_engineering/scripts",
	})
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	var out shellExecOutput
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ExitCode == 0 {
		t.Fatalf("expected failure for missing script")
	}
	if !strings.Contains(out.Stderr, "script_hint:") {
		t.Fatalf("expected script hint in stderr, got %q", out.Stderr)
	}
}

func TestBuiltinShellInvoker_RetriesOnceAfterFailureNormalization(t *testing.T) {
	projectDir := t.TempDir()
	skillsDir := t.TempDir()
	scriptDir := filepath.Join(skillsDir, "data_engineering", "scripts")
	if err := writeExecutable(scriptDir, "hello.py", "#!/usr/bin/env python3\nprint('ok')\n"); err != nil {
		t.Fatalf("write script: %v", err)
	}

	inv := NewBuiltinShellInvoker(projectDir, nil, "project")
	inv.MountRoots["skills"] = skillsDir
	inv.MaxScriptMitigationRetries = 1

	// Intentionally wrong skill path in command; retry should normalize from failure diagnostics.
	req := toolReq(t, shellExecInput{
		Argv: []string{"bash", "-c", "python3 /skills/other/scripts/hello.py"},
		Cwd:  "/skills/data_engineering/scripts",
	})
	res, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}

	var out shellExecOutput
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("expected retry success, got %d stderr=%q", out.ExitCode, out.Stderr)
	}
	if !out.ScriptPathNormalized || out.ScriptAntiPattern != "absolute_skills_path" {
		t.Fatalf("expected retry normalization metadata, got normalized=%t anti=%q", out.ScriptPathNormalized, out.ScriptAntiPattern)
	}
}

func toolReq(t *testing.T, in shellExecInput) pkgtools.ToolRequest {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	return pkgtools.ToolRequest{
		Version:  "v1",
		CallID:   "call-1",
		ToolID:   pkgtools.ToolID("builtin.shell"),
		ActionID: "exec",
		Input:    raw,
	}
}

func writeExecutable(dir, name, body string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	return os.WriteFile(path, []byte(body), 0o755)
}
