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
	Start(ctx context.Context, headless bool) (sessionID string, err error)
	Navigate(ctx context.Context, sessionID, url, waitFor string) (title string, finalURL string, err error)
	Click(ctx context.Context, sessionID, selector, waitFor string) error
	Fill(ctx context.Context, sessionID, selector, text, waitFor string) error
	Extract(ctx context.Context, sessionID, selector, attribute string) (json.RawMessage, error)
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
//   - fs.list/fs.read/fs.search/fs.write/fs.append are always available
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

	DefaultMaxBytes int

	// MaxReadBytes caps fs.read payload size returned to the model.
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
			CallID:   "shell.exec",
			ToolID:   pkgtools.ToolID("builtin.shell"),
			ActionID: "exec",
			Input:    inp,
		}
		result, err := x.ShellInvoker.Invoke(ctx, toolReq)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		var out struct {
			ExitCode int    `json:"exitCode"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		}
		if err := json.Unmarshal(result.Output, &out); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		ok := out.ExitCode == 0
		errMsg := ""
		if !ok {
			errMsg = strings.TrimSpace(out.Stderr)
			if errMsg == "" {
				errMsg = fmt.Sprintf("shell.exec exited with code %d", out.ExitCode)
			}
		}
		return types.HostOpResponse{
			Op:       req.Op,
			Ok:       ok,
			Error:    errMsg,
			ExitCode: out.ExitCode,
			Stdout:   out.Stdout,
			Stderr:   out.Stderr,
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
			CallID:   "http.fetch",
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
			Action    string `json:"action"`
			SessionID string `json:"sessionId"`
			URL       string `json:"url"`
			Selector  string `json:"selector"`
			Text      string `json:"text"`
			WaitFor   string `json:"waitFor"`
			Attribute string `json:"attribute"`
			Headless  *bool  `json:"headless"`
			FullPage  *bool  `json:"fullPage"`
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
			sessionID, err := x.Browser.Start(ctx, headless)
			if err != nil {
				return types.HostOpResponse{Op: "browser.start", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"sessionId": sessionID})
			return types.HostOpResponse{Op: "browser.start", Ok: true, Text: string(b)}

		case "navigate":
			title, finalURL, err := x.Browser.Navigate(ctx, params.SessionID, params.URL, params.WaitFor)
			if err != nil {
				return types.HostOpResponse{Op: "browser.navigate", Ok: false, Error: err.Error()}
			}
			b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
			return types.HostOpResponse{Op: "browser.navigate", Ok: true, Text: string(b)}

		case "click":
			if err := x.Browser.Click(ctx, params.SessionID, params.Selector, params.WaitFor); err != nil {
				return types.HostOpResponse{Op: "browser.click", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.click", Ok: true}

		case "type":
			if err := x.Browser.Fill(ctx, params.SessionID, params.Selector, params.Text, params.WaitFor); err != nil {
				return types.HostOpResponse{Op: "browser.type", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.type", Ok: true}

		case "extract":
			data, err := x.Browser.Extract(ctx, params.SessionID, params.Selector, params.Attribute)
			if err != nil {
				return types.HostOpResponse{Op: "browser.extract", Ok: false, Error: err.Error()}
			}
			return types.HostOpResponse{Op: "browser.extract", Ok: true, Text: strings.TrimSpace(string(data))}

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
