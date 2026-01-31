package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	internalstore "github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/resources"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

type invokerFunc func(ctx context.Context, req tools.ToolRequest) (tools.ToolCallResult, error)

func (f invokerFunc) Invoke(ctx context.Context, req tools.ToolRequest) (tools.ToolCallResult, error) {
	return f(ctx, req)
}

func TestRunner_Run_PersistsResponseAndArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := internalstore.CreateSession(cfg, "runner test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := newTestResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	if err := fs.Mount(vfs.MountResults, resultsRes); err != nil {
		t.Fatalf("mount results: %v", err)
	}

	inv := invokerFunc(func(ctx context.Context, req tools.ToolRequest) (tools.ToolCallResult, error) {
		return tools.ToolCallResult{
			Output: json.RawMessage(`{"ok":true}`),
			Artifacts: []tools.ToolArtifactWrite{
				{Path: "quote.json", Bytes: []byte(`{"price":123}`), MediaType: "application/json"},
			},
		}, nil
	})

	reg := tools.MapRegistry{
		tools.ToolID("github.com.acme.stock"): inv,
	}

	runner := tools.Orchestrator{
		Results:      resultsStore,
		ToolRegistry: reg,
	}

	resp, err := runner.Run(context.Background(), tools.ToolID("github.com.acme.stock"), "quote.latest", json.RawMessage(`{"symbol":"AAPL"}`), 0)
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

	responsePath := "/results/" + resp.CallID + "/response.json"
	b, err := fs.Read(responsePath)
	if err != nil {
		t.Fatalf("Read response.json: %v", err)
	}

	var saved tools.ToolResponse
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatalf("Unmarshal response.json: %v", err)
	}
	if saved.CallID != resp.CallID || saved.ToolID != resp.ToolID || saved.ActionID != resp.ActionID || saved.Ok != resp.Ok {
		t.Fatalf("saved response mismatch: %+v", saved)
	}

	artifactPath := "/results/" + resp.CallID + "/quote.json"
	ab, err := fs.Read(artifactPath)
	if err != nil {
		t.Fatalf("Read artifact: %v", err)
	}
	if string(ab) != `{"price":123}` {
		t.Fatalf("unexpected artifact bytes: %q", string(ab))
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestRunner_Run_UnknownTool_PersistsErrorResponse(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := internalstore.CreateSession(cfg, "runner unknown tool test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := newTestResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	if err := fs.Mount(vfs.MountResults, resultsRes); err != nil {
		t.Fatalf("mount results: %v", err)
	}

	runner := tools.Orchestrator{
		Results:      resultsStore,
		ToolRegistry: tools.MapRegistry{},
	}

	resp, err := runner.Run(context.Background(), tools.ToolID("github.com.missing.tool"), "missing.action", json.RawMessage(`{}`), 0)
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

	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestRunner_Run_InvalidArtifactPath_ReturnsToolError(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := internalstore.CreateSession(cfg, "runner invalid artifact test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := newTestResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	if err := fs.Mount(vfs.MountResults, resultsRes); err != nil {
		t.Fatalf("mount results: %v", err)
	}

	inv := invokerFunc(func(ctx context.Context, req tools.ToolRequest) (tools.ToolCallResult, error) {
		return tools.ToolCallResult{
			Output: json.RawMessage(`{"ok":true}`),
			Artifacts: []tools.ToolArtifactWrite{
				{Path: "../escape.txt", Bytes: []byte("nope"), MediaType: "text/plain"},
			},
		}, nil
	})

	runner := tools.Orchestrator{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			tools.ToolID("github.com.acme.stock"): inv,
		},
	}

	resp, err := runner.Run(context.Background(), tools.ToolID("github.com.acme.stock"), "acme.do", json.RawMessage(`{}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false response")
	}
	if resp.Error == nil || resp.Error.Code != "invalid_artifact" {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestRunner_Run_InvokeError_UsesProvidedCode(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := internalstore.CreateSession(cfg, "runner invoke error test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := newTestResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	if err := fs.Mount(vfs.MountResults, resultsRes); err != nil {
		t.Fatalf("mount results: %v", err)
	}

	inv := invokerFunc(func(ctx context.Context, req tools.ToolRequest) (tools.ToolCallResult, error) {
		return tools.ToolCallResult{}, &tools.InvokeError{Code: "timeout", Message: "command timed out", Retryable: true}
	})

	runner := tools.Orchestrator{
		Results: resultsStore,
		ToolRegistry: tools.MapRegistry{
			tools.ToolID("github.com.acme.stock"): inv,
		},
	}

	resp, err := runner.Run(context.Background(), tools.ToolID("github.com.acme.stock"), "acme.do", json.RawMessage(`{}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false response")
	}
	if resp.Error == nil || resp.Error.Code != "timeout" || !resp.Error.Retryable {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

type testResultsStore struct {
	calls map[string]*testCall
}

type testCall struct {
	responseJSON []byte
	artifacts    map[string]testArtifact
}

type testArtifact struct {
	data      []byte
	mediaType string
}

func newTestResultsStore() *testResultsStore {
	return &testResultsStore{calls: make(map[string]*testCall)}
}

func (s *testResultsStore) PutCall(callID string, responseJSON []byte) error {
	if s.calls[callID] == nil {
		s.calls[callID] = &testCall{artifacts: make(map[string]testArtifact)}
	}
	s.calls[callID].responseJSON = append([]byte(nil), responseJSON...)
	return nil
}

func (s *testResultsStore) PutArtifact(callID, artifactPath, mediaType string, content []byte) error {
	if s.calls[callID] == nil {
		s.calls[callID] = &testCall{artifacts: make(map[string]testArtifact)}
	}
	s.calls[callID].artifacts[artifactPath] = testArtifact{
		data:      append([]byte(nil), content...),
		mediaType: mediaType,
	}
	return nil
}

func (s *testResultsStore) GetCallResponseJSON(callID string) ([]byte, error) {
	c := s.calls[callID]
	if c == nil || c.responseJSON == nil {
		return nil, pkgstore.ErrResultsNotFound
	}
	return append([]byte(nil), c.responseJSON...), nil
}

func (s *testResultsStore) ListCallIDs() ([]string, error) {
	out := make([]string, 0, len(s.calls))
	for id := range s.calls {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (s *testResultsStore) GetArtifact(callID, artifactPath string) ([]byte, string, error) {
	c := s.calls[callID]
	if c == nil {
		return nil, "", pkgstore.ErrResultsNotFound
	}
	a, ok := c.artifacts[artifactPath]
	if !ok {
		return nil, "", pkgstore.ErrResultsNotFound
	}
	return append([]byte(nil), a.data...), a.mediaType, nil
}

func (s *testResultsStore) ListArtifacts(callID string) ([]pkgstore.ArtifactMeta, error) {
	c := s.calls[callID]
	if c == nil {
		return nil, pkgstore.ErrResultsNotFound
	}
	out := make([]pkgstore.ArtifactMeta, 0, len(c.artifacts))
	for p, a := range c.artifacts {
		out = append(out, pkgstore.ArtifactMeta{
			Path:      p,
			MediaType: a.mediaType,
			Size:      int64(len(a.data)),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
