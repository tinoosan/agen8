// Package http provides the native MCP `http` tool.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"net/url"
	"strings"
	"time"

	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
)

const (
	Name        = "http"
	Description = "Make a bounded HTTP request with host-matched credentials injected automatically."

	defaultHTTPMaxBytes = 256 * 1024
	maxHTTPMaxBytes     = 2 * 1024 * 1024
	defaultTimeout      = 30 * time.Second
)

type CredentialResolver interface {
	ResolveHTTP(ctx context.Context, host string) (HTTPCredential, bool, error)
}

type HTTPCredential struct {
	CredentialID credentialdomain.ID
	Injection    credentialdomain.InjectionMode
	FieldName    string
	Headers      map[string]string
	Values       map[string]string
}

type CallContext struct {
	Credentials CredentialResolver
}

type Result struct {
	Text       string
	Structured any
}

type Handler struct {
	client           *nethttp.Client
	noRedirectClient *nethttp.Client
	defaultMaxBytes  int
	maxBytes         int
}

func NewHandler() Handler {
	return Handler{
		client: &nethttp.Client{
			Timeout: defaultTimeout,
		},
		noRedirectClient: &nethttp.Client{
			Timeout: defaultTimeout,
			CheckRedirect: func(*nethttp.Request, []*nethttp.Request) error {
				return nethttp.ErrUseLastResponse
			},
		},
		defaultMaxBytes: defaultHTTPMaxBytes,
		maxBytes:        maxHTTPMaxBytes,
	}
}

func (h Handler) Schema() json.RawMessage {
	return mustHTTPSchema()
}

func (h Handler) Handle(ctx context.Context, call CallContext, args json.RawMessage) (Result, error) {
	input, err := decode(args)
	if err != nil {
		return Result{}, err
	}
	return h.run(ctx, call, input)
}

