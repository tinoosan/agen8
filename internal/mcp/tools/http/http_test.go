package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"
	"strings"
	"testing"

	credentialdomain "github.com/tinoosan/agen8-mcp-server/internal/services/credential/domain"
)

type testHTTPRoundTripper func(*nethttp.Request) (*nethttp.Response, error)

func (f testHTTPRoundTripper) RoundTrip(req *nethttp.Request) (*nethttp.Response, error) {
	return f(req)
}

type testCredentialResolver struct {
	records map[string]HTTPCredential
	calls   []string
}

func (r *testCredentialResolver) ResolveHTTP(_ context.Context, host string) (HTTPCredential, bool, error) {
	r.calls = append(r.calls, host)
	record, ok := r.records[host]
	return record, ok, nil
}

func TestDecodeRejectsBodyForGet(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com","method":"GET","body":"nope"}`))
	if err == nil || !strings.Contains(err.Error(), "body is not allowed") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestHandleValidRequestWithoutCredentials(t *testing.T) {
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		if req.URL.String() != "https://example.com/data?q=1" {
			t.Fatalf("url=%q", req.URL.String())
		}
		if got := req.Header.Get("Authorization"); got != "" {
			t.Fatalf("authorization=%q want empty", got)
		}
		return textResponse(req, 200, `{"ok":true}`), nil
	})

	result, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{
		"url":"https://example.com/data",
		"method":"GET",
		"params_list":[{"key":"q","value":"1"}]
	}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["status"] != 200 {
		t.Fatalf("status=%v", structured["status"])
	}
	if structured["body"] != `{"ok":true}` {
		t.Fatalf("body=%v", structured["body"])
	}
	if structured["credentialsInjected"] != false {
		t.Fatalf("credentialsInjected=%v", structured["credentialsInjected"])
	}
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com","method":"GET","headers_list":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "headers_list" must be omitted instead of null`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsMalformedJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com"`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("unexpected err=%v", err)
	}
}

// TestDecodeRejectsUnknownField exercises the strict-decoder path
// (DisallowUnknownFields) specifically — distinct from the malformed-JSON
// path, which rejectNullFields catches first. A syntactically valid object
// with an unexpected field passes rejectNullFields and is then rejected by
// the strict decoder.
func TestDecodeRejectsUnknownField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com","method":"GET","bogus":1}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected err=%v", err)
	}
}

// TestDecodeRejectsTrailingJSON documents that the dec.More() guard at
// http.go:259-261 ("trailing JSON tokens are not allowed") is unreachable
// dead code: rejectNullFields runs first and its json.Unmarshal already
// rejects any trailing token, so trailing JSON surfaces as the generic
// "invalid arguments" error rather than the dec.More() message. Verified
// empirically — `{...} {}` and `{...} 5` both error in rejectNullFields,
// while trailing whitespace passes both checks. Pinning the actual message
// keeps the divergence visible (mirrors T3.2's graph wording pin).
func TestDecodeRejectsTrailingJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com","method":"GET"} {}`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("unexpected err=%v", err)
	}
	if strings.Contains(err.Error(), "trailing JSON tokens are not allowed") {
		t.Fatalf("dec.More() guard is expected to be unreachable, but its message surfaced: %v", err)
	}
}

