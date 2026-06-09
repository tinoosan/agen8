// Package http provides the native MCP `http` tool.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"net/url"
	"strings"
	"time"

	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
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

type injectedHTTPCredential struct {
	headerNames map[string]struct{}
	queryKeys   map[string]struct{}
	values      []string
}

type Handler struct {
	client           *nethttp.Client
	noRedirectClient *nethttp.Client
	defaultMaxBytes  int
	maxBytes         int
}

var lookupHostIPs = net.LookupIP

func NewHandler() Handler {
	checkRedirect := func(req *nethttp.Request, via []*nethttp.Request) error {
		return validateRedirectTarget(req, via, nil)
	}
	transport := newValidatedHTTPTransport()

	return Handler{
		client: &nethttp.Client{
			Timeout:       defaultTimeout,
			Transport:     transport,
			CheckRedirect: checkRedirect,
		},
		noRedirectClient: &nethttp.Client{
			Timeout:   defaultTimeout,
			Transport: transport,
			CheckRedirect: func(*nethttp.Request, []*nethttp.Request) error {
				return nethttp.ErrUseLastResponse
			},
		},
		defaultMaxBytes: defaultHTTPMaxBytes,
		maxBytes:        maxHTTPMaxBytes,
	}
}

func newValidatedHTTPTransport() *nethttp.Transport {
	transport, ok := nethttp.DefaultTransport.(*nethttp.Transport)
	if !ok {
		return &nethttp.Transport{DialContext: validatedDialContext(&net.Dialer{})}
	}
	t := transport.Clone()
	t.DialContext = validatedDialContext(&net.Dialer{})
	return t
}

func validatedDialContext(dialer *net.Dialer) func(context.Context, string, string) (net.Conn, error) {
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if err := validateDialTarget(addr); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, addr)
	}
}

