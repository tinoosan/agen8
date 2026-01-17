package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tinoosan/workbench-core/internal/types"
)

var builtinHTTPManifest = []byte(`{
  "id": "builtin.http",
  "version": "0.1.0",
  "kind": "builtin",
  "displayName": "Builtin HTTP",
  "description": "Performs HTTP requests and returns a bounded response preview. Large bodies are written as /results/<callId>/body.* artifacts.",
  "actions": [
    {
      "id": "fetch",
      "displayName": "Fetch",
      "description": "Perform an HTTP request (GET by default). Returns status/headers and a bounded response body preview. If the response body is larger than the inline preview cap, the full read body is written as an artifact and referenced by bodyPath.",
      "inputSchema": {
        "type": "object",
        "properties": {
          "url": { "type": "string" },
          "method": { "type": "string" },
          "headers": { "type": "object" },
          "body": { "type": "string" },
          "maxBytes": { "type": "integer" },
          "followRedirects": { "type": "boolean" }
        },
        "required": ["url"]
      },
      "outputSchema": {
        "type": "object",
        "properties": {
          "finalUrl": { "type": "string" },
          "status": { "type": "integer" },
          "headers": { "type": "object" },
          "contentType": { "type": "string" },
          "bytesRead": { "type": "integer" },
          "truncated": { "type": "boolean" },
          "body": { "type": "string" },
          "bodyTruncated": { "type": "boolean" },
          "bodyPath": { "type": "string" },
          "warning": { "type": "string" }
        },
        "required": ["finalUrl", "status", "headers", "bytesRead", "truncated", "body", "bodyTruncated"]
      }
    }
  ]
}`)

func init() {
	registerBuiltin(BuiltinDef{
		ID:       types.ToolID("builtin.http"),
		Manifest: builtinHTTPManifest,
		NewInvoker: func(cfg BuiltinConfig) ToolInvoker {
			_ = cfg
			return NewBuiltinHTTPInvoker()
		},
	})
}

const (
	defaultHTTPMaxBytes      = 256 * 1024
	maxHTTPMaxBytes          = 2 * 1024 * 1024
	httpInlineBodyPreviewCap = 16 * 1024
)

// BuiltinHTTPInvoker implements the builtin tool "builtin.http" (action: "fetch").
//
// This tool exists for the common "retrieve a URL" workflow without shelling out to
// builtin.bash + curl. Like other builtins:
//   - it is discovered via /tools (manifest JSON bytes)
//   - it is executed via tool.run
//   - it persists response.json (and any artifacts) under /results/<callId>/...
//
// Agent-facing discovery:
//   - fs.List("/tools") includes "/tools/builtin.http"
//   - fs.Read("/tools/builtin.http") returns the manifest JSON bytes
//
// Agent-facing execution (host primitive):
//
//	{
//	  "op": "tool.run",
//	  "toolId": "builtin.http",
//	  "actionId": "fetch",
//	  "input": {
//	    "url": "https://example.com",
//	    "maxBytes": 262144
//	  },
//	  "timeoutMs": 10000
//	}
//
// Output policy:
//   - Reads up to maxBytes (default 256KB, max 2MB) from the response body.
//   - Returns a UTF-8 text preview inline (up to 16KB).
//   - If the body exceeds the inline preview cap, writes the full read body as an artifact:
//   - /results/<callId>/body.txt (text) or /results/<callId>/body.bin (binary)
//   - This tool does not write to /workspace directly; the agent can fs.write if it wants
//     to persist content in the workspace.
//
// Network note:
// This tool can perform outbound HTTP requests. Hosts that want stricter network policy can
// wrap/replace this invoker or add validation (allowed hosts, denylist, etc.) later.
type BuiltinHTTPInvoker struct{}

// NewBuiltinHTTPInvoker constructs a BuiltinHTTPInvoker.
func NewBuiltinHTTPInvoker() *BuiltinHTTPInvoker { return &BuiltinHTTPInvoker{} }

