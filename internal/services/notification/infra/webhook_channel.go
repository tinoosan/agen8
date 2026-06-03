package infra

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/notification/domain"
)

// SignatureHeader is the HTTP header containing the HMAC-SHA256 signature of the
// request body. Receivers verify authenticity by recomputing HMAC-SHA256(body, secret)
// and comparing to the header value (format: "sha256=<hex>").
const SignatureHeader = "X-Agen8-Signature"

const (
	defaultMaxAttempts  = 3
	defaultInitialDelay = 200 * time.Millisecond
	defaultMaxDelay     = 5 * time.Second
)

// validateWebhookURL rejects URLs that could cause SSRF attacks:
// private/loopback IPs, non-HTTP(S) schemes, and invalid URLs.
func validateWebhookURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("webhook URL is empty")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use http or https scheme, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "localhost" {
		return fmt.Errorf("webhook URL must not target localhost")
	}
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("webhook URL must not target private/loopback address %s", host)
		}
	}
	// Also check if hostname resolves to a private IP
	if ip == nil {
		addrs, err := net.LookupIP(host)
		if err == nil {
			for _, addr := range addrs {
				if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() {
					return fmt.Errorf("webhook URL hostname %s resolves to private address %s", host, addr)
				}
			}
		}
	}
	return nil
}

// WebhookChannelConfig configures a WebhookChannel. Zero values fall back to
// sensible defaults. Secret, if non-empty, enables HMAC-SHA256 signing of outgoing
// requests so receivers can verify authenticity.
type WebhookChannelConfig struct {
	HTTPClient   *http.Client
	Secret       string
	MaxAttempts  int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Logger       *slog.Logger
}

// WebhookChannel delivers notifications by POSTing JSON to a configured URL.
// The webhook URL is read from the notification's Metadata["webhookURL"] field,
// which is set by the operator's rule configuration.
//
// Delivery is retried with exponential backoff on transient transport errors and
// 5xx responses. 4xx responses are treated as permanent and not retried. When a
// signing secret is configured, each request carries an HMAC-SHA256 signature of
// the body in the header named by SignatureHeader.
type WebhookChannel struct {
	client       *http.Client
	secret       string
	maxAttempts  int
	initialDelay time.Duration
	maxDelay     time.Duration
	logger       *slog.Logger

	// Test hooks. Unexported so they remain package-private.
	validate func(string) error
	sleep    func(context.Context, time.Duration) error
}

var _ domain.NotificationChannel = (*WebhookChannel)(nil)

// NewWebhookChannel creates a new webhook notification channel from the given
// configuration. Zero values in the config are replaced with defaults.
func NewWebhookChannel(cfg WebhookChannelConfig) *WebhookChannel {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
	initial := cfg.InitialDelay
	if initial <= 0 {
		initial = defaultInitialDelay
	}
	maxDelay := cfg.MaxDelay
	if maxDelay <= 0 {
		maxDelay = defaultMaxDelay
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &WebhookChannel{
		client:       client,
		secret:       cfg.Secret,
		maxAttempts:  maxAttempts,
		initialDelay: initial,
		maxDelay:     maxDelay,
		logger:       logger,
		validate:     validateWebhookURL,
		sleep:        sleepCtx,
	}
}

// Type returns the channel identifier.
func (c *WebhookChannel) Type() string { return "webhook" }

// Send POSTs the notification as JSON to the webhook URL in the notification
// metadata. Transient failures (network errors, 5xx) are retried with exponential
// backoff up to MaxAttempts times. Permanent failures (4xx) are returned
// immediately. Missing webhookURL metadata is a configuration error and returns
// an error rather than silently skipping.
func (c *WebhookChannel) Send(ctx context.Context, n domain.Notification) error {
	webhookURL := n.Metadata["webhookURL"]
	if webhookURL == "" {
		// No webhook URL configured for this notification (e.g. the matching rule
		// lists "webhook" in Channels but has no WebhookURL). Nothing to deliver.
		return nil
	}
	if err := c.validate(webhookURL); err != nil {
		return fmt.Errorf("webhook blocked: %w", err)
	}

	payload, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("webhook marshal: %w", err)
	}

	var signature string
	if c.secret != "" {
		mac := hmac.New(sha256.New, []byte(c.secret))
		mac.Write(payload)
		signature = "sha256=" + hex.EncodeToString(mac.Sum(nil))
	}

	var lastErr error
	for attempt := 1; attempt <= c.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(payload))
		if err != nil {
			return fmt.Errorf("webhook request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "agen8-notification/1.0")
		if signature != "" {
			req.Header.Set(SignatureHeader, signature)
		}

		resp, respErr := c.client.Do(req)
		if respErr != nil {
			lastErr = fmt.Errorf("webhook send: %w", respErr)
			if attempt == c.maxAttempts {
				c.logger.Error("webhook delivery failed after retries",
					"notificationId", n.ID,
					"url", webhookURL,
					"attempts", attempt,
					"error", respErr)
				return lastErr
			}
			c.logger.Warn("webhook delivery transient error, will retry",
				"notificationId", n.ID,
				"attempt", attempt,
				"error", respErr)
		} else {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 400 {
				return nil
			}
			lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
			// 4xx is permanent — do not retry.
			if resp.StatusCode < 500 {
				c.logger.Error("webhook delivery failed (permanent)",
					"notificationId", n.ID,
					"url", webhookURL,
					"status", resp.StatusCode)
				return lastErr
			}
			if attempt == c.maxAttempts {
				c.logger.Error("webhook delivery failed after retries",
					"notificationId", n.ID,
					"url", webhookURL,
					"attempts", attempt,
					"status", resp.StatusCode)
				return lastErr
			}
			c.logger.Warn("webhook delivery 5xx, will retry",
				"notificationId", n.ID,
				"attempt", attempt,
				"status", resp.StatusCode)
		}

		delay := c.initialDelay * (1 << (attempt - 1))
		if delay > c.maxDelay {
			delay = c.maxDelay
		}
		if err := c.sleep(ctx, delay); err != nil {
			return fmt.Errorf("webhook send interrupted: %w", err)
		}
	}
	return lastErr
}

// sleepCtx sleeps for d or until ctx is cancelled, whichever comes first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
