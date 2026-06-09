package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	neturl "net/url"
	"os"
	"strings"
	"testing"

	credentialdomain "github.com/tinoosan/agen8/internal/services/credential/domain"
)

type testHTTPRoundTripper func(*nethttp.Request) (*nethttp.Response, error)

func (f testHTTPRoundTripper) RoundTrip(req *nethttp.Request) (*nethttp.Response, error) {
	return f(req)
}

type testCredentialResolver struct {
	records map[string]HTTPCredential
	calls   []string
}

func TestMain(m *testing.M) {
	oldLookupHostIPs := lookupHostIPs
	lookupHostIPs = func(host string) ([]net.IP, error) {
		switch strings.TrimSpace(strings.ToLower(host)) {
		case "localhost":
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		case "127.0.0.1", "::1":
			return []net.IP{net.ParseIP(host)}, nil
		case "api.example.com", "paper-api.alpaca.markets", "attacker.example.net", "example.com":
			return []net.IP{net.ParseIP("198.51.100.42")}, nil
		default:
			return []net.IP{net.ParseIP("198.51.100.10")}, nil
		}
	}
	code := m.Run()
	lookupHostIPs = oldLookupHostIPs
	os.Exit(code)
}

func withTestLookupHostIPs(t *testing.T, overrides map[string][]net.IP, fn func()) {
	old := lookupHostIPs
	lookupHostIPs = func(host string) ([]net.IP, error) {
		if ip, ok := overrides[strings.ToLower(strings.TrimSpace(host))]; ok {
			return ip, nil
		}
		return old(host)
	}
	t.Cleanup(func() {
		lookupHostIPs = old
	})
	fn()
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

// TestDecodeRejectsTrailingJSON feeds trailing JSON after a valid object. The
// json.Unmarshal-based rejectNullFields runs first and rejects any trailing
// token, so the input surfaces as the generic "invalid arguments" error. This
// proves the old dec.More() guard at decode() was unreachable; it was removed
// as dead code (see dec-21debbd9). Trailing input is still rejected loudly, and
// this test guards against the dead guard creeping back.
func TestDecodeRejectsTrailingJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"url":"https://example.com","method":"GET"} {}`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("unexpected err=%v", err)
	}
	if strings.Contains(err.Error(), "trailing JSON tokens are not allowed") {
		t.Fatalf("removed dec.More() guard message resurfaced; it should stay deleted: %v", err)
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

func TestHandleInjectsQueryCredentialWithReservedCharsRedactedFromURLs(t *testing.T) {
	secret := "token value+has/special?chars&equals=plus"
	resolver := &testCredentialResolver{records: map[string]HTTPCredential{
		"api.example.com": credential(credentialdomain.InjectionQuery, "apikey", map[string]string{"value": secret}),
	}}
	encodedSecret := neturl.QueryEscape(secret)
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		if got := req.URL.Query().Get("apikey"); got != secret {
			t.Fatalf("apikey=%q", got)
		}
		return textResponse(req, 200, `{"ok":true}`), nil
	})

	result, err := handler.Handle(context.Background(), CallContext{
		Credentials: resolver,
	}, json.RawMessage(`{"url":"https://api.example.com/data","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	urlValue := structured["url"].(string)
	finalURLValue := structured["finalUrl"].(string)
	if strings.Contains(urlValue, secret) || strings.Contains(finalURLValue, secret) {
		t.Fatalf("raw secret leaked: url=%q finalUrl=%q", urlValue, finalURLValue)
	}
	if strings.Contains(urlValue, encodedSecret) || strings.Contains(finalURLValue, encodedSecret) {
		t.Fatalf("encoded secret leaked: encoded=%q url=%q finalUrl=%q", encodedSecret, urlValue, finalURLValue)
	}
	if !strings.Contains(finalURLValue, "<redacted>") {
		t.Fatalf("expected redaction marker in finalUrl: finalUrl=%q", finalURLValue)
	}
}

