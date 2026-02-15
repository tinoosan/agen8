package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/debuglog"
	"github.com/tinoosan/workbench-core/pkg/store"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/tools/builtins"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// BrowserManager abstracts interactive browser sessions for HostOpBrowser.
//
// It is an interface so tests can stub it and so the agent package does not
// depend on a specific browser implementation.
type BrowserManager interface {
	Start(ctx context.Context, headless bool, userAgent string, viewportWidth, viewportHeight int, extraHeaders map[string]string) (sessionID string, err error)
	Navigate(ctx context.Context, sessionID, url, waitFor string, timeoutMs int) (title string, finalURL string, err error)
	Wait(ctx context.Context, sessionID, waitType, url, selector, state string, timeoutMs int, sleepMs int) error
	// Dismiss attempts to dismiss cookie consent banners and popups.
	// kind: cookies|popups|all; mode: accept|reject|close; maxClicks caps total clicks across strategies.
	Dismiss(ctx context.Context, sessionID, kind, mode string, maxClicks int) (json.RawMessage, error)
	Click(ctx context.Context, sessionID, selector, waitFor string, expectPopup bool, timeoutMs int) (popupPageID, popupTitle, popupURL string, err error)
	Fill(ctx context.Context, sessionID, selector, text, waitFor string, timeoutMs int) error
	Hover(ctx context.Context, sessionID, selector string, timeoutMs int) error
	Press(ctx context.Context, sessionID, selector, key string, timeoutMs int) error
	Scroll(ctx context.Context, sessionID string, dx, dy int) error
	Select(ctx context.Context, sessionID, selector string, values []string, timeoutMs int) (json.RawMessage, error)
	SetChecked(ctx context.Context, sessionID, selector string, checked bool, timeoutMs int) error
	Upload(ctx context.Context, sessionID, selector, absPath string, timeoutMs int) error
	Download(ctx context.Context, sessionID, selector, absPath string, timeoutMs int) (json.RawMessage, error)
	GoBack(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error)
	GoForward(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error)
	Reload(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error)
	TabNew(ctx context.Context, sessionID, url string, setActive bool, timeoutMs int) (pageID, title, finalURL string, err error)
	TabList(ctx context.Context, sessionID string) (json.RawMessage, error)
	TabSwitch(ctx context.Context, sessionID, pageID string) error
	TabClose(ctx context.Context, sessionID, pageID string) error
	StorageSave(ctx context.Context, sessionID, absPath string) error
	StorageLoad(ctx context.Context, sessionID, absPath string) error
	SetExtraHeaders(ctx context.Context, sessionID string, headers map[string]string) error
	SetViewport(ctx context.Context, sessionID string, width, height int) error
	Extract(ctx context.Context, sessionID, selector, attribute string) (json.RawMessage, error)
	ExtractLinks(ctx context.Context, sessionID, selector string) (json.RawMessage, error)
	Screenshot(ctx context.Context, sessionID, absPath string, fullPage bool) error
	PDF(ctx context.Context, sessionID, absPath string) error
	Close(ctx context.Context, sessionID string) error
	CleanupStale()
	Shutdown() error
}

// HostOpExecutor is a tiny "host primitive" dispatcher for demos/tests.
//
// This is not the final host API; it is a concrete reference for the agent-facing
// request/response flow:
//   - fs_list/fs_read/fs_search/fs_write/fs_append are always available
type HostOpExecutor struct {
	FS *vfs.FS

	// Core invokers for direct host operations.
	ShellInvoker pkgtools.ToolInvoker
	HTTPInvoker  pkgtools.ToolInvoker
	TraceInvoker pkgtools.ToolInvoker // For all trace actions via BuiltinTraceInvoker
	Browser      BrowserManager
	EmailClient  builtins.EmailSender

	// WorkspaceDir is the host filesystem path backing the /workspace VFS mount.
	// It is used for browser screenshots and PDFs.
	WorkspaceDir string
	// ProjectDir is the host filesystem path backing the /project VFS mount.
	// It is used for browser uploads (e.g., resumes) when the agent references /project paths.
	ProjectDir string

	DefaultMaxBytes int

	// MaxReadBytes caps fs_read payload size returned to the model.
	//
	// This protects the model context window and cost from accidental "read the whole file"
	// requests (e.g. reading large HTML pages, logs, or binary blobs).
	//
	// If zero, no explicit cap is applied beyond DefaultMaxBytes / req.MaxBytes behavior.
	MaxReadBytes int
}

