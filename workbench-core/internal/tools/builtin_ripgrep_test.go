package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestBuiltinRipgrep_Search_FindsMatch(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed; skipping")
	}

	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	run, err := store.CreateRun(cfg, "builtin ripgrep test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	rootDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rootDir, "a.txt"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.ripgrep"): tools.NewBuiltinRipgrepInvoker(rootDir),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.ripgrep"), "search", json.RawMessage(`{"query":"hello","paths":["."]}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}

	var out struct {
		Matches []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
			Text string `json:"text"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	if len(out.Matches) == 0 {
		t.Fatalf("expected at least one match")
	}
	if out.Matches[0].Path == "" || out.Matches[0].Line == 0 {
		t.Fatalf("unexpected match: %+v", out.Matches[0])
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestBuiltinRipgrep_Search_RejectsEscapePath(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("rg not installed; skipping")
	}

	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	run, err := store.CreateRun(cfg, "builtin ripgrep escape test", 100)
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
			types.ToolID("builtin.ripgrep"): tools.NewBuiltinRipgrepInvoker(t.TempDir()),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.ripgrep"), "search", json.RawMessage(`{"query":"x","paths":["../"]}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false, got %+v", resp)
	}
	if resp.Error == nil || resp.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input, got %+v", resp.Error)
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}
