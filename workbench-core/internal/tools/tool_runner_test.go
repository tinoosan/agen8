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
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type invokerFunc func(ctx context.Context, req types.ToolRequest) (tools.ToolCallResult, error)

func (f invokerFunc) Invoke(ctx context.Context, req types.ToolRequest) (tools.ToolCallResult, error) {
	return f(ctx, req)
}

func TestRunner_Run_PersistsResponseAndArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := store.CreateSession(cfg, "runner test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	inv := invokerFunc(func(ctx context.Context, req types.ToolRequest) (tools.ToolCallResult, error) {
		return tools.ToolCallResult{
			Output: json.RawMessage(`{"ok":true}`),
			Artifacts: []tools.ToolArtifactWrite{
				{Path: "quote.json", Bytes: []byte(`{"price":123}`), MediaType: "application/json"},
			},
		}, nil
	})

	reg := tools.MapRegistry{
		types.ToolID("github.com.acme.stock"): inv,
	}

	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: reg,
	}

	resp, err := runner.Run(context.Background(), types.ToolID("github.com.acme.stock"), "quote.latest", json.RawMessage(`{"symbol":"AAPL"}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if resp.CallID == "" {
		t.Fatalf("expected callId to be set")
	}
	if len(resp.Artifacts) != 1 || resp.Artifacts[0].Path != "quote.json" {
		t.Fatalf("unexpected artifacts: %+v", resp.Artifacts)
	}

	// response.json written
	responsePath := "/results/" + resp.CallID + "/response.json"
	b, err := fs.Read(responsePath)
	if err != nil {
		t.Fatalf("Read response.json: %v", err)
	}

	var saved types.ToolResponse
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatalf("Unmarshal response.json: %v", err)
	}
	if saved.CallID != resp.CallID || saved.ToolID != resp.ToolID || saved.ActionID != resp.ActionID || saved.Ok != resp.Ok {
		t.Fatalf("saved response mismatch: %+v", saved)
	}

	// artifact written
	artifactPath := "/results/" + resp.CallID + "/quote.json"
	ab, err := fs.Read(artifactPath)
	if err != nil {
		t.Fatalf("Read artifact: %v", err)
	}
	if string(ab) != `{"price":123}` {
		t.Fatalf("unexpected artifact bytes: %q", string(ab))
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestRunner_Run_UnknownTool_PersistsErrorResponse(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := store.CreateSession(cfg, "runner unknown tool test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: tools.MapRegistry{},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("github.com.missing.tool"), "missing.action", json.RawMessage(`{}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false response")
	}
	if resp.Error == nil || resp.Error.Code != "unknown_tool" {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	if _, err := fs.Read("/results/" + resp.CallID + "/response.json"); err != nil {
		t.Fatalf("expected persisted response.json: %v", err)
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestRunner_Run_InvalidArtifactPath_ReturnsToolError(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := store.CreateSession(cfg, "runner invalid artifact test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	inv := invokerFunc(func(ctx context.Context, req types.ToolRequest) (tools.ToolCallResult, error) {
		return tools.ToolCallResult{
			Output: json.RawMessage(`{"ok":true}`),
			Artifacts: []tools.ToolArtifactWrite{
				{Path: "../escape.txt", Bytes: []byte("nope"), MediaType: "text/plain"},
			},
		}, nil
	})

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("github.com.acme.stock"): inv,
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("github.com.acme.stock"), "acme.do", json.RawMessage(`{}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false response")
	}
	if resp.Error == nil || resp.Error.Code != "invalid_artifact" {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestRunner_Run_InvokeError_UsesProvidedCode(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := store.CreateSession(cfg, "runner invoke error test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	inv := invokerFunc(func(ctx context.Context, req types.ToolRequest) (tools.ToolCallResult, error) {
		return tools.ToolCallResult{}, &tools.InvokeError{Code: "timeout", Message: "command timed out", Retryable: true}
	})

	runner := tools.Runner{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			types.ToolID("github.com.acme.stock"): inv,
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("github.com.acme.stock"), "acme.do", json.RawMessage(`{}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false response")
	}
	if resp.Error == nil || resp.Error.Code != "timeout" || !resp.Error.Retryable {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	// Ensure no on-disk results directory was created for the run.
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}