func TestRedactStructuredNestedValues(t *testing.T) {
	nested := map[string]any{
		"string": "bearer api-token-value",
		"headers": map[string][]string{
			"Authorization": {"api-token-value", "Bearer api-token-value"},
			"Other":         {"other"},
		},
		"metadata": map[string]any{
			"notes": []any{
				"api-token-value",
				map[string]any{
					"nested": "api-token-value",
					"ignore": "public",
					"arr":    []string{"public", "api-token-value"},
					"mapslice": []any{
						map[string]any{"inner": "api-token-value"},
					},
				},
			},
		},
	}

	injected := []string{"api-token-value"}
	redactStructured(nested, injected)

	if strings.Contains(nested["string"].(string), "api-token-value") {
		t.Fatalf("top-level string was not redacted: %v", nested["string"])
	}
	headerValues := nested["headers"].(map[string][]string)
	for _, value := range headerValues["Authorization"] {
		if strings.Contains(value, "api-token-value") {
			t.Fatalf("header value leaked credential: %v", value)
		}
	}
	metadata := nested["metadata"].(map[string]any)
	notes := metadata["notes"].([]any)
	if got := notes[0].(string); strings.Contains(got, "api-token-value") {
		t.Fatalf("slice value leaked credential: %v", got)
	}
	innerMap := notes[1].(map[string]any)
	if got := innerMap["nested"].(string); strings.Contains(got, "api-token-value") {
		t.Fatalf("nested map value leaked credential: %v", got)
	}
	innerArr := innerMap["arr"].([]string)
	for _, value := range innerArr {
		if strings.Contains(value, "api-token-value") {
			t.Fatalf("array map entry leaked credential: %v", value)
		}
	}
	innerList := innerMap["mapslice"].([]any)
	innerNested := innerList[0].(map[string]any)
	if got := innerNested["inner"].(string); strings.Contains(got, "api-token-value") {
		t.Fatalf("deep nested map leaked credential: %v", got)
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

func TestHandleRejectsURLWithUserinfo(t *testing.T) {
	handler := testHandler(func(*nethttp.Request) (*nethttp.Response, error) {
		t.Fatalf("request should not execute when userinfo is present")
		return nil, nil
	})

	_, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://alice:secret@example.com/data","method":"GET"}`))
	if err == nil || !strings.Contains(err.Error(), "url must not include userinfo") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestHandleRejectsLoopbackTargetHosts(t *testing.T) {
	withTestLookupHostIPs(t, map[string][]net.IP{
		"localhost":         {net.ParseIP("127.0.0.1")},
		"metadata.internal": {net.ParseIP("169.254.169.254")},
	}, func() {
		handler := testHandler(func(*nethttp.Request) (*nethttp.Response, error) {
			t.Fatalf("request should not execute for blocked host")
			return nil, nil
		})
		_, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://localhost","method":"GET"}`))
		if err == nil || !strings.Contains(err.Error(), "unsafe target host") {
			t.Fatalf("localhost err=%v", err)
		}
		_, err = handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://127.0.0.1","method":"GET"}`))
		if err == nil || !strings.Contains(err.Error(), "loopback") {
			t.Fatalf("127.0.0.1 err=%v", err)
		}
		_, err = handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://metadata.internal","method":"GET"}`))
		if err == nil || !strings.Contains(err.Error(), "link-local") {
			t.Fatalf("metadata err=%v", err)
		}
	})
}

func TestHandleRejectsPercentEncodedHostLabel(t *testing.T) {
	handler := testHandler(func(*nethttp.Request) (*nethttp.Response, error) {
		t.Fatalf("request should not execute for encoded host label")
		return nil, nil
	})

	_, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://%6c%6f%63%61%6c%68%6f%73%74","method":"GET"}`))
	if err == nil || !strings.Contains(err.Error(), "invalid url") {
		t.Fatalf("encoded host host err=%v", err)
	}
}

func TestHandleRejectsEncodedIPv6ZoneHostnames(t *testing.T) {
	handler := testHandler(func(*nethttp.Request) (*nethttp.Response, error) {
		t.Fatalf("request should not execute for encoded IPv6 zone hostnames")
		return nil, nil
	})

	_, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://[::1%25EN0]/","method":"GET"}`))
	if err == nil || !strings.Contains(err.Error(), "unsafe target host") || !strings.Contains(err.Error(), "percent-encoding") {
		t.Fatalf("encoded ipv6 zone err=%v", err)
	}
}

func TestHandleRejectsPrivateHostResolution(t *testing.T) {
	withTestLookupHostIPs(t, map[string][]net.IP{
		"private.example.com": {net.ParseIP("10.1.2.3"), net.ParseIP("198.51.100.99")},
	}, func() {
		handler := testHandler(func(*nethttp.Request) (*nethttp.Response, error) {
			t.Fatalf("request should not execute for private resolved host")
			return nil, nil
		})
		_, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://private.example.com","method":"GET"}`))
		if err == nil || !strings.Contains(err.Error(), "private IP") {
			t.Fatalf("private host err=%v", err)
		}
	})
}

func TestHandleAcceptsPublicResolvedHosts(t *testing.T) {
	withTestLookupHostIPs(t, map[string][]net.IP{
		"public.example.com": {net.ParseIP("93.184.216.34")},
	}, func() {
		handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
			if got := req.URL.Hostname(); got != "public.example.com" {
				t.Fatalf("hostname=%q", got)
			}
			return textResponse(req, 200, `{"ok":true}`), nil
		})
		_, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://public.example.com","method":"GET"}`))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
	})
}

func TestHandleStripsInjectedHeaderCredentialOnCrossHostRedirect(t *testing.T) {
	resolver := &testCredentialResolver{records: map[string]HTTPCredential{
		"api.example.com": credential(credentialdomain.InjectionHeader, "X-API-Key", map[string]string{"value": "api-secret"}),
	}}
	handler := Handler{
		client: &nethttp.Client{Transport: testHTTPRoundTripper(func(req *nethttp.Request) (*nethttp.Response, error) {
			switch req.URL.Hostname() {
			case "api.example.com":
				if got := req.Header.Get("X-API-Key"); got != "api-secret" {
					t.Fatalf("first hop should carry injected credential: %q", got)
				}
				return &nethttp.Response{
					StatusCode: 302,
					Status:     "302 Found",
					Header: nethttp.Header{
						"Location": []string{"https://attacker.example.net/callback"},
					},
					Body:    io.NopCloser(strings.NewReader("")),
					Request: req,
				}, nil
			case "attacker.example.net":
				if got := req.Header.Get("X-API-Key"); got != "" {
					t.Fatalf("credential leaked across redirect: %q", got)
				}
				return textResponse(req, 200, `{"ok":true}`), nil
			default:
				return nil, fmt.Errorf("unexpected host %q", req.URL.Hostname())
			}
		})},
		defaultMaxBytes: defaultHTTPMaxBytes,
		maxBytes:        maxHTTPMaxBytes,
	}
	handler.noRedirectClient = handler.client

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

func TestHandleStripsInjectedQueryCredentialOnCrossHostRedirect(t *testing.T) {
	resolver := &testCredentialResolver{records: map[string]HTTPCredential{
		"api.example.com": credential(credentialdomain.InjectionQuery, "apikey", map[string]string{"value": "query-secret"}),
	}}
	handler := Handler{
		client: &nethttp.Client{Transport: testHTTPRoundTripper(func(req *nethttp.Request) (*nethttp.Response, error) {
			switch req.URL.Hostname() {
			case "api.example.com":
				if got := req.URL.Query().Get("apikey"); got != "query-secret" {
					t.Fatalf("first hop should carry injected query credential: %q", got)
				}
				return &nethttp.Response{
					StatusCode: 302,
					Status:     "302 Found",
					Header: nethttp.Header{
						"Location": []string{"https://attacker.example.net/callback?apikey=attacker-token&safe=yes"},
					},
					Body:    io.NopCloser(strings.NewReader("")),
					Request: req,
				}, nil
			case "attacker.example.net":
				if got := req.URL.Query().Get("apikey"); got != "" {
					t.Fatalf("query credential leaked across redirect: %q", got)
				}
				if got := req.URL.Query().Get("safe"); got != "yes" {
					t.Fatalf("query should preserve non-injected params: %q", got)
				}
				return textResponse(req, 200, `{"ok":true}`), nil
			default:
				return nil, fmt.Errorf("unexpected host %q", req.URL.Hostname())
			}
		})},
		defaultMaxBytes: defaultHTTPMaxBytes,
		maxBytes:        maxHTTPMaxBytes,
	}
	handler.noRedirectClient = handler.client

	_, err := handler.Handle(context.Background(), CallContext{
		Credentials: resolver,
	}, json.RawMessage(`{"url":"https://api.example.com/data","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "api.example.com" {
		t.Fatalf("credential calls=%+v", resolver.calls)
	}
}

func TestHandleRejectsRedirectToBlockedHost(t *testing.T) {
	hits := 0
	handler := NewHandler()
	handler.client.Transport = testHTTPRoundTripper(func(req *nethttp.Request) (*nethttp.Response, error) {
		hits++
		switch req.URL.Hostname() {
		case "api.example.com":
			return &nethttp.Response{
				StatusCode: 302,
				Status:     "302 Found",
				Header: nethttp.Header{
					"Location": []string{"https://localhost/secret"},
				},
				Body:    io.NopCloser(strings.NewReader("")),
				Request: req,
			}, nil
		case "localhost":
			t.Fatalf("redirect destination should be rejected before request execution")
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected host %q", req.URL.Hostname())
		}
	})
	handler.noRedirectClient = handler.client

	_, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://api.example.com","method":"GET"}`))
	if err == nil || !strings.Contains(err.Error(), "unsafe redirect target host") || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("handle: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 request hop, got %d", hits)
	}
}

func TestValidateDialTargetBlocksPrivateRebinding(t *testing.T) {
	withTestLookupHostIPs(t, map[string][]net.IP{
		"api.example.com":    {net.ParseIP("198.51.100.55")},
		"rebind.example.net": {net.ParseIP("198.51.100.55"), net.ParseIP("10.1.2.3")},
	}, func() {
		if err := validateDialTarget("api.example.com:443"); err != nil {
			t.Fatalf("unexpected error for public host: %v", err)
		}
		if err := validateDialTarget("rebind.example.net:443"); err == nil {
			t.Fatal("expected rebinding protection to reject mixed/unsafe resolutions")
		}
		if err := validateDialTarget("api.example.com"); err == nil {
			t.Fatalf("expected missing port to fail early with invalid dial target")
		}
	})
}

func TestValidatedDialContextRejectsUnsafeTargetBeforeNetworkConnect(t *testing.T) {
	withTestLookupHostIPs(t, map[string][]net.IP{
		"rebind.example.net": {net.ParseIP("127.0.0.1")},
	}, func() {
		dialFn := validatedDialContext(&net.Dialer{})
		_, err := dialFn(context.Background(), "tcp", "rebind.example.net:443")
		if err == nil || !strings.Contains(err.Error(), "unsafe dial target") {
			t.Fatalf("dialFn should block unsafe target before connect: %v", err)
		}
	})
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

func TestHandleSanitizesFinalUrlUserinfo(t *testing.T) {
	finalURL, err := neturl.Parse("https://alice:secret@example.com/callback")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	handler := testHandler(func(req *nethttp.Request) (*nethttp.Response, error) {
		return &nethttp.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Header:     nethttp.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Request: &nethttp.Request{
				URL: finalURL,
			},
		}, nil
	})

	result, err := handler.Handle(context.Background(), CallContext{}, json.RawMessage(`{"url":"https://example.com","method":"GET"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	finalURLValue := structured["finalUrl"].(string)
	if strings.Contains(finalURLValue, "alice") || strings.Contains(finalURLValue, "@") {
		t.Fatalf("finalUrl leaked userinfo: %q", finalURLValue)
	}
	if finalURLValue != "https://example.com/callback" {
		t.Fatalf("finalUrl unexpected=%q", finalURLValue)
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
