package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
)

func TestBuiltinTest_Detect_List_Run_Go_OK(t *testing.T) {
	root := t.TempDir()

	// Create a tiny go module with one passing test.
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/tmp\n\ngo 1.22\n")
	writeFile(t, filepath.Join(root, "x.go"), "package tmp\n\nfunc Add(a,b int) int { return a+b }\n")
	writeFile(t, filepath.Join(root, "x_test.go"), "package tmp\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2)!=3 { t.Fatal(\"bad\") }\n}\n")

	resultsStore := store.NewInMemoryResultsStore()
	_, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.test"): tools.NewBuiltinTestInvoker(root),
		},
	}

	// detect
	resp, err := runner.Run(context.Background(), types.ToolID("builtin.test"), "detect", json.RawMessage(`{"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run detect: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var det struct {
		Framework string `json:"framework"`
	}
	if err := json.Unmarshal(resp.Output, &det); err != nil {
		t.Fatalf("Unmarshal detect: %v", err)
	}
	if det.Framework != "go" {
		t.Fatalf("expected framework=go, got %q", det.Framework)
	}

	// list
	resp, err = runner.Run(context.Background(), types.ToolID("builtin.test"), "list", json.RawMessage(`{"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run list: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var lst struct {
		Framework string   `json:"framework"`
		Files     []string `json:"files"`
		Tests     []string `json:"tests"`
	}
	if err := json.Unmarshal(resp.Output, &lst); err != nil {
		t.Fatalf("Unmarshal list: %v", err)
	}
	if lst.Framework != "go" {
		t.Fatalf("expected framework=go, got %q", lst.Framework)
	}
	if !sliceContains(lst.Files, "x_test.go") {
		t.Fatalf("expected x_test.go, got %+v", lst.Files)
	}
	if !sliceContains(lst.Tests, "TestAdd") {
		t.Fatalf("expected TestAdd, got %+v", lst.Tests)
	}

	// run
	resp, err = runner.Run(context.Background(), types.ToolID("builtin.test"), "run", json.RawMessage(`{"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var runOut struct {
		Framework string `json:"framework"`
		ExitCode  int    `json:"exitCode"`
		Passed    bool   `json:"passed"`
		Stdout    string `json:"stdout"`
		Stderr    string `json:"stderr"`
	}
	if err := json.Unmarshal(resp.Output, &runOut); err != nil {
		t.Fatalf("Unmarshal run: %v", err)
	}
	if runOut.Framework != "go" {
		t.Fatalf("expected framework=go, got %q", runOut.Framework)
	}
	if runOut.ExitCode != 0 || !runOut.Passed {
		t.Fatalf("expected passing tests, got exitCode=%d passed=%v stdout=%q stderr=%q", runOut.ExitCode, runOut.Passed, runOut.Stdout, runOut.Stderr)
	}
}

func TestBuiltinTest_Detect_Unknown(t *testing.T) {
	root := t.TempDir()
	// Ensure no markers.
	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("hi\n"), 0644)

	resultsStore := store.NewInMemoryResultsStore()
	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.test"): tools.NewBuiltinTestInvoker(root),
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.test"), "detect", json.RawMessage(`{"cwd":"."}`), 0)
	if err != nil {
		t.Fatalf("Run detect: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	var det struct {
		Framework string `json:"framework"`
	}
	if err := json.Unmarshal(resp.Output, &det); err != nil {
		t.Fatalf("Unmarshal detect: %v", err)
	}
	if det.Framework != "unknown" {
		t.Fatalf("expected framework=unknown, got %q", det.Framework)
	}
}