func debugAppendNDJSON(payload map[string]any) {
	// #region agent log
	// Debug-mode log sink (NDJSON).
	//
	// IMPORTANT: Do not log secrets; keep payloads small.
	f, err := debuglog.OpenLogFile()
	if err == nil {
		if b, jerr := json.Marshal(payload); jerr == nil {
			_, _ = f.Write(append(b, '\n'))
		}
		_ = f.Close()
	}
	// #endregion
}

func (x *HostOpExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if x == nil || x.FS == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing FS"}
	}
	if err := req.Validate(); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}

	switch req.Op {
	case types.HostOpNoop:
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: req.Text}

	case types.HostOpFSList:
		entries, err := x.FS.List(req.Path)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Path)
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, Entries: out}

	case types.HostOpFSRead:
		b, err := x.FS.Read(req.Path)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		maxBytes := req.MaxBytes
		if maxBytes == 0 {
			maxBytes = x.DefaultMaxBytes
		}
		if maxBytes <= 0 {
			maxBytes = 4096
		}
		if x.MaxReadBytes > 0 && maxBytes > x.MaxReadBytes {
			maxBytes = x.MaxReadBytes
		}
		text, b64, truncated := encodeReadPayload(b, maxBytes)
		return types.HostOpResponse{
			Op:        req.Op,
			Ok:        true,
			BytesLen:  len(b),
			Text:      text,
			BytesB64:  b64,
			Truncated: truncated,
		}

	case types.HostOpFSSearch:
		results, err := x.FS.Search(ctx, req.Path, req.Query, req.Limit)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, Results: results}

	case types.HostOpFSWrite:
		if err := x.FS.Write(req.Path, []byte(req.Text)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpFSAppend:
		if err := x.FS.Append(req.Path, []byte(req.Text)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpFSEdit:
		beforeBytes, err := x.FS.Read(req.Path)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
				beforeBytes = nil
			} else {
				return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
			}
		}

		before := string(beforeBytes)
		after, err := ApplyStructuredEdits(before, req.Input)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}

		if err := x.FS.Write(req.Path, []byte(after)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpFSPatch:
		beforeBytes, err := x.FS.Read(req.Path)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
				beforeBytes = nil
			} else {
				return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
			}
		}
		after, err := ApplyUnifiedDiffStrict(string(beforeBytes), req.Text)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		if err := x.FS.Write(req.Path, []byte(after)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpShellExec:
		if x.ShellInvoker == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "shell invoker not configured"}
		}
		payload := map[string]any{"argv": req.Argv}
		if strings.TrimSpace(req.Cwd) != "" {
			payload["cwd"] = req.Cwd
		}
		if req.Stdin != "" {
			payload["stdin"] = req.Stdin
		}
		inp, err := json.Marshal(payload)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		toolReq := pkgtools.ToolRequest{
			Version:  "v1",
			CallID:   "shell_exec",
			ToolID:   pkgtools.ToolID("builtin.shell"),
			ActionID: "exec",
			Input:    inp,
		}
		result, err := x.ShellInvoker.Invoke(ctx, toolReq)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		var out struct {
			ExitCode             int    `json:"exitCode"`
			Stdout               string `json:"stdout"`
			Stderr               string `json:"stderr"`
			Warning              string `json:"warning"`
			VFSPathTranslated    bool   `json:"vfsPathTranslated"`
			VFSPathMounts        string `json:"vfsPathMounts"`
			ScriptPathNormalized bool   `json:"scriptPathNormalized"`
			ScriptAntiPattern    string `json:"scriptAntiPattern"`
		}
		if err := json.Unmarshal(result.Output, &out); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		ok := out.ExitCode == 0
		errMsg := ""
		if !ok {
			errMsg = strings.TrimSpace(out.Stderr)
			if errMsg == "" {
				errMsg = fmt.Sprintf("shell_exec exited with code %d", out.ExitCode)
			}
		}
		return types.HostOpResponse{
			Op:                   req.Op,
			Ok:                   ok,
			Error:                errMsg,
			ExitCode:             out.ExitCode,
			Stdout:               out.Stdout,
			Stderr:               out.Stderr,
			Warning:              out.Warning,
			VFSPathTranslated:    out.VFSPathTranslated,
			VFSPathMounts:        strings.TrimSpace(out.VFSPathMounts),
			ScriptPathNormalized: out.ScriptPathNormalized,
			ScriptAntiPattern:    strings.TrimSpace(out.ScriptAntiPattern),
		}

	case types.HostOpHTTPFetch:
		if x.HTTPInvoker == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "http invoker not configured"}
		}
		payload := map[string]any{"url": req.URL}
		if strings.TrimSpace(req.Method) != "" {
			payload["method"] = strings.TrimSpace(req.Method)
		}
		if req.Headers != nil {
			payload["headers"] = req.Headers
		}
		if req.Body != "" {
			payload["body"] = req.Body
		}
		if req.MaxBytes != 0 {
			payload["maxBytes"] = req.MaxBytes
		}
		if req.FollowRedirects != nil {
			payload["followRedirects"] = *req.FollowRedirects
		}
		inp, err := json.Marshal(payload)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		toolReq := pkgtools.ToolRequest{
			Version:  "v1",
			CallID:   "http_fetch",
			ToolID:   pkgtools.ToolID("builtin.http"),
			ActionID: "fetch",
			Input:    inp,
		}
		result, err := x.HTTPInvoker.Invoke(ctx, toolReq)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		var out struct {
			FinalURL      string              `json:"finalUrl"`
			Status        int                 `json:"status"`
			Headers       map[string][]string `json:"headers"`
			ContentType   string              `json:"contentType"`
			BytesRead     int                 `json:"bytesRead"`
			Truncated     bool                `json:"truncated"`
			Body          string              `json:"body"`
			BodyTruncated bool                `json:"bodyTruncated"`
			Warning       string              `json:"warning"`
		}
		if err := json.Unmarshal(result.Output, &out); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{
			Op:            req.Op,
			Ok:            true,
			FinalURL:      out.FinalURL,
			Status:        out.Status,
			Headers:       out.Headers,
			ContentType:   out.ContentType,
			BytesRead:     out.BytesRead,
			Truncated:     out.Truncated,
			Body:          out.Body,
			BodyTruncated: out.BodyTruncated,
			Warning:       out.Warning,
		}

	case types.HostOpTrace:
		// Route all trace actions to TraceInvoker (BuiltinTraceInvoker).
		if x.TraceInvoker == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "trace invoker not configured"}
		}
		toolReq := pkgtools.ToolRequest{
			Version:  "v1",
			CallID:   "trace." + req.Action,
			ToolID:   pkgtools.ToolID("builtin.trace"),
			ActionID: req.Action,
			Input:    req.Input,
		}
		result, err := x.TraceInvoker.Invoke(ctx, toolReq)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: string(result.Output)}

	case types.HostOpEmail:
		if x.EmailClient == nil {
			return types.HostOpResponse{
				Op:    req.Op,
				Ok:    false,
				Error: "email not configured (set GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, GOOGLE_OAUTH_REFRESH_TOKEN, and GMAIL_USER)",
			}
		}
		var params struct {
			To      string `json:"to"`
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}
		if err := json.Unmarshal(req.Input, &params); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		if err := x.EmailClient.Send(params.To, params.Subject, params.Body); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpBrowser:
		if x.Browser == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "browser not configured"}
		}
		if req.Input == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "browser.input is required"}
		}

		var params struct {
			Action      string   `json:"action"`
			SessionID   string   `json:"sessionId"`
			URL         string   `json:"url"`
			WaitType    string   `json:"waitType"`
			State       string   `json:"state"`
			SleepMs     *int     `json:"sleepMs"`
			Selector    string   `json:"selector"`
			Text        string   `json:"text"`
			WaitFor     string   `json:"waitFor"`
			Attribute   string   `json:"attribute"`
			Kind        string   `json:"kind"`
			Mode        string   `json:"mode"`
			MaxClicks   *int     `json:"maxClicks"`
			AutoDismiss *bool    `json:"autoDismiss"`
			TimeoutMs   *int     `json:"timeoutMs"`
			Headless    *bool    `json:"headless"`
			UserAgent   string   `json:"userAgent"`
			ViewportW   *int     `json:"viewportWidth"`
			ViewportH   *int     `json:"viewportHeight"`
			HeadersJSON string   `json:"headersJson"`
			ExpectPopup *bool    `json:"expectPopup"`
			SetActive   *bool    `json:"setActive"`
			PageID      string   `json:"pageId"`
			Key         string   `json:"key"`
			DX          *int     `json:"dx"`
			DY          *int     `json:"dy"`
			Value       string   `json:"value"`
			Values      []string `json:"values"`
			FilePath    string   `json:"filePath"`
			Filename    string   `json:"filename"`
			FullPage    *bool    `json:"fullPage"`
		}
		if err := json.Unmarshal(req.Input, &params); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		action := strings.ToLower(strings.TrimSpace(params.Action))
		if action == "" {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "action is required"}
		}

		switch action {
		case "start":
			headless := true
			if params.Headless != nil {
				headless = *params.Headless
			}
			viewportW, viewportH := 0, 0
			if params.ViewportW != nil {
				viewportW = *params.ViewportW
			}
			if params.ViewportH != nil {
				viewportH = *params.ViewportH
			}
			var extraHeaders map[string]string
			if strings.TrimSpace(params.HeadersJSON) != "" {
				if err := json.Unmarshal([]byte(params.HeadersJSON), &extraHeaders); err != nil {
					return types.HostOpResponse{Op: "browser.start", Ok: false, Error: "headersJson must be a JSON object of string values: " + err.Error()}
				}
			}
			sessionID, err := x.Browser.Start(ctx, headless, strings.TrimSpace(params.UserAgent), viewportW, viewportH, extraHeaders)
			if err != nil {
				return types.HostOpResponse{Op: "browser.start", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"sessionId": sessionID})
			return types.HostOpResponse{Op: "browser.start", Ok: true, Text: string(b)}

		case "navigate":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			title, finalURL, err := x.Browser.Navigate(ctx, params.SessionID, params.URL, params.WaitFor, timeoutMs)
			if err != nil {
				return types.HostOpResponse{Op: "browser.navigate", Ok: false, Error: err.Error()}
			}
			autoDismiss := true
			if params.AutoDismiss != nil {
				autoDismiss = *params.AutoDismiss
			}
			if autoDismiss {
				// Best-effort: accept cookie consent banners that block interaction.
				_, _ = x.Browser.Dismiss(ctx, params.SessionID, "cookies", "accept", 3)
			}
			b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
			return types.HostOpResponse{Op: "browser.navigate", Ok: true, Text: string(b)}

		case "wait":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			sleepMs := 0
			if params.SleepMs != nil && *params.SleepMs > 0 {
				sleepMs = *params.SleepMs
			}
			if err := x.Browser.Wait(ctx, params.SessionID, strings.TrimSpace(params.WaitType), params.URL, params.Selector, strings.TrimSpace(params.State), timeoutMs, sleepMs); err != nil {
				return types.HostOpResponse{Op: "browser.wait", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.wait", Ok: true}

		case "dismiss":
			maxClicks := 3
			if params.MaxClicks != nil && *params.MaxClicks > 0 {
				maxClicks = *params.MaxClicks
			}
			kind := strings.TrimSpace(params.Kind)
			if kind == "" {
				kind = "cookies"
			}
			mode := strings.TrimSpace(params.Mode)
			if mode == "" {
				if kind == "popups" {
					mode = "close"
				} else {
					mode = "accept"
				}
			}
			out, err := x.Browser.Dismiss(ctx, params.SessionID, kind, mode, maxClicks)
			if err != nil {
				return types.HostOpResponse{Op: "browser.dismiss", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.dismiss", Ok: true, Text: strings.TrimSpace(string(out))}

		case "click":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			expectPopup := false
			if params.ExpectPopup != nil {
				expectPopup = *params.ExpectPopup
			}
			pageID, title, finalURL, err := x.Browser.Click(ctx, params.SessionID, params.Selector, params.WaitFor, expectPopup, timeoutMs)
			if err != nil {
				return types.HostOpResponse{Op: "browser.click", Ok: false, Error: err.Error()}
			}
			if strings.TrimSpace(pageID) != "" {
				b, _ := json.Marshal(map[string]string{"pageId": strings.TrimSpace(pageID), "title": strings.TrimSpace(title), "url": strings.TrimSpace(finalURL)})
				return types.HostOpResponse{Op: "browser.click", Ok: true, Text: string(b)}
			}
			return types.HostOpResponse{Op: "browser.click", Ok: true}

		case "type":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			if err := x.Browser.Fill(ctx, params.SessionID, params.Selector, params.Text, params.WaitFor, timeoutMs); err != nil {
				return types.HostOpResponse{Op: "browser.type", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.type", Ok: true}

		case "hover":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			if err := x.Browser.Hover(ctx, params.SessionID, params.Selector, timeoutMs); err != nil {
				return types.HostOpResponse{Op: "browser.hover", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.hover", Ok: true}

		case "press":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			if err := x.Browser.Press(ctx, params.SessionID, params.Selector, params.Key, timeoutMs); err != nil {
				return types.HostOpResponse{Op: "browser.press", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.press", Ok: true}

		case "scroll":
			dx := 0
			if params.DX != nil {
				dx = *params.DX
			}
			dy := 500
			if params.DY != nil {
				dy = *params.DY
			}
			if err := x.Browser.Scroll(ctx, params.SessionID, dx, dy); err != nil {
				return types.HostOpResponse{Op: "browser.scroll", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.scroll", Ok: true}

		case "select":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			vals := append([]string(nil), params.Values...)
			if strings.TrimSpace(params.Value) != "" {
				vals = append([]string{strings.TrimSpace(params.Value)}, vals...)
			}
			out, err := x.Browser.Select(ctx, params.SessionID, params.Selector, vals, timeoutMs)
			if err != nil {
				return types.HostOpResponse{Op: "browser.select", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.select", Ok: true, Text: strings.TrimSpace(string(out))}

		case "check", "uncheck":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			checked := action == "check"
			if err := x.Browser.SetChecked(ctx, params.SessionID, params.Selector, checked, timeoutMs); err != nil {
				return types.HostOpResponse{Op: "browser." + action, Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser." + action, Ok: true}

		case "upload":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			abs, err := x.resolveBrowserFilePath(params.FilePath)
			if err != nil {
				return types.HostOpResponse{Op: "browser.upload", Ok: false, Error: err.Error()}
			}
			if err := x.Browser.Upload(ctx, params.SessionID, params.Selector, abs, timeoutMs); err != nil {
				return types.HostOpResponse{Op: "browser.upload", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.upload", Ok: true}

		case "download":
			if strings.TrimSpace(x.WorkspaceDir) == "" {
				return types.HostOpResponse{Op: "browser.download", Ok: false, Error: "workspace dir not configured"}
			}
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			filename := strings.TrimSpace(params.Filename)
			if filename == "" {
				filename = fmt.Sprintf("download-%s", uuid.NewString()[:8])
			}
			rel, err := safeRelPath(filename)
			if err != nil {
				return types.HostOpResponse{Op: "browser.download", Ok: false, Error: err.Error()}
			}
			absPath := filepath.Join(x.WorkspaceDir, rel)
			out, err := x.Browser.Download(ctx, params.SessionID, params.Selector, absPath, timeoutMs)
			if err != nil {
				return types.HostOpResponse{Op: "browser.download", Ok: false, Error: err.Error()}
			}
			resp := map[string]any{"path": filepath.ToSlash(filepath.Join("/workspace", rel))}
			if len(out) != 0 {
				var extra map[string]any
				if jerr := json.Unmarshal(out, &extra); jerr == nil {
					for k, v := range extra {
						if k == "path" {
							continue
						}
						resp[k] = v
					}
				}
			}
			b, _ := json.Marshal(resp)
			return types.HostOpResponse{Op: "browser.download", Ok: true, Text: string(b)}

		case "back":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			title, finalURL, err := x.Browser.GoBack(ctx, params.SessionID, timeoutMs)
			if err != nil {
				return types.HostOpResponse{Op: "browser.back", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
			return types.HostOpResponse{Op: "browser.back", Ok: true, Text: string(b)}

		case "forward":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			title, finalURL, err := x.Browser.GoForward(ctx, params.SessionID, timeoutMs)
			if err != nil {
				return types.HostOpResponse{Op: "browser.forward", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
			return types.HostOpResponse{Op: "browser.forward", Ok: true, Text: string(b)}

		case "reload":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			title, finalURL, err := x.Browser.Reload(ctx, params.SessionID, timeoutMs)
			if err != nil {
				return types.HostOpResponse{Op: "browser.reload", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
			return types.HostOpResponse{Op: "browser.reload", Ok: true, Text: string(b)}

		case "tab_new":
			timeoutMs := 0
			if params.TimeoutMs != nil && *params.TimeoutMs > 0 {
				timeoutMs = *params.TimeoutMs
			}
			setActive := true
			if params.SetActive != nil {
				setActive = *params.SetActive
			}
			pageID, title, finalURL, err := x.Browser.TabNew(ctx, params.SessionID, params.URL, setActive, timeoutMs)
			if err != nil {
				return types.HostOpResponse{Op: "browser.tab_new", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"pageId": pageID, "title": title, "url": finalURL})
			return types.HostOpResponse{Op: "browser.tab_new", Ok: true, Text: string(b)}

		case "tab_list":
			out, err := x.Browser.TabList(ctx, params.SessionID)
			if err != nil {
				return types.HostOpResponse{Op: "browser.tab_list", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.tab_list", Ok: true, Text: strings.TrimSpace(string(out))}

		case "tab_switch":
			if err := x.Browser.TabSwitch(ctx, params.SessionID, params.PageID); err != nil {
				return types.HostOpResponse{Op: "browser.tab_switch", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.tab_switch", Ok: true}

		case "tab_close":
			if err := x.Browser.TabClose(ctx, params.SessionID, params.PageID); err != nil {
				return types.HostOpResponse{Op: "browser.tab_close", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.tab_close", Ok: true}

		case "storage_save":
			if strings.TrimSpace(x.WorkspaceDir) == "" {
				return types.HostOpResponse{Op: "browser.storage_save", Ok: false, Error: "workspace dir not configured"}
			}
			filename := strings.TrimSpace(params.Filename)
			if filename == "" {
				filename = fmt.Sprintf("storage-%s.json", uuid.NewString()[:8])
			}
			rel, err := safeRelPath(filename)
			if err != nil {
				return types.HostOpResponse{Op: "browser.storage_save", Ok: false, Error: err.Error()}
			}
			absPath := filepath.Join(x.WorkspaceDir, rel)
			if err := x.Browser.StorageSave(ctx, params.SessionID, absPath); err != nil {
				return types.HostOpResponse{Op: "browser.storage_save", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"path": filepath.ToSlash(filepath.Join("/workspace", rel))})
			return types.HostOpResponse{Op: "browser.storage_save", Ok: true, Text: string(b)}

		case "storage_load":
			if strings.TrimSpace(x.WorkspaceDir) == "" {
				return types.HostOpResponse{Op: "browser.storage_load", Ok: false, Error: "workspace dir not configured"}
			}
			filename := strings.TrimSpace(params.Filename)
			if filename == "" {
				return types.HostOpResponse{Op: "browser.storage_load", Ok: false, Error: "filename is required"}
			}
			rel, err := safeRelPath(filename)
			if err != nil {
				return types.HostOpResponse{Op: "browser.storage_load", Ok: false, Error: err.Error()}
			}
			absPath := filepath.Join(x.WorkspaceDir, rel)
			if err := x.Browser.StorageLoad(ctx, params.SessionID, absPath); err != nil {
				return types.HostOpResponse{Op: "browser.storage_load", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"path": filepath.ToSlash(filepath.Join("/workspace", rel))})
			return types.HostOpResponse{Op: "browser.storage_load", Ok: true, Text: string(b)}

		case "set_headers":
			var hdr map[string]string
			if strings.TrimSpace(params.HeadersJSON) != "" {
				if err := json.Unmarshal([]byte(params.HeadersJSON), &hdr); err != nil {
					return types.HostOpResponse{Op: "browser.set_headers", Ok: false, Error: "headersJson must be a JSON object of string values: " + err.Error()}
				}
			}
			if err := x.Browser.SetExtraHeaders(ctx, params.SessionID, hdr); err != nil {
				return types.HostOpResponse{Op: "browser.set_headers", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.set_headers", Ok: true}

		case "set_viewport":
			w, h := 0, 0
			if params.ViewportW != nil {
				w = *params.ViewportW
			}
			if params.ViewportH != nil {
				h = *params.ViewportH
			}
			if w <= 0 || h <= 0 {
				return types.HostOpResponse{Op: "browser.set_viewport", Ok: false, Error: "viewportWidth and viewportHeight are required"}
			}
			if err := x.Browser.SetViewport(ctx, params.SessionID, w, h); err != nil {
				return types.HostOpResponse{Op: "browser.set_viewport", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.set_viewport", Ok: true}

		case "extract":
			data, err := x.Browser.Extract(ctx, params.SessionID, params.Selector, params.Attribute)
			if err != nil {
				return types.HostOpResponse{Op: "browser.extract", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.extract", Ok: true, Text: strings.TrimSpace(string(data))}

		case "extract_links":
			selector := strings.TrimSpace(params.Selector)
			if selector == "" {
				selector = "a"
			}
			data, err := x.Browser.ExtractLinks(ctx, params.SessionID, selector)
			if err != nil {
				return types.HostOpResponse{Op: "browser.extract_links", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.extract_links", Ok: true, Text: strings.TrimSpace(string(data))}

		case "screenshot":
			if strings.TrimSpace(x.WorkspaceDir) == "" {
				return types.HostOpResponse{Op: "browser.screenshot", Ok: false, Error: "workspace dir not configured"}
			}
			fullPage := true
			if params.FullPage != nil {
				fullPage = *params.FullPage
			}
			filename := fmt.Sprintf("screenshot-%s.png", uuid.NewString()[:8])
			absPath := filepath.Join(x.WorkspaceDir, filename)
			if err := x.Browser.Screenshot(ctx, params.SessionID, absPath, fullPage); err != nil {
				return types.HostOpResponse{Op: "browser.screenshot", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"path": filepath.ToSlash(filepath.Join("/workspace", filename))})
			return types.HostOpResponse{Op: "browser.screenshot", Ok: true, Text: string(b)}

		case "pdf":
			if strings.TrimSpace(x.WorkspaceDir) == "" {
				return types.HostOpResponse{Op: "browser.pdf", Ok: false, Error: "workspace dir not configured"}
			}
			filename := fmt.Sprintf("document-%s.pdf", uuid.NewString()[:8])
			absPath := filepath.Join(x.WorkspaceDir, filename)
			if err := x.Browser.PDF(ctx, params.SessionID, absPath); err != nil {
				return types.HostOpResponse{Op: "browser.pdf", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"path": filepath.ToSlash(filepath.Join("/workspace", filename))})
			return types.HostOpResponse{Op: "browser.pdf", Ok: true, Text: string(b)}

		case "close":
			if err := x.Browser.Close(ctx, params.SessionID); err != nil {
				return types.HostOpResponse{Op: "browser.close", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.close", Ok: true}

		default:
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "unknown browser action: " + action}
		}

	default:
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
}

// PrettyJSON is a small helper for demos/logging.
func PrettyJSON(v any) string {
	b, err := types.MarshalPretty(v)
	if err != nil {
		return "<json marshal error: " + err.Error() + ">"
	}
	return string(b)
}

func safeRelPath(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", fmt.Errorf("filename is required")
	}
	if strings.Contains(spec, "\\") {
		spec = strings.ReplaceAll(spec, "\\", "/")
	}
	if strings.HasPrefix(spec, "/") {
		return "", fmt.Errorf("filename must be a relative path")
	}
	clean := filepath.Clean(spec)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("filename is required")
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("filename must not contain '..'")
	}
	return clean, nil
}

func (x *HostOpExecutor) resolveBrowserFilePath(vpath string) (string, error) {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		return "", fmt.Errorf("filePath is required")
	}
	if strings.HasPrefix(vpath, "/workspace/") || vpath == "/workspace" {
		if strings.TrimSpace(x.WorkspaceDir) == "" {
			return "", fmt.Errorf("workspace dir not configured")
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(vpath, "/workspace"), "/")
		if rel == "" {
			return "", fmt.Errorf("filePath must point to a file under /workspace (not the directory)")
		}
		r, err := safeRelPath(rel)
		if err != nil {
			return "", err
		}
		return filepath.Join(x.WorkspaceDir, r), nil
	}
	if strings.HasPrefix(vpath, "/project/") || vpath == "/project" {
		if strings.TrimSpace(x.ProjectDir) == "" {
			return "", fmt.Errorf("project dir not configured")
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(vpath, "/project"), "/")
		if rel == "" {
			return "", fmt.Errorf("filePath must point to a file under /project (not the directory)")
		}
		r, err := safeRelPath(rel)
		if err != nil {
			return "", err
		}
		return filepath.Join(x.ProjectDir, r), nil
	}
	return "", fmt.Errorf("filePath must be a VFS path under /project or /workspace")
}

func encodeReadPayload(b []byte, maxBytes int) (text string, bytesB64 string, truncated bool) {
	if maxBytes <= 0 {
		maxBytes = len(b)
	}
	n := len(b)
	if n > maxBytes {
		n = maxBytes
		truncated = true
	}
	head := b[:n]

	// Prefer returning text when valid UTF-8.
	// If we truncated, try trimming bytes until the prefix is valid UTF-8.
	for len(head) > 0 && !utf8.Valid(head) {
		head = head[:len(head)-1]
	}
	if len(head) > 0 && utf8.Valid(head) {
		return string(head), "", truncated
	}

	// Binary or non-UTF8: return base64 so the contract is lossless.
	return "", base64.StdEncoding.EncodeToString(b[:n]), truncated
}

func AgentSay(logf func(string, ...any), exec func(types.HostOpRequest) types.HostOpResponse, req types.HostOpRequest) types.HostOpResponse {
	logf("agent -> host:\n%s", PrettyJSON(req))
	resp := exec(req)
	// Avoid dumping huge raw bytes; HostOpResponse may contain truncated text or base64.
	logf("host -> agent:\n%s", strings.TrimSpace(PrettyJSON(resp)))
	return resp
}
