package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestBuiltinShell_Exec_CatFile_OK(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	run, err := store.CreateRun(cfg, "builtin shell ok test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "hello.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.shell"): tools.NewBuiltinShellInvoker(rootDir, nil, ""),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.shell"), "exec", json.RawMessage(`{"argv":["cat","hello.txt"],"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got err=%+v", resp.Error)
	}

	var out struct {
		ExitCode int    `json:"exitCode"`
		Stdout   string `json:"stdout"`
		Stderr   string `json:"stderr"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	if out.ExitCode != 0 {
		t.Fatalf("expected exitCode=0, got %d", out.ExitCode)
	}
	if strings.TrimSpace(out.Stdout) != "hello" {
		t.Fatalf("unexpected stdout: %q", out.Stdout)
	}
	if out.Stderr != "" {
		t.Fatalf("expected empty stderr, got %q", out.Stderr)
	}

	if _, err := fs.Read("/results/" + resp.CallID + "/response.json"); err != nil {
		t.Fatalf("expected persisted response.json: %v", err)
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestBuiltinShell_Exec_RejectsEscapeCwd(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	run, err := store.CreateRun(cfg, "builtin shell cwd test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.shell"): tools.NewBuiltinShellInvoker(t.TempDir(), nil, ""),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.shell"), "exec", json.RawMessage(`{"argv":["ls"],"cwd":"../"}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false, got %+v", resp)
	}
	if resp.Error == nil || resp.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input error, got %+v", resp.Error)
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestBuiltinShell_Exec_TruncatesAndWritesStdoutArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	run, err := store.CreateRun(cfg, "builtin bash truncation test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	rootDir := t.TempDir()
	// Slightly over 64KB.
	full := []byte(strings.Repeat("a", 70*1024))
	if err := os.WriteFile(filepath.Join(rootDir, "big.txt"), full, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	inv := tools.NewBuiltinShellInvoker(rootDir, nil, "")
	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.shell"): inv,
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.shell"), "exec", json.RawMessage(`{"argv":["cat","big.txt"],"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got err=%+v", resp.Error)
	}

	var out struct {
		Stdout     string `json:"stdout"`
		StdoutPath string `json:"stdoutPath"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	if out.StdoutPath != "stdout.txt" {
		t.Fatalf("expected stdoutPath=stdout.txt, got %q", out.StdoutPath)
	}
	if len(out.Stdout) != 64*1024 {
		t.Fatalf("expected truncated stdout length 65536, got %d", len(out.Stdout))
	}

	b, err := fs.Read("/results/" + resp.CallID + "/stdout.txt")
	if err != nil {
		t.Fatalf("Read stdout artifact: %v", err)
	}
	if string(b) != string(full) {
		t.Fatalf("stdout artifact mismatch")
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestBuiltinShell_Exec_RejectsAbsolutePathArgs(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, err := store.CreateRun(cfg, "builtin bash abs path args test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.shell"): tools.NewBuiltinShellInvoker(t.TempDir(), nil, ""),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.shell"), "exec", json.RawMessage(`{"argv":["cat","/etc/hosts"],"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false, got %+v", resp)
	}
	if resp.Error == nil || resp.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input error, got %+v", resp.Error)
	}
}

func TestBuiltinShell_EnvFiltersSensitiveVars(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env filtering test is unix-specific")
	}
	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	rootDir := t.TempDir()

	t.Setenv("AWS_SECRET_ACCESS_KEY", "supersecret")
	t.Setenv("GIT_DIR", "/tmp/secret")
	t.Setenv("SAFE_KEY", "keepme")

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.shell"): tools.NewBuiltinShellInvoker(rootDir, nil, ""),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.shell"), "exec", json.RawMessage(`{"argv":["env"],"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got err=%+v", resp.Error)
	}

	var out struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	if strings.Contains(out.Stdout, "AWS_SECRET_ACCESS_KEY") {
		t.Fatalf("stdout should not include filtered env key")
	}
	if !strings.Contains(out.Stdout, "SAFE_KEY=keepme") {
		t.Fatalf("stdout should retain non-sensitive env vars")
	}
}

func TestDefaultShellDenylist_BlocksHighRiskCommands(t *testing.T) {
	deny := tools.DefaultShellDenylist()
	for _, name := range []string{
		"bash",
		"sh",
		"python3",
		"node",
		"sudo",
		"ssh",
		"scp",
		"nc",
		"curl",
		"wget",
		"apt-get",
		"npm",
		"chmod",
		"launchctl",
		"rm",
		"dd",
	} {
		if !deny[name] {
			t.Fatalf("expected %q to be denied", name)
		}
	}
	for _, name := range []string{
		"ls",
		"cat",
		"rg",
		"ping",
		"dig",
		"ps",
		"df",
		"uname",
	} {
		if deny[name] {
			t.Fatalf("expected %q to NOT be denied", name)
		}
	}
}

func TestBuiltinShell_Exec_AllowsVFSArguments(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "foo.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	inv := tools.NewBuiltinShellInvoker(rootDir, nil, vfs.MountProject)
	input, err := json.Marshal(struct {
		Argv []string `json:"argv"`
		Cwd  string   `json:"cwd"`
	}{
		Argv: []string{"ls", "/" + vfs.MountProject},
		Cwd:  ".",
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	req := types.ToolRequest{
		Version:  "v1",
		CallID:   "vfs-arg",
		ToolID:   types.ToolID("builtin.shell"),
		ActionID: "exec",
		Input:    input,
	}
	resp, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out struct {
		Stdout string `json:"stdout"`
		Exit   int    `json:"exitCode"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	if out.Exit != 0 {
		t.Fatalf("exitCode=%d", out.Exit)
	}
	if !strings.Contains(out.Stdout, "foo.txt") {
		t.Fatalf("stdout=%q; want it to list foo.txt", out.Stdout)
	}
}

func TestBuiltinShell_Exec_TranslatesOutputPathsToVFS(t *testing.T) {
	rootDir := t.TempDir()
	inv := tools.NewBuiltinShellInvoker(rootDir, nil, vfs.MountProject)
	input, err := json.Marshal(struct {
		Argv []string `json:"argv"`
	}{
		Argv: []string{"pwd"},
	})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	req := types.ToolRequest{
		Version:  "v1",
		CallID:   "vfs-output",
		ToolID:   types.ToolID("builtin.shell"),
		ActionID: "exec",
		Input:    input,
	}
	resp, err := inv.Invoke(context.Background(), req)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var out struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	stdout := strings.TrimSpace(out.Stdout)
	if !strings.Contains(stdout, "/"+vfs.MountProject) {
		t.Fatalf("stdout=%q; want it to mention /%s", stdout, vfs.MountProject)
	}
	if strings.Contains(stdout, rootDir) {
		t.Fatalf("stdout=%q; should not expose host path", stdout)
	}
}
