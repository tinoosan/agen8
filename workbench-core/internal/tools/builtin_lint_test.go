package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
)

func TestBuiltinLint_DetectAndFormat_Gofmt_OK(t *testing.T) {
	if _, err := exec.LookPath("gofmt"); err != nil {
		t.Skip("gofmt not installed; skipping")
	}

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/tmp\n\ngo 1.22\n")
	// Make a badly formatted file.
	writeFile(t, filepath.Join(root, "x.go"), "package tmp\n\nfunc  Add(a int,b int) int {return a+b}\n")

	resultsStore := store.NewInMemoryResultsStore()
	_, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.lint"): tools.NewBuiltinLintInvoker(root),
		},
	}

	// detect
	resp, err := runner.Run(context.Background(), types.ToolID("builtin.lint"), "detect", json.RawMessage(`{"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run detect: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var det struct {
		Formatters []string `json:"formatters"`
	}
	if err := json.Unmarshal(resp.Output, &det); err != nil {
		t.Fatalf("Unmarshal detect: %v", err)
	}
	if !sliceContains(det.Formatters, "gofmt") {
		t.Fatalf("expected gofmt formatter, got %+v", det.Formatters)
	}

	// format
	resp, err = runner.Run(context.Background(), types.ToolID("builtin.lint"), "format", json.RawMessage(`{"cwd":".","files":["x.go"]}`), 0)
	if err != nil {
		t.Fatalf("Run format: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var out struct {
		Tool         string   `json:"tool"`
		ExitCode     int      `json:"exitCode"`
		ChangedFiles []string `json:"changedFiles"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal format: %v", err)
	}
	if out.Tool != "gofmt" {
		t.Fatalf("expected tool=gofmt, got %q", out.Tool)
	}
	if out.ExitCode != 0 {
		t.Fatalf("expected exitCode=0, got %d", out.ExitCode)
	}
	if !sliceContains(out.ChangedFiles, "x.go") {
		t.Fatalf("expected x.go in changedFiles, got %+v", out.ChangedFiles)
	}

	afterBytes, err := os.ReadFile(filepath.Join(root, "x.go"))
	if err != nil {
		t.Fatalf("ReadFile after: %v", err)
	}
	after := string(afterBytes)
	if strings.Contains(after, "func  Add") {
		t.Fatalf("expected gofmt to fix spacing, got:\n%s", after)
	}
	if !strings.Contains(after, "func Add(") {
		t.Fatalf("expected gofmt output to contain func Add(, got:\n%s", after)
	}
}

