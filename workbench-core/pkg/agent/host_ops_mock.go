package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tinoosan/workbench-core/pkg/debuglog"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// HostOpExecutor is a tiny "host primitive" dispatcher for demos/tests.
//
// This is not the final host API; it is a concrete reference for the agent-facing
// request/response flow:
//   - fs.list/fs.read/fs.write/fs.append are always available
//   - tool.run executes via tools.Runner and returns a ToolResponse
type HostOpExecutor struct {
	FS     *vfs.FS
	Runner *pkgtools.Runner

	// Core invokers for direct host operations.
	ShellInvoker pkgtools.ToolInvoker
	HTTPInvoker  pkgtools.ToolInvoker
	TraceInvoker pkgtools.ToolInvoker // For all trace actions via BuiltinTraceInvoker

	DefaultMaxBytes int

	// MaxReadBytes caps fs.read payload size returned to the model.
	//
	// This protects the model context window and cost from accidental "read the whole file"
	// requests (e.g. reading large HTML pages, logs, or binary blobs).
	//
	// If zero, no explicit cap is applied beyond DefaultMaxBytes / req.MaxBytes behavior.
	MaxReadBytes int
}

const (
	// defaultToolsReadBytes is the default read budget for tool manifests under /tools/<toolId>.
	//
	// Tool manifests can be larger than the general-purpose fs.read default. If manifests are
	// truncated, the agent can't reliably discover required fields and schemas.
	defaultToolsReadBytes = 64 * 1024
)

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
		// #region agent log
		// H3: validate failures on tool.run (often missing input) can cause tool loops.
		if strings.TrimSpace(req.Op) == types.HostOpToolRun || strings.TrimSpace(req.Op) == "tool_run" {
			debugAppendNDJSON(map[string]any{
				"sessionId":    "debug-session",
				"runId":        "tools-trouble-pre",
				"hypothesisId": "H3",
				"location":     "agent/host_ops_mock.go:validate",
				"message":      "host op validate failed",
				"data": map[string]any{
					"op":        strings.TrimSpace(req.Op),
					"toolId":    strings.TrimSpace(req.ToolID.String()),
					"actionId":  strings.TrimSpace(req.ActionID),
					"inputNil":  req.Input == nil,
					"inputLen":  len(req.Input),
					"timeoutMs": req.TimeoutMs,
					"error":     err.Error(),
				},
				"timestamp": time.Now().UnixMilli(),
			})
		}
		// #endregion
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
		// #region agent log
		// H1/H3: reads to /tools may be truncated or invalid, causing discovery failures.
		isTools := strings.HasPrefix(strings.TrimSpace(req.Path), "/tools/")
		if isTools {
			debugAppendNDJSON(map[string]any{
				"sessionId":    "debug-session",
				"runId":        "tools-trouble-pre",
				"hypothesisId": "H1",
				"location":     "agent/host_ops_mock.go:fs.read:entry",
				"message":      "fs.read request",
				"data": map[string]any{
					"path":         strings.TrimSpace(req.Path),
					"reqMaxBytes":  req.MaxBytes,
					"defaultMax":   x.DefaultMaxBytes,
					"maxReadBytes": x.MaxReadBytes,
				},
				"timestamp": time.Now().UnixMilli(),
			})
		}
		// #endregion

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
		// Special-case: tool manifests must not be truncated by default.
		if strings.HasPrefix(strings.TrimSpace(req.Path), "/tools/") && maxBytes < defaultToolsReadBytes {
			maxBytes = defaultToolsReadBytes
		}
		if x.MaxReadBytes > 0 && maxBytes > x.MaxReadBytes {
			maxBytes = x.MaxReadBytes
		}
		// #region agent log
		if strings.HasPrefix(strings.TrimSpace(req.Path), "/tools/") {
			debugAppendNDJSON(map[string]any{
				"sessionId":    "debug-session",
				"runId":        "tools-trouble-pre",
				"hypothesisId": "H1",
				"location":     "agent/host_ops_mock.go:fs.read:budget",
				"message":      "fs.read budget selected",
				"data": map[string]any{
					"path":        strings.TrimSpace(req.Path),
					"bytesLen":    len(b),
					"maxBytes":    maxBytes,
					"toolsBoost":  defaultToolsReadBytes,
					"maxReadCap":  x.MaxReadBytes,
					"defaultMax":  x.DefaultMaxBytes,
					"reqMaxBytes": req.MaxBytes,
				},
				"timestamp": time.Now().UnixMilli(),
			})
		}
		// #endregion
		text, b64, truncated := encodeReadPayload(b, maxBytes)
		// #region agent log
		if strings.HasPrefix(strings.TrimSpace(req.Path), "/tools/") {
			debugAppendNDJSON(map[string]any{
				"sessionId":    "debug-session",
				"runId":        "tools-trouble-pre",
				"hypothesisId": "H1",
				"location":     "agent/host_ops_mock.go:fs.read:exit",
				"message":      "fs.read response",
				"data": map[string]any{
					"path":         strings.TrimSpace(req.Path),
					"bytesLen":     len(b),
					"returnedText": len(text),
					"returnedB64":  len(b64),
					"truncated":    truncated,
				},
				"timestamp": time.Now().UnixMilli(),
			})
		}
		// #endregion
		return types.HostOpResponse{
			Op:        req.Op,
			Ok:        true,
			BytesLen:  len(b),
			Text:      text,
			BytesB64:  b64,
			Truncated: truncated,
		}

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
			if errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "not found") {
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
			if errors.Is(err, fs.ErrNotExist) || strings.Contains(err.Error(), "not found") {
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

	case types.HostOpToolRun:
		// #region agent log
		// H2/H3/H4: tool.run failures (unknown_tool, invalid_input, timeout) explain "trouble with tools".
		debugAppendNDJSON(map[string]any{
			"sessionId":    "debug-session",
			"runId":        "tools-trouble-pre",
			"hypothesisId": "H2",
			"location":     "agent/host_ops_mock.go:tool.run:entry",
			"message":      "tool.run request",
			"data": map[string]any{
				"toolId":     strings.TrimSpace(req.ToolID.String()),
				"actionId":   strings.TrimSpace(req.ActionID),
				"timeoutMs":  req.TimeoutMs,
				"inputBytes": len(req.Input),
			},
			"timestamp": time.Now().UnixMilli(),
		})
		// #endregion
		if x.Runner == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing Runner"}
		}
		resp, err := x.Runner.Run(ctx, req.ToolID, req.ActionID, req.Input, req.TimeoutMs)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		// #region agent log
		hid := "H2"
		if resp.Error != nil {
			// H3 invalid_input and H4 timeout/tool_failed are common failure modes.
			if strings.TrimSpace(resp.Error.Code) == "invalid_input" {
				hid = "H3"
			} else if strings.TrimSpace(resp.Error.Code) == "timeout" || strings.TrimSpace(resp.Error.Code) == "tool_failed" || strings.TrimSpace(resp.Error.Code) == "unknown_tool" {
				hid = "H4"
			}
		}
		debugAppendNDJSON(map[string]any{
			"sessionId":    "debug-session",
			"runId":        "tools-trouble-pre",
			"hypothesisId": hid,
			"location":     "agent/host_ops_mock.go:tool.run:exit",
			"message":      "tool.run response",
			"data": map[string]any{
				"toolId":   strings.TrimSpace(resp.ToolID.String()),
				"actionId": strings.TrimSpace(resp.ActionID),
				"ok":       resp.Ok,
				"callId":   strings.TrimSpace(resp.CallID),
				"errorCode": func() string {
					if resp.Error != nil {
						return strings.TrimSpace(resp.Error.Code)
					}
					return ""
				}(),
				"retryable": func() bool {
					if resp.Error != nil {
						return resp.Error.Retryable
					}
					return false
				}(),
				"outputBytes": len(resp.Output),
				"artifacts":   len(resp.Artifacts),
			},
			"timestamp": time.Now().UnixMilli(),
		})
		// #endregion
		return types.HostOpResponse{Op: req.Op, Ok: true, ToolResponse: &resp}

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
			ExitCode   int    `json:"exitCode"`
			Stdout     string `json:"stdout"`
			Stderr     string `json:"stderr"`
			StdoutPath string `json:"stdoutPath"`
			StderrPath string `json:"stderrPath"`
		}
		if err := json.Unmarshal(result.Output, &out); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{
			Op:         req.Op,
			Ok:         true,
			ExitCode:   out.ExitCode,
			Stdout:     out.Stdout,
			Stderr:     out.Stderr,
			StdoutPath: out.StdoutPath,
			StderrPath: out.StderrPath,
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
			BodyPath      string              `json:"bodyPath"`
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
			BodyPath:      out.BodyPath,
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
