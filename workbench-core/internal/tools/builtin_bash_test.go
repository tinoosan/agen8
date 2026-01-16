package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestBuiltinBash_Exec_CatFile_OK(t *testing.T) {
	tmpDir := t.TempDir()
	oldDataDir := config.DataDir
	config.DataDir = tmpDir
	defer func() { config.DataDir = oldDataDir }()

	run, err := store.CreateRun("builtin bash ok test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewVirtualResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewVirtualResultsResource: %v", err)
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
			types.ToolID("builtin.bash"): tools.NewBuiltinBashInvoker(rootDir),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.bash"), "exec", json.RawMessage(`{"argv":["cat","hello.txt"],"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
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

func TestBuiltinBash_Exec_RejectsEscapeCwd(t *testing.T) {
	tmpDir := t.TempDir()
	oldDataDir := config.DataDir
	config.DataDir = tmpDir
	defer func() { config.DataDir = oldDataDir }()

	run, err := store.CreateRun("builtin bash cwd test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewVirtualResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewVirtualResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.bash"): tools.NewBuiltinBashInvoker(t.TempDir()),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.bash"), "exec", json.RawMessage(`{"argv":["ls"],"cwd":"../"}`), 0)
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

func TestBuiltinBash_Exec_TruncatesAndWritesStdoutArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	oldDataDir := config.DataDir
	config.DataDir = tmpDir
	defer func() { config.DataDir = oldDataDir }()

	run, err := store.CreateRun("builtin bash truncation test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewVirtualResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewVirtualResultsResource: %v", err)
	}

	rootDir := t.TempDir()
	// Slightly over 64KB.
	full := []byte(strings.Repeat("a", 70*1024))
	if err := os.WriteFile(filepath.Join(rootDir, "big.txt"), full, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	inv := tools.NewBuiltinBashInvoker(rootDir)
	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.bash"): inv,
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.bash"), "exec", json.RawMessage(`{"argv":["cat","big.txt"],"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
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

func TestDefaultBashAllowlist_IncludesITCommands(t *testing.T) {
	allow := tools.DefaultBashAllowlist()
	for _, name := range []string{
		"curl",
		"wget",
		"ping",
		"dig",
		"nslookup",
		"traceroute",
		"netstat",
		"lsof",
		"ps",
		"df",
		"uname",
	} {
		if !allow[name] {
			t.Fatalf("expected %q to be allowlisted", name)
		}
	}
}