func TestDecodeRejectsMissingURL(t *testing.T) {
	_, err := decode(json.RawMessage(`{"method":"GET"}`))
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsMissingMethod(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com"}`))
	if err == nil || !strings.Contains(err.Error(), "method is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsUnsupportedMethod(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com","method":"TRACE"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported method "TRACE"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNegativeMaxBytes(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com","method":"GET","maxBytes":-1}`))
	if err == nil || !strings.Contains(err.Error(), "maxBytes must be >= 0") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNegativeTimeout(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com","method":"GET","timeoutMs":-1}`))
	if err == nil || !strings.Contains(err.Error(), "timeoutMs must be >= 0") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestHandleInjectsResolvedBearerCredential(t *testing.T) {
	resolver := &testCredentialResolver{records: map[string]HTTPCredential{
		"api.example.com": credential(credentialdomain.InjectionBearer, "", map[string]string{"value": "api-token"}),
	}}
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		if got := req.Header.Get("Authorization"); got != "Bearer api-token" {
			t.Fatalf("authorization=%q", got)
		}
		return textResponse(req, 200, "secret api-token echoed"), nil
	})

	result, err := handler.Handle(context.Background(), CallContext{
		Credentials: resolver,
	}, json.RawMessage(`{"url":"https://api.example.com","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "api.example.com" {
		t.Fatalf("credential calls=%+v", resolver.calls)
	}
	structured := result.Structured.(map[string]any)
	if structured["credentialsInjected"] != true {
		t.Fatalf("credentialsInjected=%v", structured["credentialsInjected"])
	}
	if strings.Contains(structured["body"].(string), "api-token") {
		t.Fatalf("body leaked credential: %q", structured["body"])
	}
}

func TestHandleInjectsResolvedHeaderCredential(t *testing.T) {
	resolver := &testCredentialResolver{records: map[string]HTTPCredential{
		"api.example.com": credential(credentialdomain.InjectionHeader, "X-API-Key", map[string]string{"value": "api-secret"}),
	}}
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		if got := req.Header.Get("X-API-Key"); got != "api-secret" {
			t.Fatalf("X-API-Key=%q", got)
		}
		return textResponse(req, 200, "ok"), nil
	})

	_, err := handler.Handle(context.Background(), CallContext{
		Credentials: resolver,
	}, json.RawMessage(`{"url":"https://api.example.com","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "api.example.com" {
		t.Fatalf("credential calls=%+v", resolver.calls)
	}
}

func TestHandleInjectsResolvedMultiHeaderCredential(t *testing.T) {
	resolver := &testCredentialResolver{records: map[string]HTTPCredential{
		"paper-api.alpaca.markets": {
			Headers: map[string]string{
				"APCA-API-KEY-ID":     "key-id",
				"APCA-API-SECRET-KEY": "secret-key",
			},
		},
	}}
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		if got := req.Header.Get("APCA-API-KEY-ID"); got != "key-id" {
			t.Fatalf("APCA-API-KEY-ID=%q", got)
		}
		if got := req.Header.Get("APCA-API-SECRET-KEY"); got != "secret-key" {
			t.Fatalf("APCA-API-SECRET-KEY=%q", got)
		}
		return textResponse(req, 200, `{"status":"ACTIVE","secret":"secret-key"}`), nil
	})

	result, err := handler.Handle(context.Background(), CallContext{
		Credentials: resolver,
	}, json.RawMessage(`{"url":"https://paper-api.alpaca.markets:443/v2/account","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "paper-api.alpaca.markets" {
		t.Fatalf("credential calls=%+v", resolver.calls)
	}
	structured := result.Structured.(map[string]any)
	if structured["credentialsInjected"] != true {
		t.Fatalf("credentialsInjected=%v", structured["credentialsInjected"])
	}
	if strings.Contains(structured["body"].(string), "secret-key") {
		t.Fatalf("body leaked credential: %q", structured["body"])
	}
}

func TestHandleInjectsResolvedQueryCredential(t *testing.T) {
	resolver := &testCredentialResolver{records: map[string]HTTPCredential{
		"api.example.com": credential(credentialdomain.InjectionQuery, "apikey", map[string]string{"value": "query-secret"}),
	}}
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		if got := req.URL.Query().Get("apikey"); got != "query-secret" {
			t.Fatalf("apikey=%q", got)
		}
		return textResponse(req, 200, "query-secret echoed"), nil
	})

	result, err := handler.Handle(context.Background(), CallContext{
		Credentials: resolver,
	}, json.RawMessage(`{"url":"https://api.example.com/data?q=1","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if strings.Contains(structured["body"].(string), "query-secret") {
		t.Fatalf("body leaked credential: %q", structured["body"])
	}
	if strings.Contains(structured["finalUrl"].(string), "query-secret") {
		t.Fatalf("final url leaked credential: %q", structured["finalUrl"])
	}
}

func TestHandleTruncatesResponse(t *testing.T) {
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		return textResponse(req, 200, "abcdef"), nil
	})
	handler.defaultMaxBytes = 3
	handler.maxBytes = 3

	result, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://example.com","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["body"] != "abc" || structured["truncated"] != true {
		t.Fatalf("structured=%+v", structured)
	}
}

func TestHandleOmitsBinaryResponse(t *testing.T) {
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		return &nethttp.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     nethttp.Header{"Content-Type": []string{"application/octet-stream"}},
			Body:       io.NopCloser(strings.NewReader("\x00\x01")),
			Request:    req,
		}, nil
	})

	result, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://example.com","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["bodyOmitted"] != true || structured["bodyOmittedReason"] != "non-text response" {
		t.Fatalf("structured=%+v", structured)
	}
}

func TestHandleMalformedCredentialFailsLoudly(t *testing.T) {
	resolver := &testCredentialResolver{records: map[string]HTTPCredential{
		"api.example.com": credential(credentialdomain.InjectionHeader, "X-API-Key", map[string]string{}),
	}}
	handler := testHandler(func(*nethttp.Request) (*nethttp.Response, error) {
		return nil, fmt.Errorf("network should not be reached")
	})

	_, err := handler.Handle(context.Background(), CallContext{
		Credentials: resolver,
	}, json.RawMessage(`{"url":"https://api.example.com","method":"GET"}`))
	if err == nil || !strings.Contains(err.Error(), "requires fieldName and value") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func testHandler(fn func(*nethttp.Request) (*nethttp.Response, error)) Handler {
	client := &nethttp.Client{Transport: testHTTPRoundTripper(fn)}
	return Handler{
		client:           client,
		noRedirectClient: client,
		defaultMaxBytes:  defaultHTTPMaxBytes,
		maxBytes:         maxHTTPMaxBytes,
	}
}

func textResponse(req *nethttp.Request, status int, body string) *nethttp.Response {
	return &nethttp.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, nethttp.StatusText(status)),
		Header:     nethttp.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func credential(injection credentialdomain.InjectionMode, fieldName string, values map[string]string) HTTPCredential {
	return HTTPCredential{
		CredentialID: "cred-test",
		Injection:    injection,
		FieldName:    fieldName,
		Values:       values,
	}
}