func validateDialTarget(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("http: invalid dial target %q: %w", addr, err)
	}
	if err := validateRequestHost(host); err != nil {
		return fmt.Errorf("http: unsafe dial target host %q: %w", host, err)
	}
	return nil
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
	if parsedURL.User != nil {
		return Result{}, fmt.Errorf("http: url must not include userinfo")
	}
	switch strings.ToLower(strings.TrimSpace(parsedURL.Scheme)) {
	case "http", "https":
	default:
		return Result{}, fmt.Errorf("http: url scheme must be http or https")
	}
	if strings.TrimSpace(parsedURL.Host) == "" {
		return Result{}, fmt.Errorf("http: url host is required")
	}
	if err := validateRequestHost(parsedURL.Hostname()); err != nil {
		return Result{}, fmt.Errorf("http: unsafe target host %q: %w", strings.TrimSpace(parsedURL.Hostname()), err)
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
	safeInputURL := sanitizeURLString(requestURL)
	injected, injectedValues, injectedCred, err := h.injectCredential(ctx, call, parsedURL.Hostname(), &requestURL, headers)
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
	} else if injected {
		client = withInjectedRedirectScrubber(client, injectedCred)
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
		"url":                 safeInputURL,
		"finalUrl":            sanitizeURLString(finalURL),
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

func (h Handler) injectCredential(ctx context.Context, call CallContext, host string, requestURL *string, headers map[string]string) (bool, []string, *injectedHTTPCredential, error) {
	record, found, err := resolveCredentialRecord(ctx, call, host)
	if err != nil {
		return false, nil, nil, err
	}
	if !found {
		return false, nil, nil, nil
	}
	injected := &injectedHTTPCredential{headerNames: map[string]struct{}{}, queryKeys: map[string]struct{}{}, values: nil}
	if len(record.Headers) > 0 {
		injectedValues := make([]string, 0, len(record.Headers))
		for key, value := range record.Headers {
			headerName := strings.TrimSpace(key)
			headerValue := strings.TrimSpace(value)
			if headerName == "" || headerValue == "" {
				return false, nil, nil, fmt.Errorf("http: multi-header credential requires header names and values")
			}
			headers[headerName] = headerValue
			injectedValues = append(injectedValues, headerValue)
			injected.headerNames[headerName] = struct{}{}
		}
		injected.values = injectedValues
		return true, injectedValues, injected, nil
	}
	switch record.Injection {
	case credentialdomain.InjectionBearer:
		token := credentialValue(record.Values, "value", "token", "api_key", "apikey", "key", "secret")
		if token == "" {
			return false, nil, nil, fmt.Errorf("http: bearer credential requires value")
		}
		value := "Bearer " + token
		headers["Authorization"] = value
		injected.headerNames["Authorization"] = struct{}{}
		injected.values = []string{token, value}
		return true, injected.values, injected, nil
	case credentialdomain.InjectionHeader:
		headerName := strings.TrimSpace(record.FieldName)
		if headerName == "" {
			headerName = credentialValue(record.Values, "headerName", "header", "name")
		}
		value := credentialValue(record.Values, "value", "token", "api_key", "apikey", "key", "secret")
		if headerName == "" || value == "" {
			return false, nil, nil, fmt.Errorf("http: header credential requires fieldName and value")
		}
		headers[headerName] = value
		injected.headerNames[headerName] = struct{}{}
		injected.values = []string{value}
		return true, injected.values, injected, nil
	case credentialdomain.InjectionQuery:
		paramName := strings.TrimSpace(record.FieldName)
		if paramName == "" {
			paramName = credentialValue(record.Values, "paramName", "param", "name")
		}
		value := credentialValue(record.Values, "value", "token", "api_key", "apikey", "key", "secret")
		if paramName == "" {
			return false, nil, nil, fmt.Errorf("http: query credential requires fieldName")
		}
		if value == "" {
			return false, nil, nil, fmt.Errorf("http: query credential requires value")
		}
		parsedURL, err := url.Parse(strings.TrimSpace(*requestURL))
		if err != nil {
			return false, nil, nil, fmt.Errorf("http: credential query injection: %w", err)
		}
		queryValues := parsedURL.Query()
		queryValues.Set(paramName, value)
		parsedURL.RawQuery = queryValues.Encode()
		*requestURL = parsedURL.String()
		injected.queryKeys[paramName] = struct{}{}
		injected.values = []string{value}
		return true, injected.values, injected, nil
	default:
		return false, nil, nil, fmt.Errorf("http: unsupported credential injection %q", record.Injection)
	}
}

func withInjectedRedirectScrubber(client *nethttp.Client, injected *injectedHTTPCredential) *nethttp.Client {
	if client == nil || injected == nil || len(injected.values) == 0 {
		return client
	}
	dup := *client
	dup.CheckRedirect = func(req *nethttp.Request, via []*nethttp.Request) error {
		return validateRedirectTarget(req, via, injected)
	}
	return &dup
}

func validateRedirectTarget(req *nethttp.Request, via []*nethttp.Request, injected *injectedHTTPCredential) error {
	if len(via) >= 10 {
		return fmt.Errorf("http: too many redirects")
	}
	if req == nil || req.URL == nil {
		return nil
	}
	host := strings.TrimSpace(req.URL.Hostname())
	if host == "" {
		return nil
	}
	if err := validateRequestHost(host); err != nil {
		return fmt.Errorf("http: unsafe redirect target host %q: %w", host, err)
	}
	if injected == nil {
		return nil
	}
	return scrubRedirectCredentials(req, via, injected)
}

func scrubRedirectCredentials(req *nethttp.Request, via []*nethttp.Request, injected *injectedHTTPCredential) error {
	if injected == nil {
		return nil
	}
	if len(via) == 0 {
		return nil
	}
	previous := via[len(via)-1]
	if previous.URL == nil || req.URL == nil {
		return nil
	}
	if !sameRedirectAuthority(previous.URL, req.URL) {
		scrubHeaderCredentials(req.Header, injected)
		removeInjectedQueryParams(req, injected)
	}
	return nil
}

func sameRedirectAuthority(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func scrubHeaderCredentials(headers nethttp.Header, injected *injectedHTTPCredential) {
	if headers == nil || injected == nil || len(injected.headerNames) == 0 {
		return
	}
	for key := range injected.headerNames {
		headers.Del(key)
	}
}

func removeInjectedQueryParams(req *nethttp.Request, injected *injectedHTTPCredential) {
	if req == nil || req.URL == nil || injected == nil || len(injected.queryKeys) == 0 {
		return
	}
	query := req.URL.Query()
	changed := false
	for key := range injected.queryKeys {
		if query.Get(key) != "" {
			changed = true
		}
		query.Del(key)
	}
	if changed {
		req.URL.RawQuery = query.Encode()
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

func sanitizeURLString(raw string) string {
	parsedURL, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsedURL.User = nil
	return parsedURL.String()
}

func validateRequestHost(host string) error {
	// validateRequestHost is the SSRF boundary for outbound HTTP targets.
	// It rejects ambiguous host forms and any host that resolves to a non
	// global endpoint (loopback/private/link-local/multicast/reserved).
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return fmt.Errorf("host is required")
	}
	// Host parsing in net/url is intentionally strict for percent-encoded forms.
	// We only support plain host names and literal IPs here, so any `%`-encoding
	// is rejected as an unsupported and ambiguous input form.
	if strings.Contains(host, "%") {
		return fmt.Errorf("host contains percent-encoding, which is not supported")
	}
	if ip := net.ParseIP(host); ip != nil {
		return validateTargetIP(host, ip)
	}
	ips, err := lookupHostIPs(host)
	if err != nil {
		return fmt.Errorf("resolve failed: %w", err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("host has no IP records")
	}
	return validateResolvedIPs(host, ips)
}

func validateResolvedIPs(host string, ips []net.IP) error {
	for _, ip := range ips {
		if err := validateTargetIP(host, ip); err != nil {
			return err
		}
	}
	return nil
}

func validateTargetIP(host string, ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("%s has an invalid resolved IP", host)
	}
	if ip.IsUnspecified() {
		return fmt.Errorf("%s resolves to unspecified IP", host)
	}
	if ip.IsLoopback() {
		return fmt.Errorf("%s resolves to loopback IP %q", host, ip)
	}
	if ip.IsPrivate() {
		return fmt.Errorf("%s resolves to private IP %q", host, ip)
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("%s resolves to link-local IP %q", host, ip)
	}
	if ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return fmt.Errorf("%s resolves to multicast IP %q", host, ip)
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 0 {
			return fmt.Errorf("%s resolves to reserved IPv4 %q", host, ip)
		}
	}
	if !ip.IsGlobalUnicast() {
		return fmt.Errorf("%s resolves to non-global IP %q", host, ip)
	}
	return nil
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
		value[key] = redactValue(item, injectedValues)
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

func redactValue(value any, injectedValues []string) any {
	switch typed := value.(type) {
	case string:
		return redactString(typed, injectedValues)
	case map[string][]string:
		return redactHeaderValues(typed, injectedValues)
	case map[string]any:
		redactStructured(typed, injectedValues)
		return typed
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, entry := range typed {
			out[key] = redactString(entry, injectedValues)
		}
		return out
	case []string:
		if len(typed) == 0 {
			return typed
		}
		out := make([]string, 0, len(typed))
		for _, entry := range typed {
			out = append(out, redactString(entry, injectedValues))
		}
		return out
	case []any:
		if len(typed) == 0 {
			return typed
		}
		out := make([]any, 0, len(typed))
		for _, entry := range typed {
			out = append(out, redactValue(entry, injectedValues))
		}
		return out
	default:
		return value
	}
}

func redactString(value string, injectedValues []string) string {
	if value == "" || len(injectedValues) == 0 {
		return value
	}
	redacted := value
	for _, secret := range dedupeValues(injectedValues) {
		for _, variant := range redactionCandidates(secret) {
			redacted = strings.ReplaceAll(redacted, variant, "<redacted>")
		}
	}
	return redacted
}

func redactionCandidates(secret string) []string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil
	}
	out := make([]string, 0, 6)
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	add(secret)
	add(url.QueryEscape(secret))
	add(url.PathEscape(secret))
	add(strings.ToLower(url.QueryEscape(secret)))
	add(strings.ToUpper(url.QueryEscape(secret)))
	if unesc, err := url.QueryUnescape(secret); err == nil {
		add(unesc)
	}
	return out
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
