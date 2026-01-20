package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/tinoosan/workbench-core/internal/types"
)

// RetryConfig controls retry behavior for LLM requests.
type RetryConfig struct {
	// MaxRetries is the number of retries after the initial attempt.
	// If 0, no retries are performed.
	MaxRetries int

	// InitialDelay is the delay before the first retry attempt.
	InitialDelay time.Duration

	// MaxDelay caps the backoff delay. If 0, a default is used.
	MaxDelay time.Duration

	// Multiplier controls exponential backoff growth (e.g. 2.0 doubles each attempt).
	// If <= 1, a default is used.
	Multiplier float64
}

func (c RetryConfig) withDefaults() RetryConfig {
	if c.MaxRetries < 0 {
		c.MaxRetries = 0
	}
	if c.InitialDelay <= 0 {
		c.InitialDelay = 250 * time.Millisecond
	}
	if c.MaxDelay <= 0 {
		c.MaxDelay = 4 * time.Second
	}
	if c.Multiplier <= 1 {
		c.Multiplier = 2.0
	}
	return c
}

// RetryClient wraps an LLMClient and retries transient failures with exponential backoff.
//
// Notes:
// - This wrapper only retries *transient* failures (rate limits, timeouts, network errors).
// - It intentionally does NOT retry structured-output schema rejection errors; the underlying
//   OpenAI client already falls back from json_schema to JSON-object mode when appropriate.
type RetryClient struct {
	Wrapped types.LLMClient
	Config  RetryConfig
}

func NewRetryClient(wrapped types.LLMClient, cfg RetryConfig) *RetryClient {
	return &RetryClient{Wrapped: wrapped, Config: cfg.withDefaults()}
}

func (r *RetryClient) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	if r == nil || r.Wrapped == nil {
		return types.LLMResponse{}, fmt.Errorf("retry client wrapped LLM is nil")
	}
	cfg := r.Config.withDefaults()

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			return types.LLMResponse{}, ctx.Err()
		}

		resp, err := r.Wrapped.Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
		if !isRetryableLLMError(err) {
			return types.LLMResponse{}, err
		}
		lastErr = err

		if attempt == cfg.MaxRetries {
			break
		}

		if err := sleepWithContext(ctx, delay); err != nil {
			return types.LLMResponse{}, err
		}
		delay = nextDelay(delay, cfg.Multiplier, cfg.MaxDelay)
	}

	return types.LLMResponse{}, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// GenerateStream retries streaming requests only if the stream fails before any non-reasoning
// text chunk is emitted (to avoid duplicated streamed output in the UI).
func (r *RetryClient) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	if r == nil || r.Wrapped == nil {
		return types.LLMResponse{}, fmt.Errorf("retry client wrapped LLM is nil")
	}
	cfg := r.Config.withDefaults()

	s, ok := r.Wrapped.(types.LLMClientStreaming)
	if !ok {
		// Fallback: run a non-streaming request (with retries) and emit a single chunk.
		resp, err := r.Generate(ctx, req)
		if err != nil {
			return types.LLMResponse{}, err
		}
		if cb != nil {
			if err := cb(types.LLMStreamChunk{Text: resp.Text}); err != nil {
				return types.LLMResponse{}, err
			}
			if err := cb(types.LLMStreamChunk{Done: true}); err != nil {
				return types.LLMResponse{}, err
			}
		}
		return resp, nil
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			return types.LLMResponse{}, ctx.Err()
		}

		sawNonReasoningText := false
		wrappedCB := cb
		if cb != nil {
			wrappedCB = func(chunk types.LLMStreamChunk) error {
				// Do not TrimSpace: providers may legitimately stream single-space deltas,
				// and higher layers (agent JSON decoder) can depend on receiving them.
				if !chunk.IsReasoning && chunk.Text != "" {
					sawNonReasoningText = true
				}
				return cb(chunk)
			}
		}

		resp, err := s.GenerateStream(ctx, req, wrappedCB)
		if err == nil {
			return resp, nil
		}
		// If we already emitted any visible content, do not retry (would duplicate output).
		if sawNonReasoningText {
			return types.LLMResponse{}, err
		}
		if !isRetryableLLMError(err) {
			return types.LLMResponse{}, err
		}
		lastErr = err

		if attempt == cfg.MaxRetries {
			break
		}

		if err := sleepWithContext(ctx, delay); err != nil {
			return types.LLMResponse{}, err
		}
		delay = nextDelay(delay, cfg.Multiplier, cfg.MaxDelay)
	}

	return types.LLMResponse{}, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func nextDelay(cur time.Duration, mult float64, max time.Duration) time.Duration {
	if cur <= 0 {
		cur = 250 * time.Millisecond
	}
	if mult <= 1 {
		mult = 2
	}
	// Guard against overflow.
	n := time.Duration(math.Round(float64(cur) * mult))
	if n <= 0 || n < cur {
		n = cur
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()

	if ctx == nil {
		<-t.C
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func isRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	// Never retry context cancellation/deadline.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// OpenAI-compatible API errors.
	var apierr *openai.Error
	if errors.As(err, &apierr) && apierr != nil {
		switch apierr.StatusCode {
		case 408, 409, 425, 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}

	// Do not retry schema/structured output incompatibility errors. Underlying client
	// should handle schema fallbacks deterministically without retry storms.
	if isSchemaRejectionError(err) {
		return false
	}

	// Network timeouts.
	var ne net.Error
	if errors.As(err, &ne) && ne != nil {
		if ne.Timeout() {
			return true
		}
		// Many transient network errors do not mark Timeout=true; fall through to string checks.
	}

	// EOF is commonly returned on transient connection drops.
	if errors.Is(err, io.EOF) {
		return true
	}

	// Conservative string matching for common transient network failures.
	s := strings.ToLower(safeErrorString(err))
	switch {
	case strings.Contains(s, "rate limit"),
		strings.Contains(s, "too many requests"),
		strings.Contains(s, "timeout"),
		strings.Contains(s, "timed out"),
		strings.Contains(s, "temporary failure"),
		strings.Contains(s, "connection reset"),
		strings.Contains(s, "broken pipe"),
		strings.Contains(s, "connection refused"),
		strings.Contains(s, "tls handshake timeout"),
		strings.Contains(s, "server closed idle connection"),
		strings.Contains(s, "unexpected eof"):
		return true
	default:
		return false
	}
}

func isSchemaRejectionError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(safeErrorString(err))
	// Mirror the broad signals used by the OpenAI client fallback logic.
	return strings.Contains(s, "json_schema") ||
		strings.Contains(s, "response_format") ||
		strings.Contains(s, "structured")
}

func safeErrorString(err error) (out string) {
	if err == nil {
		return ""
	}
	defer func() {
		if recover() != nil {
			out = ""
		}
	}()
	return err.Error()
}

