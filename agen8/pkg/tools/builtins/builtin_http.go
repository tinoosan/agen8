package builtins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	pkgtools "github.com/tinoosan/agen8/pkg/tools"
)

const (
	defaultHTTPMaxBytes      = 256 * 1024
	maxHTTPMaxBytes          = 2 * 1024 * 1024
	httpInlineBodyPreviewCap = 16 * 1024
)

// BuiltinHTTPInvoker implements the builtin tool "builtin.http" (action: "fetch").
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
	Warning       string `json:"warning,omitempty"`
}

// Invoke executes builtin.http fetch.
func (h *BuiltinHTTPInvoker) Invoke(ctx context.Context, req pkgtools.ToolRequest) (pkgtools.ToolCallResult, error) {
	if h == nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "builtin.http invoker is nil"}
	}
	if err := req.Validate(); err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: err.Error()}
	}
	if req.ActionID != "fetch" {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported action %q (allowed: fetch)", req.ActionID)}
	}

	var in httpFetchInput
	if err := json.Unmarshal(req.Input, &in); err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid input JSON: %v", err)}
	}
	in.URL = strings.TrimSpace(in.URL)
	if in.URL == "" {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: "url is required"}
	}

	u, err := url.Parse(in.URL)
	if err != nil || u == nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("invalid url %q", in.URL)}
	}
	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "http", "https":
	default:
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: "url scheme must be http or https"}
	}

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("unsupported method %q", method)}
	}
	if (method == http.MethodGet || method == http.MethodHead) && strings.TrimSpace(in.Body) != "" {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("body is not allowed for %s", method)}
	}

	maxBytes := in.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultHTTPMaxBytes
	}
	if maxBytes < 0 {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: "maxBytes must be >= 0"}
	}
	if maxBytes > maxHTTPMaxBytes {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: fmt.Sprintf("maxBytes exceeds max %d", maxHTTPMaxBytes)}
	}

	var bodyReader io.Reader
	if strings.TrimSpace(in.Body) != "" {
		bodyReader = strings.NewReader(in.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "invalid_input", Message: err.Error(), Err: err}
	}
	for k, v := range in.Headers {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		httpReq.Header.Set(k, strings.TrimSpace(v))
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", "agen8-builtin.http/0.1")
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
				return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "timeout", Message: "request timed out", Retryable: true, Err: err}
			}
			return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: "request cancelled", Retryable: true, Err: err}
		}
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: err.Error(), Err: err}
	}
	defer resp.Body.Close()

	readLimit := int64(maxBytes)
	limited := io.LimitReader(resp.Body, readLimit+1)
	bodyBytes, readErr := io.ReadAll(limited)
	if readErr != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: readErr.Error(), Err: readErr}
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
		if out.Truncated {
			out.BodyTruncated = true
		}
	}

	outJSON, err := json.Marshal(out)
	if err != nil {
		return pkgtools.ToolCallResult{}, &pkgtools.InvokeError{Code: "tool_failed", Message: fmt.Sprintf("marshal output: %v", err), Err: err}
	}
	return pkgtools.ToolCallResult{Output: outJSON}, nil
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
		return true
	}
	if strings.HasPrefix(ct, "text/") {
		return true
	}
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
	if len(b) == 0 {
		return true
	}
	if len(b) > 32*1024 {
		b = b[:32*1024]
	}
	return bytes.IndexByte(b, 0) == -1
}
