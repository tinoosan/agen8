package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
)

func TestBuiltinFind_Files_MatchesBasenameAndPathAndDoubleStar(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}
	_, err := store.CreateRun(cfg, "builtin find test", 100)
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	_, err = resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	rootDir := t.TempDir()
	mkdirAll(t, filepath.Join(rootDir, "dir", "sub"))
	mkdirAll(t, filepath.Join(rootDir, ".git"))
	writeFile(t, filepath.Join(rootDir, "a.txt"), "a")
	writeFile(t, filepath.Join(rootDir, "b.go"), "package main\n")
	writeFile(t, filepath.Join(rootDir, "dir", "c.txt"), "c")
	writeFile(t, filepath.Join(rootDir, "dir", "sub", "d.go"), "package sub\n")
	writeFile(t, filepath.Join(rootDir, ".git", "config"), "[core]\n")

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.find"): tools.NewBuiltinFindInvoker(rootDir),
		},
	}

	t.Run("BasenamePattern", func(t *testing.T) {
		resp, err := runner.Run(context.Background(), types.ToolID("builtin.find"), "files", json.RawMessage(`{"pattern":"*.go","type":"f"}`), 0)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !resp.Ok {
			t.Fatalf("expected ok=true, got %+v", resp)
		}
		var out struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal(resp.Output, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		// Should match both go files (and not enter .git).
		if !sliceContains(out.Paths, "b.go") {
			t.Fatalf("expected b.go, got %+v", out.Paths)
		}
		if !sliceContains(out.Paths, "dir/sub/d.go") {
			t.Fatalf("expected dir/sub/d.go, got %+v", out.Paths)
		}
	})

	t.Run("PathPattern", func(t *testing.T) {
		resp, err := runner.Run(context.Background(), types.ToolID("builtin.find"), "files", json.RawMessage(`{"pattern":"dir/*.txt","type":"f"}`), 0)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !resp.Ok {
			t.Fatalf("expected ok=true, got %+v", resp)
		}
		var out struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal(resp.Output, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if len(out.Paths) != 1 || out.Paths[0] != "dir/c.txt" {
			t.Fatalf("expected [dir/c.txt], got %+v", out.Paths)
		}
	})

	t.Run("DoubleStarAnywhere", func(t *testing.T) {
		resp, err := runner.Run(context.Background(), types.ToolID("builtin.find"), "files", json.RawMessage(`{"pattern":"**/d.go","type":"f"}`), 0)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !resp.Ok {
			t.Fatalf("expected ok=true, got %+v", resp)
		}
		var out struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal(resp.Output, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if len(out.Paths) != 1 || out.Paths[0] != "dir/sub/d.go" {
			t.Fatalf("expected [dir/sub/d.go], got %+v", out.Paths)
		}
	})

	t.Run("DoubleStarSuffixPattern", func(t *testing.T) {
		resp, err := runner.Run(context.Background(), types.ToolID("builtin.find"), "files", json.RawMessage(`{"pattern":"**/sub/d.go","type":"f"}`), 0)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !resp.Ok {
			t.Fatalf("expected ok=true, got %+v", resp)
		}
		var out struct {
			Paths []string `json:"paths"`
		}
		if err := json.Unmarshal(resp.Output, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if len(out.Paths) != 1 || out.Paths[0] != "dir/sub/d.go" {
			t.Fatalf("expected [dir/sub/d.go], got %+v", out.Paths)
		}
	})
}

func TestBuiltinFind_Files_RejectsEscapeCwd(t *testing.T) {
	resultsStore := store.NewInMemoryResultsStore()
	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.find"): tools.NewBuiltinFindInvoker(t.TempDir()),
		},
	}
	resp, err := runner.Run(context.Background(), types.ToolID("builtin.find"), "files", json.RawMessage(`{"cwd":"../","pattern":"*.go"}`), 0)
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

func TestBuiltinFind_Files_TruncatesByMaxResults(t *testing.T) {
	resultsStore := store.NewInMemoryResultsStore()

	rootDir := t.TempDir()
	writeFile(t, filepath.Join(rootDir, "a.txt"), "a")
	writeFile(t, filepath.Join(rootDir, "b.txt"), "b")

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.find"): tools.NewBuiltinFindInvoker(rootDir),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.find"), "files", json.RawMessage(`{"pattern":"*.txt","type":"f","maxResults":1}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var out struct {
		Paths     []string `json:"paths"`
		Truncated bool     `json:"truncated"`
		Limit     string   `json:"limit"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(out.Paths) != 1 {
		t.Fatalf("expected 1 path, got %+v", out.Paths)
	}
	if !out.Truncated || out.Limit != "maxResults" {
		t.Fatalf("expected truncated=true limit=maxResults, got truncated=%v limit=%q", out.Truncated, out.Limit)
	}
}

func mkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
}