func (h Handler) run(ctx context.Context, call CallContext, input requestInput) (Result, error) {
	if h.client == nil {
		h = NewHandler()
	}
	if h.noRedirectClient == nil {
		h.noRedirectClient = &nethttp.Client{
			Timeout: defaultTimeout,
			CheckRedirect: func(*nethttp.Request, []*nethttp.Request) error {
				return nethttp.ErrUseLastResponse
			},
		}
	}
	if h.defaultMaxBytes <= 0 {
		h.defaultMaxBytes = defaultHTTPMaxBytes
	}
	if h.maxBytes <= 0 {
		h.maxBytes = maxHTTPMaxBytes
	}

	parsedURL, err := url.Parse(strings.TrimSpace(input.URL))
	if err != nil || parsedURL == nil {
		return Result{}, fmt.Errorf("http: invalid url %q", input.URL)
	}
	switch strings.ToLower(strings.TrimSpace(parsedURL.Scheme)) {
	case "http", "https":
	default:
		return Result{}, fmt.Errorf("http: url scheme must be http or https")
	}
	if strings.TrimSpace(parsedURL.Host) == "" {
		return Result{}, fmt.Errorf("http: url host is required")
	}
	if len(input.Params) > 0 {
		queryValues := parsedURL.Query()
		for key, value := range input.Params {
			queryValues.Set(key, value)
		}
		parsedURL.RawQuery = queryValues.Encode()
	}

	headers := cloneHeaderMap(input.Headers)
	requestURL := parsedURL.String()
	injected, injectedValues, err := h.injectCredential(ctx, call, parsedURL.Hostname(), &requestURL, headers)
	if err != nil {
		return Result{}, err
	}

	maxBytes := input.MaxBytes
	if maxBytes == 0 {
		maxBytes = h.defaultMaxBytes
	}
	if maxBytes > h.maxBytes {
		return Result{}, fmt.Errorf("http: maxBytes exceeds max %d", h.maxBytes)
	}

	requestContext := ctx
	cancel := func() {}
	if input.TimeoutMs > 0 {
		requestContext, cancel = context.WithTimeout(ctx, time.Duration(input.TimeoutMs)*time.Millisecond)
	}
	defer cancel()

	var bodyReader io.Reader
	if input.Body != "" {
		bodyReader = strings.NewReader(input.Body)
	}
	httpReq, err := nethttp.NewRequestWithContext(requestContext, input.Method, requestURL, bodyReader)
	if err != nil {
		return Result{}, fmt.Errorf("http: build request: %w", err)
	}
	for key, value := range headers {
		headerName := strings.TrimSpace(key)
		if headerName == "" {
			continue
		}
		httpReq.Header.Set(headerName, strings.TrimSpace(value))
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", "agen8-mcp.http/0.1")
	}

	client := h.client
	if !input.FollowRedirects {
		client = h.noRedirectClient
	}
	httpResp, err := client.Do(httpReq)
	if err != nil {
		if requestContext.Err() == context.DeadlineExceeded {
			return Result{}, fmt.Errorf("http: request timed out")
		}
		if requestContext.Err() != nil {
			return Result{}, fmt.Errorf("http: request cancelled")
		}
		return Result{}, fmt.Errorf("http: request failed: %w", err)
	}
	defer httpResp.Body.Close()

	bodyBytes, truncated, err := readLimited(httpResp.Body, maxBytes)
	if err != nil {
		return Result{}, fmt.Errorf("http: read response: %w", err)
	}
	finalURL := requestURL
	if httpResp.Request != nil && httpResp.Request.URL != nil {
		finalURL = httpResp.Request.URL.String()
	}
	contentType := strings.TrimSpace(httpResp.Header.Get("Content-Type"))
	textBody := ""
	bodyOmitted := false
	bodyOmittedReason := ""
	if isTextContentType(contentType) && utf8Likely(bodyBytes) {
		textBody = string(bodyBytes)
	} else {
		bodyOmitted = true
		bodyOmittedReason = "non-text response"
	}

	structured := map[string]any{
		"ok":                  true,
		"tool":                Name,
		"url":                 input.URL,
		"finalUrl":            finalURL,
		"status":              httpResp.StatusCode,
		"statusText":          httpResp.Status,
		"headers":             cloneHeader(httpResp.Header),
		"contentType":         contentType,
		"body":                textBody,
		"bodyOmitted":         bodyOmitted,
		"bodyOmittedReason":   bodyOmittedReason,
		"truncated":           truncated,
		"bytesRead":           len(bodyBytes),
		"credentialsInjected": injected,
	}
	redactStructured(structured, injectedValues)
	text, err := encodeText(structured)
	if err != nil {
		return Result{}, err
	}
	return Result{Text: text, Structured: structured}, nil
}

type keyValueEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type request struct {
	URL             string          `json:"url"`
	Method          string          `json:"method"`
	ParamsList      []keyValueEntry `json:"params_list"`
	HeadersList     []keyValueEntry `json:"headers_list"`
	Body            string          `json:"body"`
	MaxBytes        int             `json:"maxBytes"`
	TimeoutMs       int             `json:"timeoutMs"`
	FollowRedirects *bool           `json:"followRedirects"`
}

type requestInput struct {
	URL             string
	Method          string
	Params          map[string]string
	Headers         map[string]string
	Body            string
	MaxBytes        int
	TimeoutMs       int
	FollowRedirects bool
}

func decode(args json.RawMessage) (requestInput, error) {
	if err := rejectNullFields(args); err != nil {
		return requestInput{}, err
	}
	var raw request
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return requestInput{}, fmt.Errorf("http: invalid arguments: %w", err)
	}
	if dec.More() {
		return requestInput{}, fmt.Errorf("http: invalid arguments: trailing JSON tokens are not allowed")
	}
	requestURL := strings.TrimSpace(raw.URL)
	if requestURL == "" {
		return requestInput{}, fmt.Errorf("http: url is required")
	}
	method := normalizeMethod(raw.Method)
	if method == "" {
		return requestInput{}, fmt.Errorf("http: method is required")
	}
	switch method {
	case nethttp.MethodGet, nethttp.MethodHead, nethttp.MethodPost, nethttp.MethodPut, nethttp.MethodPatch, nethttp.MethodDelete:
	default:
		return requestInput{}, fmt.Errorf("http: unsupported method %q", strings.TrimSpace(raw.Method))
	}
	body := raw.Body
	if (method == nethttp.MethodGet || method == nethttp.MethodHead) && strings.TrimSpace(body) != "" {
		return requestInput{}, fmt.Errorf("http: body is not allowed for %s", method)
	}
	if raw.MaxBytes < 0 {
		return requestInput{}, fmt.Errorf("http: maxBytes must be >= 0")
	}
	if raw.TimeoutMs < 0 {
		return requestInput{}, fmt.Errorf("http: timeoutMs must be >= 0")
	}
	followRedirects := true
	if raw.FollowRedirects != nil {
		followRedirects = *raw.FollowRedirects
	}
	return requestInput{
		URL:             requestURL,
		Method:          method,
		Params:          mapFromList(raw.ParamsList),
		Headers:         mapFromList(raw.HeadersList),
		Body:            body,
		MaxBytes:        raw.MaxBytes,
		TimeoutMs:       raw.TimeoutMs,
		FollowRedirects: followRedirects,
	}, nil
}

