package tools_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	internaltools "github.com/tinoosan/workbench-core/internal/tools"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestBuiltinHTTP_Fetch_SmallBody_OK(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, run, err := store.CreateSession(cfg, "builtin http test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	runner := pkgtools.Runner{
		Results: resultsStore,
		ToolRegistry: pkgtools.MapRegistry{
			pkgtools.ToolID("builtin.http"): internaltools.NewBuiltinHTTPInvoker(),
		},
	}

	resp, err := runner.Run(context.Background(), pkgtools.ToolID("builtin.http"), "fetch", json.RawMessage(`{"url":"`+srv.URL+`"}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}

	var out struct {
		Status        int    `json:"status"`
		Body          string `json:"body"`
		Truncated     bool   `json:"truncated"`
		BodyTruncated bool   `json:"bodyTruncated"`
		BodyPath      string `json:"bodyPath"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	if out.Status != 200 {
		t.Fatalf("expected status=200, got %d", out.Status)
	}
	if strings.TrimSpace(out.Body) != "hello world" {
		t.Fatalf("unexpected body: %q", out.Body)
	}
	if out.Truncated || out.BodyTruncated {
		t.Fatalf("expected not truncated, got truncated=%v bodyTruncated=%v", out.Truncated, out.BodyTruncated)
	}
	if out.BodyPath != "" {
		t.Fatalf("expected no bodyPath for small body, got %q", out.BodyPath)
	}

	if _, err := fs.Read("/results/" + resp.CallID + "/response.json"); err != nil {
		t.Fatalf("expected persisted response.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "runs", run.RunId, "results")); err == nil {
		t.Fatalf("expected no on-disk results directory")
	}
}

func TestBuiltinHTTP_Fetch_LargeBody_WritesArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.Config{DataDir: tmpDir}

	_, _, err := store.CreateSession(cfg, "builtin http large test", 100)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewResultsResource: %v", err)
	}

	// >16KB preview cap.
	full := strings.Repeat("a", 40*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(full))
	}))
	defer srv.Close()

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, resultsRes)

	runner := pkgtools.Runner{
		Results: resultsStore,
		ToolRegistry: pkgtools.MapRegistry{
			pkgtools.ToolID("builtin.http"): internaltools.NewBuiltinHTTPInvoker(),
		},
	}

	resp, err := runner.Run(context.Background(), pkgtools.ToolID("builtin.http"), "fetch", json.RawMessage(`{"url":"`+srv.URL+`"}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}

	var out struct {
		Body          string `json:"body"`
		BodyTruncated bool   `json:"bodyTruncated"`
		BodyPath      string `json:"bodyPath"`
	}
	if err := json.Unmarshal(resp.Output, &out); err != nil {
		t.Fatalf("Unmarshal output: %v", err)
	}
	if !out.BodyTruncated {
		t.Fatalf("expected bodyTruncated=true for large body")
	}
	if out.BodyPath == "" {
		t.Fatalf("expected bodyPath for large body")
	}
	if len(out.Body) == 0 || len(out.Body) >= len(full) {
		t.Fatalf("expected inline preview shorter than full body, got %d", len(out.Body))
	}

	b, err := fs.Read("/results/" + resp.CallID + "/" + out.BodyPath)
	if err != nil {
		t.Fatalf("Read body artifact: %v", err)
	}
	if string(b) != full {
		t.Fatalf("body artifact mismatch")
	}
}

func TestBuiltinHTTP_Fetch_RejectsNonHTTPURL(t *testing.T) {
	resultsStore := store.NewInMemoryResultsStore()
	runner := pkgtools.Runner{
		Results: resultsStore,
		ToolRegistry: pkgtools.MapRegistry{
			pkgtools.ToolID("builtin.http"): internaltools.NewBuiltinHTTPInvoker(),
		},
	}

	resp, err := runner.Run(context.Background(), pkgtools.ToolID("builtin.http"), "fetch", json.RawMessage(`{"url":"file:///etc/hosts"}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false, got %+v", resp)
	}
	if resp.Error == nil || resp.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input, got %+v", resp.Error)
	}
}