type httpFetchInput struct {
	URL             string            `json:"url"`
	Method          string            `json:"method,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	Body            string            `json:"body,omitempty"`
	MaxBytes        int               `json:"maxBytes,omitempty"`
	FollowRedirects *bool             `json:"followRedirects,omitempty"`
}

type httpFetchOutput struct {
	FinalURL string              `json:"finalUrl"`
	Status   int                 `json:"status"`
	Headers  map[string][]string `json:"headers"`

	ContentType string `json:"contentType,omitempty"`
	BytesRead   int    `json:"bytesRead"`
	Truncated   bool   `json:"truncated"`

	Body          string `json:"body"`
	BodyTruncated bool   `json:"bodyTruncated"`
	BodyPath      string `json:"bodyPath,omitempty"`
	Warning       string `json:"warning,omitempty"`
}

// Invoke executes builtin.http fetch.
func (h *BuiltinHTTPInvoker) Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error) {
	if h == nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "builtin.http invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "fetch" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: fetch)", req.ActionID)}
	}

	var in httpFetchInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	in.URL = strings.TrimSpace(in.URL)
	if in.URL == "" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "url is required"}
	}

	u, err := url.Parse(in.URL)
	if err != nil || u == nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid url %q", in.URL)}
	}
	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "http", "https":
	default:
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "url scheme must be http or https"}
	}

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported method %q", method)}
	}
	if (method == http.MethodGet || method == http.MethodHead) && strings.TrimSpace(in.Body) != "" {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("body is not allowed for %s", method)}
	}

	maxBytes := in.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultHTTPMaxBytes
	}
	if maxBytes < 0 {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: "maxBytes must be >= 0"}
	}
	if maxBytes > maxHTTPMaxBytes {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: fmt.Sprintf("maxBytes exceeds max %d", maxHTTPMaxBytes)}
	}

	var bodyReader io.Reader
	if strings.TrimSpace(in.Body) != "" {
		bodyReader = strings.NewReader(in.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "invalid_input", Message: err.Error(), Err: err}
	}
	for k, v := range in.Headers {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		httpReq.Header.Set(k, strings.TrimSpace(v))
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", "workbench-builtin.http/0.1")
	}

	followRedirects := true
	if in.FollowRedirects != nil {
		followRedirects = *in.FollowRedirects
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}
	if !followRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		if ctx != nil && ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return ToolCallResult{}, &InvokeError{Code: "timeout", Message: "request timed out", Retryable: true, Err: err}
			}
			return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: "request cancelled", Retryable: true, Err: err}
		}
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}
	defer resp.Body.Close()

	// Read up to maxBytes (+1 to detect truncation).
	readLimit := int64(maxBytes)
	limited := io.LimitReader(resp.Body, readLimit+1)
	bodyBytes, readErr := io.ReadAll(limited)
	if readErr != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: readErr.Error(), Err: readErr}
	}
	truncated := len(bodyBytes) > maxBytes
	if truncated {
		bodyBytes = bodyBytes[:maxBytes]
	}

	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	if finalURL == "" {
		finalURL = u.String()
	}

	ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
	isText := isTextContentType(ct) && utf8Likely(bodyBytes)

	out := httpFetchOutput{
		FinalURL:    finalURL,
		Status:      resp.StatusCode,
		Headers:     cloneHeader(resp.Header),
		BytesRead:   len(bodyBytes),
		Truncated:   truncated,
		ContentType: ct,
	}

	var artifacts []ToolArtifactWrite
	fullBodyPath := ""
	if len(bodyBytes) > httpInlineBodyPreviewCap {
		if isText {
			fullBodyPath = "body.txt"
		} else {
			fullBodyPath = "body.bin"
		}
		mediaType := ct
		if strings.TrimSpace(mediaType) == "" {
			if isText {
				mediaType = "text/plain; charset=utf-8"
			} else {
				mediaType = "application/octet-stream"
			}
		}
		artifacts = append(artifacts, ToolArtifactWrite{
			Path:      fullBodyPath,
			Bytes:     append([]byte(nil), bodyBytes...),
			MediaType: mediaType,
		})
	}

	if isText {
		preview := string(bodyBytes)
		if len(preview) > httpInlineBodyPreviewCap {
			preview = preview[:httpInlineBodyPreviewCap]
			out.BodyTruncated = true
		}
		if out.Truncated {
			out.BodyTruncated = true
		}
		out.Body = preview
	} else {
		out.Body = ""
		out.Warning = "response body is binary or non-text; preview omitted"
		if fullBodyPath == "" && len(bodyBytes) > 0 {
			// Keep the UX consistent: if we can't preview, store bytes as an artifact when non-empty.
			fullBodyPath = "body.bin"
			mediaType := ct
			if strings.TrimSpace(mediaType) == "" {
				mediaType = "application/octet-stream"
			}
			artifacts = append(artifacts, ToolArtifactWrite{
				Path:      fullBodyPath,
				Bytes:     append([]byte(nil), bodyBytes...),
				MediaType: mediaType,
			})
		}
	}
	if fullBodyPath != "" {
		out.BodyPath = fullBodyPath
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return ToolCallResult{}, &InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return ToolCallResult{Output: outJSON, Artifacts: artifacts}, nil
}

func cloneHeader(h http.Header) map[string][]string {
	if h == nil {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(h))
	for k, v := range h {
		vs := make([]string, 0, len(v))
		for _, it := range v {
			vs = append(vs, it)
		}
		out[k] = vs
	}
	return out
}

func isTextContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		// Unknown content-type: allow preview when it looks like UTF-8 text.
		return true
	}
	if strings.HasPrefix(ct, "text/") {
		return true
	}
	// Common "text-ish" types.
	for _, p := range []string{
		"application/json",
		"application/xml",
		"application/xhtml+xml",
		"application/javascript",
		"application/x-javascript",
		"application/graphql-response+json",
		"application/problem+json",
	} {
		if strings.HasPrefix(ct, p) {
			return true
		}
	}
	return false
}

func utf8Likely(b []byte) bool {
	// Best-effort heuristic: avoid expensive utf8.Valid on huge bodies.
	// We only need "likely text" for preview decisions.
	if len(b) == 0 {
		return true
	}
	if len(b) > 32*1024 {
		b = b[:32*1024]
	}
	return bytes.IndexByte(b, 0) == -1
}