func rejectNullFields(args json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(args, &fields); err != nil {
		return fmt.Errorf("http: invalid arguments: %w", err)
	}
	for field, raw := range fields {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return fmt.Errorf("http: field %q must be omitted instead of null", field)
		}
	}
	return nil
}

func mustHTTPSchema() json.RawMessage {
	keyValueSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key":   map[string]any{"type": "string"},
			"value": map[string]any{"type": "string"},
		},
		"required":             []string{"key", "value"},
		"additionalProperties": false,
	}
	body, err := json.Marshal(map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":    map[string]any{"type": "string", "description": "Absolute HTTP or HTTPS URL."},
			"method": map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD"}},
			"params_list": map[string]any{
				"type":  "array",
				"items": keyValueSchema,
			},
			"headers_list": map[string]any{
				"type":  "array",
				"items": keyValueSchema,
			},
			"body":            map[string]any{"type": "string"},
			"maxBytes":        map[string]any{"type": "integer", "minimum": 0},
			"timeoutMs":       map[string]any{"type": "integer", "minimum": 0},
			"followRedirects": map[string]any{"type": "boolean"},
		},
		"required":             []string{"url", "method"},
		"additionalProperties": false,
	})
	if err != nil {
		panic(fmt.Sprintf("http schema encode: %v", err))
	}
	return body
}

func (h Handler) injectCredential(ctx context.Context, call CallContext, host string, requestURL *string, headers map[string]string) (bool, []string, error) {
	record, found, err := resolveCredentialRecord(ctx, call, host)
	if err != nil {
		return false, nil, err
	}
	if !found {
		return false, nil, nil
	}
	if len(record.Headers) > 0 {
		injectedValues := make([]string, 0, len(record.Headers))
		for key, value := range record.Headers {
			headerName := strings.TrimSpace(key)
			headerValue := strings.TrimSpace(value)
			if headerName == "" || headerValue == "" {
				return false, nil, fmt.Errorf("http: multi-header credential requires header names and values")
			}
			headers[headerName] = headerValue
			injectedValues = append(injectedValues, headerValue)
		}
		return true, injectedValues, nil
	}
	switch record.Injection {
	case credentialdomain.InjectionBearer:
		token := credentialValue(record.Values, "value", "token", "api_key", "apikey", "key", "secret")
		if token == "" {
			return false, nil, fmt.Errorf("http: bearer credential requires value")
		}
		value := "Bearer " + token
		headers["Authorization"] = value
		return true, []string{token, value}, nil
	case credentialdomain.InjectionHeader:
		headerName := strings.TrimSpace(record.FieldName)
		if headerName == "" {
			headerName = credentialValue(record.Values, "headerName", "header", "name")
		}
		value := credentialValue(record.Values, "value", "token", "api_key", "apikey", "key", "secret")
		if headerName == "" || value == "" {
			return false, nil, fmt.Errorf("http: header credential requires fieldName and value")
		}
		headers[headerName] = value
		return true, []string{value}, nil
	case credentialdomain.InjectionQuery:
		paramName := strings.TrimSpace(record.FieldName)
		if paramName == "" {
			paramName = credentialValue(record.Values, "paramName", "param", "name")
		}
		value := credentialValue(record.Values, "value", "token", "api_key", "apikey", "key", "secret")
		if paramName == "" {
			return false, nil, fmt.Errorf("http: query credential requires fieldName")
		}
		if value == "" {
			return false, nil, fmt.Errorf("http: query credential requires value")
		}
		parsedURL, err := url.Parse(strings.TrimSpace(*requestURL))
		if err != nil {
			return false, nil, fmt.Errorf("http: credential query injection: %w", err)
		}
		queryValues := parsedURL.Query()
		queryValues.Set(paramName, value)
		parsedURL.RawQuery = queryValues.Encode()
		*requestURL = parsedURL.String()
		return true, []string{value}, nil
	default:
		return false, nil, fmt.Errorf("http: unsupported credential injection %q", record.Injection)
	}
}

