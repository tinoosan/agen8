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

func (s *stubBrowserManager) Start(_ context.Context, headless bool, userAgent string, viewportWidth, viewportHeight int, extraHeaders map[string]string) (string, error) {
	_ = userAgent
	_ = viewportWidth
	_ = viewportHeight
	_ = extraHeaders
	s.startHeadless = headless
	return "sess-1", nil
}

func (s *stubBrowserManager) Navigate(_ context.Context, sessionID, url, waitFor string, timeoutMs int) (string, string, error) {
	_ = sessionID
	_ = waitFor
	_ = timeoutMs
	return "Example", strings.TrimSpace(url), nil
}

func (s *stubBrowserManager) Wait(_ context.Context, sessionID, waitType, url, selector, state string, timeoutMs int, sleepMs int) error {
	_ = sessionID
	_ = waitType
	_ = url
	_ = selector
	_ = state
	_ = timeoutMs
	_ = sleepMs
	return nil
}

func (s *stubBrowserManager) Dismiss(_ context.Context, sessionID, kind, mode string, maxClicks int) (json.RawMessage, error) {
	_ = sessionID
	_ = maxClicks
	s.dismissCalls++
	s.dismissKind = kind
	s.dismissMode = mode
	return json.RawMessage(`{"clicked":0}`), nil
}

func (s *stubBrowserManager) Click(_ context.Context, sessionID, selector, waitFor string, expectPopup bool, timeoutMs int) (string, string, string, error) {
	_ = sessionID
	_ = selector
	_ = waitFor
	_ = expectPopup
	_ = timeoutMs
	return "", "", "", nil
}

func (s *stubBrowserManager) Fill(_ context.Context, sessionID, selector, text, waitFor string, timeoutMs int) error {
	_ = sessionID
	_ = selector
	_ = text
	_ = waitFor
	_ = timeoutMs
	return nil
}

func (s *stubBrowserManager) Hover(_ context.Context, sessionID, selector string, timeoutMs int) error {
	_ = sessionID
	_ = selector
	_ = timeoutMs
	return nil
}

func (s *stubBrowserManager) Press(_ context.Context, sessionID, selector, key string, timeoutMs int) error {
	_ = sessionID
	_ = selector
	_ = key
	_ = timeoutMs
	return nil
}

func (s *stubBrowserManager) Scroll(_ context.Context, sessionID string, dx, dy int) error {
	_ = sessionID
	_ = dx
	_ = dy
	return nil
}

func (s *stubBrowserManager) Select(_ context.Context, sessionID, selector string, values []string, timeoutMs int) (json.RawMessage, error) {
	_ = sessionID
	_ = selector
	_ = values
	_ = timeoutMs
	return json.RawMessage(`[]`), nil
}

func (s *stubBrowserManager) SetChecked(_ context.Context, sessionID, selector string, checked bool, timeoutMs int) error {
	_ = sessionID
	_ = selector
	_ = checked
	_ = timeoutMs
	return nil
}

func (s *stubBrowserManager) Upload(_ context.Context, sessionID, selector, absPath string, timeoutMs int) error {
	_ = sessionID
	_ = selector
	_ = absPath
	_ = timeoutMs
	return nil
}

func (s *stubBrowserManager) Download(_ context.Context, sessionID, selector, absPath string, timeoutMs int) (json.RawMessage, error) {
	_ = sessionID
	_ = selector
	_ = absPath
	_ = timeoutMs
	return json.RawMessage(`{}`), nil
}

func (s *stubBrowserManager) GoBack(_ context.Context, sessionID string, timeoutMs int) (string, string, error) {
	_ = sessionID
	_ = timeoutMs
	return "Back", "https://example.com", nil
}

func (s *stubBrowserManager) GoForward(_ context.Context, sessionID string, timeoutMs int) (string, string, error) {
	_ = sessionID
	_ = timeoutMs
	return "Forward", "https://example.com", nil
}

func (s *stubBrowserManager) Reload(_ context.Context, sessionID string, timeoutMs int) (string, string, error) {
	_ = sessionID
	_ = timeoutMs
	return "Reload", "https://example.com", nil
}

func (s *stubBrowserManager) TabNew(_ context.Context, sessionID, url string, setActive bool, timeoutMs int) (string, string, string, error) {
	_ = sessionID
	_ = url
	_ = setActive
	_ = timeoutMs
	return "tab-1", "Tab", "https://example.com", nil
}

func (s *stubBrowserManager) TabList(_ context.Context, sessionID string) (json.RawMessage, error) {
	_ = sessionID
	return json.RawMessage(`[]`), nil
}

func (s *stubBrowserManager) TabSwitch(_ context.Context, sessionID, pageID string) error {
	_ = sessionID
	_ = pageID
	return nil
}

func (s *stubBrowserManager) TabClose(_ context.Context, sessionID, pageID string) error {
	_ = sessionID
	_ = pageID
	return nil
}

func (s *stubBrowserManager) StorageSave(_ context.Context, sessionID, absPath string) error {
	_ = sessionID
	_ = absPath
	return nil
}

func (s *stubBrowserManager) StorageLoad(_ context.Context, sessionID, absPath string) error {
	_ = sessionID
	_ = absPath
	return nil
}

func (s *stubBrowserManager) SetExtraHeaders(_ context.Context, sessionID string, headers map[string]string) error {
	_ = sessionID
	_ = headers
	return nil
}

func (s *stubBrowserManager) SetViewport(_ context.Context, sessionID string, width, height int) error {
	_ = sessionID
	_ = width
	_ = height
	return nil
}

func (s *stubBrowserManager) Extract(_ context.Context, sessionID, selector, attribute string) (json.RawMessage, error) {
	_ = sessionID
	_ = selector
	_ = attribute
	return json.RawMessage(`["a","b"]`), nil
}

func (s *stubBrowserManager) ExtractLinks(_ context.Context, sessionID, selector string) (json.RawMessage, error) {
	_ = sessionID
	_ = selector
	return json.RawMessage(`[]`), nil
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
