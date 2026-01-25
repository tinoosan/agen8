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
	"github.com/tinoosan/workbench-core/pkg/llm/types"
)

// RetryConfig controls retry behavior for LLM requests.
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
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
type RetryClient struct {
	Wrapped types.LLMClient
	Config  RetryConfig
}

func NewRetryClient(client types.LLMClient, cfg RetryConfig) *RetryClient {
	if client == nil {
		return nil
	}
	return &RetryClient{
		Wrapped: client,
		Config:  cfg.withDefaults(),
	}
}

func (c *RetryClient) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	if c == nil || c.Wrapped == nil {
		return types.LLMResponse{}, fmt.Errorf("retry client is nil")
	}

	cfg := c.Config.withDefaults()
	if cfg.MaxRetries == 0 {
		return c.Wrapped.Generate(ctx, req)
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := backoff(cfg.InitialDelay, cfg.Multiplier, cfg.MaxDelay, attempt)
			if err := sleep(ctx, delay); err != nil {
				return types.LLMResponse{}, err
			}
		}

		resp, err := c.Wrapped.Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !isTransient(err) {
			return types.LLMResponse{}, err
		}
	}

	return types.LLMResponse{}, lastErr
}

func (c *RetryClient) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	if c == nil || c.Wrapped == nil {
		return types.LLMResponse{}, fmt.Errorf("retry client is nil")
	}
	streaming, ok := c.Wrapped.(types.LLMClientStreaming)
	if !ok {
		return types.LLMResponse{}, fmt.Errorf("LLM client does not support streaming")
	}

	cfg := c.Config.withDefaults()
	if cfg.MaxRetries == 0 {
		return streaming.GenerateStream(ctx, req, cb)
	}

	var lastErr error
	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := backoff(cfg.InitialDelay, cfg.Multiplier, cfg.MaxDelay, attempt)
			if err := sleep(ctx, delay); err != nil {
				return types.LLMResponse{}, err
			}
		}

		emitted := false
		wrappedCb := func(chunk types.LLMStreamChunk) error {
			if chunk.Text != "" || chunk.Done || chunk.IsReasoning {
				emitted = true
			}
			if cb != nil {
				return cb(chunk)
			}
			return nil
		}

		resp, err := streaming.GenerateStream(ctx, req, wrappedCb)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if emitted || !isTransient(err) {
			return types.LLMResponse{}, err
		}
	}

	return types.LLMResponse{}, lastErr
}

func backoff(initial time.Duration, mult float64, max time.Duration, attempt int) time.Duration {
	delay := float64(initial) * math.Pow(mult, float64(attempt-1))
	if delay > float64(max) {
		delay = float64(max)
	}
	return time.Duration(delay)
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode >= 500 || apiErr.StatusCode == 429 {
			return true
		}
		if apiErr.StatusCode == 408 {
			return true
		}
		msg := strings.ToLower(strings.TrimSpace(apiErr.Message))
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "rate limit") {
			return true
		}
		return false
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"):
		return true
	case strings.Contains(msg, "rate limit"):
		return true
	case strings.Contains(msg, "connection reset"):
		return true
	case strings.Contains(msg, "connection refused"):
		return true
	case strings.Contains(msg, "connection aborted"):
		return true
	}
	return false
}