func resolveCredentialRecord(ctx context.Context, call CallContext, host string) (HTTPCredential, bool, error) {
	if call.Credentials == nil || strings.TrimSpace(host) == "" {
		return HTTPCredential{}, false, nil
	}
	record, found, err := call.Credentials.ResolveHTTP(ctx, strings.TrimSpace(host))
	if err != nil {
		return HTTPCredential{}, false, err
	}
	return record, found, nil
}

func readLimited(body io.Reader, maxBytes int) ([]byte, bool, error) {
	limited := io.LimitReader(body, int64(maxBytes+1))
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return nil, false, err
	}
	truncated := len(bodyBytes) > maxBytes
	if truncated {
		bodyBytes = bodyBytes[:maxBytes]
	}
	return bodyBytes, truncated, nil
}

func normalizeMethod(method string) string {
	return strings.ToUpper(strings.TrimSpace(method))
}

func mapFromList(list []keyValueEntry) map[string]string {
	if len(list) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, entry := range list {
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			continue
		}
		out[key] = entry.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func credentialValue(values map[string]string, keys ...string) string {
	if len(values) == 0 {
		return ""
	}
	normalized := make(map[string]string, len(values))
	for key, value := range values {
		normalized[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}
	for _, key := range keys {
		if value := strings.TrimSpace(normalized[strings.ToLower(strings.TrimSpace(key))]); value != "" {
			return value
		}
	}
	return ""
}

func cloneHeader(h nethttp.Header) map[string][]string {
	if h == nil {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(h))
	for key, values := range h {
		cloneValues := make([]string, len(values))
		copy(cloneValues, values)
		out[key] = cloneValues
	}
	return out
}

func cloneHeaderMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func isTextContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if contentType == "" || strings.HasPrefix(contentType, "text/") {
		return true
	}
	for _, prefix := range []string{"application/json", "application/xml", "application/xhtml+xml", "application/javascript", "application/problem+json"} {
		if strings.HasPrefix(contentType, prefix) || strings.Contains(contentType, "+json") {
			return true
		}
	}
	return false
}

func utf8Likely(payload []byte) bool {
	if len(payload) == 0 {
		return true
	}
	if len(payload) > 32*1024 {
		payload = payload[:32*1024]
	}
	return bytes.IndexByte(payload, 0) == -1
}

func redactStructured(value map[string]any, injectedValues []string) {
	if len(value) == 0 || len(injectedValues) == 0 {
		return
	}
	for key, item := range value {
		switch typed := item.(type) {
		case string:
			value[key] = redactString(typed, injectedValues)
		case map[string][]string:
			value[key] = redactHeaderValues(typed, injectedValues)
		}
	}
}

func redactHeaderValues(headers map[string][]string, injectedValues []string) map[string][]string {
	if len(headers) == 0 || len(injectedValues) == 0 {
		return headers
	}
	out := make(map[string][]string, len(headers))
	for key, values := range headers {
		copied := make([]string, len(values))
		for i, value := range values {
			copied[i] = redactString(value, injectedValues)
		}
		out[key] = copied
	}
	return out
}

func redactString(value string, injectedValues []string) string {
	if value == "" || len(injectedValues) == 0 {
		return value
	}
	redacted := value
	for _, secret := range dedupeValues(injectedValues) {
		redacted = strings.ReplaceAll(redacted, secret, "<redacted>")
	}
	return redacted
}

func dedupeValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func encodeText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("http: encode structured response: %w", err)
	}
	return string(encoded), nil
}
