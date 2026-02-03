package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

type stubBrowserManager struct {
	startHeadless bool

	dismissCalls int
	dismissKind  string
	dismissMode  string

	screenshotAbsPath string
	pdfAbsPath        string
}

func (s *stubBrowserManager) Start(_ context.Context, headless bool) (string, error) {
	s.startHeadless = headless
	return "sess-1", nil
}

func (s *stubBrowserManager) Navigate(_ context.Context, sessionID, url, waitFor string) (string, string, error) {
	_ = sessionID
	_ = waitFor
	return "Example", strings.TrimSpace(url), nil
}

func (s *stubBrowserManager) Dismiss(_ context.Context, sessionID, kind, mode string, maxClicks int) (json.RawMessage, error) {
	_ = sessionID
	_ = maxClicks
	s.dismissCalls++
	s.dismissKind = kind
	s.dismissMode = mode
	return json.RawMessage(`{"clicked":0}`), nil
}

func (s *stubBrowserManager) Click(_ context.Context, sessionID, selector, waitFor string) error {
	_ = sessionID
	_ = selector
	_ = waitFor
	return nil
}

func (s *stubBrowserManager) Fill(_ context.Context, sessionID, selector, text, waitFor string) error {
	_ = sessionID
	_ = selector
	_ = text
	_ = waitFor
	return nil
}

func (s *stubBrowserManager) Extract(_ context.Context, sessionID, selector, attribute string) (json.RawMessage, error) {
	_ = sessionID
	_ = selector
	_ = attribute
	return json.RawMessage(`["a","b"]`), nil
}

func (s *stubBrowserManager) Screenshot(_ context.Context, sessionID, absPath string, fullPage bool) error {
	_ = sessionID
	_ = fullPage
	s.screenshotAbsPath = absPath
	return nil
}

func (s *stubBrowserManager) PDF(_ context.Context, sessionID, absPath string) error {
	_ = sessionID
	s.pdfAbsPath = absPath
	return nil
}

func (s *stubBrowserManager) Close(_ context.Context, sessionID string) error {
	_ = sessionID
	return nil
}

func (s *stubBrowserManager) CleanupStale()   {}
func (s *stubBrowserManager) Shutdown() error { return nil }

func TestHostOpExecutor_Browser_ScreenshotWritesToWorkspace(t *testing.T) {
	tmp := t.TempDir()
	stub := &stubBrowserManager{}
	exec := &HostOpExecutor{
		FS:              vfs.NewFS(),
		Browser:         stub,
		WorkspaceDir:    tmp,
		DefaultMaxBytes: 4096,
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpBrowser,
		Input: json.RawMessage(`{"action":"screenshot","sessionId":"sess-1"}`),
	})
	if !resp.Ok {
		t.Fatalf("expected ok, got error: %s", resp.Error)
	}
	if stub.screenshotAbsPath == "" {
		t.Fatalf("expected manager screenshot to be called")
	}
	if !strings.HasPrefix(stub.screenshotAbsPath, tmp+string(filepath.Separator)) && stub.screenshotAbsPath != tmp {
		t.Fatalf("expected screenshot path within temp dir, got %q", stub.screenshotAbsPath)
	}
	if !strings.Contains(resp.Text, `"/workspace/`) {
		t.Fatalf("expected VFS path in response text, got %q", resp.Text)
	}
}

func TestHostOpExecutor_Browser_Navigate_AutoDismissesCookiesByDefault(t *testing.T) {
	stub := &stubBrowserManager{}
	exec := &HostOpExecutor{
		FS:              vfs.NewFS(),
		Browser:         stub,
		WorkspaceDir:    t.TempDir(),
		DefaultMaxBytes: 4096,
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpBrowser,
		Input: json.RawMessage(`{"action":"navigate","sessionId":"sess-1","url":"https://example.com"}`),
	})
	if !resp.Ok {
		t.Fatalf("expected ok, got error: %s", resp.Error)
	}
	if stub.dismissCalls == 0 {
		t.Fatalf("expected auto-dismiss to be invoked")
	}
	if stub.dismissKind != "cookies" || stub.dismissMode != "accept" {
		t.Fatalf("expected cookies/accept, got %q/%q", stub.dismissKind, stub.dismissMode)
	}
}
