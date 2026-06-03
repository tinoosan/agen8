package infra

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
)

// newTestWebhookChannel constructs a WebhookChannel configured for tests:
// validation is disabled (so httptest loopback servers can be used) and the
// retry sleep is replaced with an immediate no-op.
func newTestWebhookChannel(t *testing.T, cfg WebhookChannelConfig) *WebhookChannel {
	t.Helper()
	ch := NewWebhookChannel(cfg)
	ch.validate = func(string) error { return nil }
	ch.sleep = func(ctx context.Context, _ time.Duration) error {
		return ctx.Err()
	}
	return ch
}

func notificationWithURL(url string) domain.Notification {
	return domain.Notification{
		ID:       "notif-1",
		UserID:   "prof-1",
		Source:   "heartbeat",
		Trigger:  "outcome_critical",
		Severity: domain.SeverityCritical,
		Title:    "Test",
		Metadata: map[string]string{"webhookURL": url},
	}
}

// TestWebhookChannel_SuccessfulDelivery verifies a 200 response completes in one attempt
// and the signature header is set when a secret is configured.
func TestWebhookChannel_SuccessfulDelivery(t *testing.T) {
	const secret = "test-secret"
	var attempts int32
	var gotSignature string
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		gotSignature = r.Header.Get(SignatureHeader)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := newTestWebhookChannel(t, WebhookChannelConfig{Secret: secret, MaxAttempts: 3})
	if err := ch.Send(context.Background(), notificationWithURL(srv.URL)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(gotBody)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSignature != want {
		t.Fatalf("signature header = %q, want %q", gotSignature, want)
	}

	// Sanity: body is the JSON-marshalled notification.
	var decoded domain.Notification
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("body not JSON notification: %v", err)
	}
	if decoded.ID != "notif-1" {
		t.Fatalf("decoded.ID = %q, want notif-1", decoded.ID)
	}
}

// TestWebhookChannel_TransientFailureThenSuccess verifies 5xx responses are retried
// and a subsequent 2xx response completes delivery.
func TestWebhookChannel_TransientFailureThenSuccess(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := newTestWebhookChannel(t, WebhookChannelConfig{MaxAttempts: 3})
	if err := ch.Send(context.Background(), notificationWithURL(srv.URL)); err != nil {
		t.Fatalf("Send after retries: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
}

// TestWebhookChannel_PermanentFailureNotRetried verifies that a 4xx response is
// treated as permanent and not retried.
func TestWebhookChannel_PermanentFailureNotRetried(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	ch := newTestWebhookChannel(t, WebhookChannelConfig{MaxAttempts: 5})
	err := ch.Send(context.Background(), notificationWithURL(srv.URL))
	if err == nil {
		t.Fatal("Send returned nil, want error")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1 (no retry on 4xx)", got)
	}
}

// TestWebhookChannel_ExhaustsRetriesOnPersistent5xx verifies that after MaxAttempts
// the error is returned and attempts stop.
func TestWebhookChannel_ExhaustsRetriesOnPersistent5xx(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	ch := newTestWebhookChannel(t, WebhookChannelConfig{MaxAttempts: 4})
	err := ch.Send(context.Background(), notificationWithURL(srv.URL))
	if err == nil {
		t.Fatal("Send returned nil, want error after exhausting retries")
	}
	if got := atomic.LoadInt32(&attempts); got != 4 {
		t.Fatalf("attempts = %d, want 4", got)
	}
}

// TestWebhookChannel_TimeoutIsRetried verifies that client-side timeouts are classified
// as transient and retried.
func TestWebhookChannel_TimeoutIsRetried(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			time.Sleep(100 * time.Millisecond)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 20 * time.Millisecond}
	ch := newTestWebhookChannel(t, WebhookChannelConfig{
		HTTPClient:  client,
		MaxAttempts: 3,
	})
	if err := ch.Send(context.Background(), notificationWithURL(srv.URL)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got < 2 {
		t.Fatalf("attempts = %d, want >= 2", got)
	}
}

// TestWebhookChannel_NoSecret_NoSignatureHeader verifies that when no secret is
// configured, no signature header is added.
func TestWebhookChannel_NoSecret_NoSignatureHeader(t *testing.T) {
	var gotSigHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSigHeader = r.Header.Get(SignatureHeader)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ch := newTestWebhookChannel(t, WebhookChannelConfig{MaxAttempts: 1})
	if err := ch.Send(context.Background(), notificationWithURL(srv.URL)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotSigHeader != "" {
		t.Fatalf("signature header = %q, want empty when no secret configured", gotSigHeader)
	}
}

// TestWebhookChannel_ContextCancelStopsRetry verifies a cancelled context aborts
// the retry loop rather than continuing to hammer the endpoint.
func TestWebhookChannel_ContextCancelStopsRetry(t *testing.T) {
	var attempts int32
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		cancel()
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ch := newTestWebhookChannel(t, WebhookChannelConfig{MaxAttempts: 5})
	// Replace sleep with a version that honours ctx cancellation.
	ch.sleep = func(c context.Context, _ time.Duration) error {
		return c.Err()
	}
	err := ch.Send(ctx, notificationWithURL(srv.URL))
	if err == nil {
		t.Fatal("Send returned nil, want ctx.Err() after cancel")
	}
	if got := atomic.LoadInt32(&attempts); got > 2 {
		t.Fatalf("attempts = %d, want <= 2 after cancel", got)
	}
}
