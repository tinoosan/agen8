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
	"github.com/tinoosan/agen8/pkg/llm/types"
)

// RetryConfig controls retry behavior for LLM requests.
type RetryConfig struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	OnRetry      func(ctx context.Context, info RetryAttemptInfo)
}

// RetryAttemptInfo describes a retry that will be attempted.
type RetryAttemptInfo struct {
	Class      string
	Attempt    int
	Delay      time.Duration
	StatusCode int
	Code       string
	Message    string
}

// ErrorInfo classifies LLM provider/runtime errors for policy and UI diagnostics.
type ErrorInfo struct {
	Class      string
	Retryable  bool
	StatusCode int
	Code       string
	Message    string
	// ProviderDetail captures backend-specific detail text when available.
	ProviderDetail string
	// ProviderRoute identifies the backend route family (e.g. chatgpt_codex, openrouter).
	ProviderRoute string
}

func (c RetryConfig) WithDefaults() RetryConfig {
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
		Config:  cfg.WithDefaults(),
	}
}

func (c *RetryClient) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	if c == nil || c.Wrapped == nil {
		return types.LLMResponse{}, fmt.Errorf("retry client is nil")
	}

	cfg := c.Config.WithDefaults()
	if cfg.MaxRetries == 0 {
		return c.Wrapped.Generate(ctx, req)
	}

	for attempt := 0; ; attempt++ {
		resp, err := c.Wrapped.Generate(ctx, req)
		if err == nil {
			return resp, nil
		}
		info := ClassifyError(err)
		if !info.Retryable || attempt >= cfg.MaxRetries {
			return types.LLMResponse{}, err
		}
		delay := backoff(cfg.InitialDelay, cfg.Multiplier, cfg.MaxDelay, attempt+1)
		if cfg.OnRetry != nil {
			cfg.OnRetry(ctx, RetryAttemptInfo{
				Class:      info.Class,
				Attempt:    attempt + 1,
				Delay:      delay,
				StatusCode: info.StatusCode,
				Code:       info.Code,
				Message:    info.Message,
			})
		}
		if err := sleep(ctx, delay); err != nil {
			return types.LLMResponse{}, err
		}
	}
}

func (c *RetryClient) SupportsStreaming() bool {
	if c == nil || c.Wrapped == nil {
		return false
	}
	return c.Wrapped.SupportsStreaming()
}

func (c *RetryClient) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	if c == nil || c.Wrapped == nil {
		return types.LLMResponse{}, fmt.Errorf("retry client is nil")
	}
	if !c.SupportsStreaming() {
		return types.LLMResponse{}, fmt.Errorf("LLM client does not support streaming")
	}
	streaming, ok := c.Wrapped.(types.LLMClientStreaming)
	if !ok {
		return types.LLMResponse{}, fmt.Errorf("LLM client does not implement streaming")
	}

	cfg := c.Config.WithDefaults()
	if cfg.MaxRetries == 0 {
		return streaming.GenerateStream(ctx, req, cb)
	}

	for attempt := 0; ; attempt++ {
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
		info := ClassifyError(err)
		if emitted || !info.Retryable || attempt >= cfg.MaxRetries {
			return types.LLMResponse{}, err
		}
		delay := backoff(cfg.InitialDelay, cfg.Multiplier, cfg.MaxDelay, attempt+1)
		if cfg.OnRetry != nil {
			cfg.OnRetry(ctx, RetryAttemptInfo{
				Class:      info.Class,
				Attempt:    attempt + 1,
				Delay:      delay,
				StatusCode: info.StatusCode,
				Code:       info.Code,
				Message:    info.Message,
			})
		}
		if err := sleep(ctx, delay); err != nil {
			return types.LLMResponse{}, err
		}
	}
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
	return ClassifyError(err).Retryable
}

// ClassifyError returns normalized error metadata for retry policy and UI reporting.
func ClassifyError(err error) ErrorInfo {
	if err == nil {
		return ErrorInfo{}
	}
	errMsg := safeErrorString(err)
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return withProviderContext(ErrorInfo{Class: "timeout", Retryable: true, Message: errMsg}, errMsg)
	}
	if errors.Is(err, io.EOF) {
		return withProviderContext(ErrorInfo{Class: "network", Retryable: true, Message: errMsg}, errMsg)
	}
	if errors.Is(err, net.ErrClosed) {
		return withProviderContext(ErrorInfo{Class: "network", Retryable: true, Message: errMsg}, errMsg)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return withProviderContext(ErrorInfo{Class: "timeout", Retryable: true, Message: errMsg}, errMsg)
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return withProviderContext(classifyOpenAIError(apiErr), errMsg)
	}

	msg := strings.ToLower(errMsg)
	switch {
	case looksLikeUsageLimit(msg, ""):
		return withProviderContext(ErrorInfo{Class: "rate_limit", Retryable: true, Message: errMsg}, errMsg)
	case looksLikeQuota(msg, ""):
		return withProviderContext(ErrorInfo{Class: "quota", Retryable: false, Message: errMsg}, errMsg)
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "invalid api key"), strings.Contains(msg, "authentication"):
		return withProviderContext(ErrorInfo{Class: "auth", Retryable: false, Message: errMsg}, errMsg)
	case strings.Contains(msg, "forbidden"), strings.Contains(msg, "permission"):
		return withProviderContext(ErrorInfo{Class: "permission", Retryable: false, Message: errMsg}, errMsg)
	case strings.Contains(msg, "bad request"), strings.Contains(msg, "invalid request"), strings.Contains(msg, "unprocessable"):
		return withProviderContext(ErrorInfo{Class: "invalid_request", Retryable: false, Message: errMsg}, errMsg)
	case strings.Contains(msg, "timeout"):
		return withProviderContext(ErrorInfo{Class: "timeout", Retryable: true, Message: errMsg}, errMsg)
	case strings.Contains(msg, "rate limit"):
		return withProviderContext(ErrorInfo{Class: "rate_limit", Retryable: true, Message: errMsg}, errMsg)
	case strings.Contains(msg, "connection reset"):
		return withProviderContext(ErrorInfo{Class: "network", Retryable: true, Message: errMsg}, errMsg)
	case strings.Contains(msg, "connection refused"):
		return withProviderContext(ErrorInfo{Class: "network", Retryable: true, Message: errMsg}, errMsg)
	case strings.Contains(msg, "connection aborted"):
		return withProviderContext(ErrorInfo{Class: "network", Retryable: true, Message: errMsg}, errMsg)
	}
	return withProviderContext(ErrorInfo{Class: "unknown", Retryable: false, Message: errMsg}, errMsg)
}

func safeErrorString(err error) (out string) {
	if err == nil {
		return ""
	}
	defer func() {
		if recover() != nil {
			out = "unknown error"
		}
	}()
	return strings.TrimSpace(err.Error())
}

func classifyOpenAIError(apiErr *openai.Error) ErrorInfo {
	msg := strings.TrimSpace(apiErr.Message)
	code := strings.TrimSpace(apiErr.Code)
	msgLower := strings.ToLower(msg)
	info := ErrorInfo{
		StatusCode: apiErr.StatusCode,
		Code:       code,
		Message:    msg,
	}
	if strings.Contains(msgLower, "data policy") ||
		strings.Contains(msgLower, "free model publication") ||
		strings.Contains(msgLower, "settings/privacy") {
		info.Class = "policy"
		info.Retryable = false
		return info
	}
	if looksLikeUsageLimit(msgLower, strings.ToLower(code)) {
		info.Class = "rate_limit"
		info.Retryable = true
		return info
	}
	switch apiErr.StatusCode {
	case 402:
		info.Class = "quota"
		info.Retryable = false
		return info
	case 408:
		info.Class = "timeout"
		info.Retryable = true
		return info
	case 409:
		info.Class = "server"
		info.Retryable = true
		return info
	case 429:
		if looksLikeQuota(strings.ToLower(msg), strings.ToLower(code)) {
			info.Class = "quota"
			info.Retryable = false
			return info
		}
		info.Class = "rate_limit"
		info.Retryable = true
		return info
	case 401:
		info.Class = "auth"
		info.Retryable = false
		return info
	case 403:
		info.Class = "permission"
		info.Retryable = false
		return info
	case 400, 422:
		info.Class = "invalid_request"
		info.Retryable = false
		return info
	}
	if apiErr.StatusCode >= 500 {
		info.Class = "server"
		info.Retryable = true
		return info
	}
	if looksLikeQuota(msgLower, strings.ToLower(code)) {
		info.Class = "quota"
		info.Retryable = false
		return info
	}
	if strings.Contains(msgLower, "timeout") {
		info.Class = "timeout"
		info.Retryable = true
		return info
	}
	if strings.Contains(msgLower, "rate limit") {
		info.Class = "rate_limit"
		info.Retryable = true
		return info
	}
	info.Class = "unknown"
	info.Retryable = false
	return info
}

func looksLikeQuota(msgLower, codeLower string) bool {
	return strings.Contains(msgLower, "insufficient_quota") ||
		strings.Contains(msgLower, "insufficient quota") ||
		strings.Contains(msgLower, "quota exceeded") ||
		strings.Contains(msgLower, "out of credits") ||
		strings.Contains(msgLower, "payment required") ||
		strings.Contains(msgLower, "no credits") ||
		strings.Contains(codeLower, "insufficient_quota") ||
		strings.Contains(codeLower, "insufficient_credits")
}

func looksLikeUsageLimit(msgLower, codeLower string) bool {
	return strings.Contains(msgLower, "usage_limit_reached") ||
		strings.Contains(msgLower, "usage not included") ||
		strings.Contains(msgLower, "usage_not_included") ||
		strings.Contains(msgLower, "rate_limit_exceeded") ||
		strings.Contains(codeLower, "usage_limit_reached") ||
		strings.Contains(codeLower, "usage_not_included") ||
		strings.Contains(codeLower, "rate_limit_exceeded")
}

func withProviderContext(info ErrorInfo, message string) ErrorInfo {
	if strings.TrimSpace(info.ProviderDetail) == "" {
		info.ProviderDetail = extractProviderDetail(message)
	}
	if strings.TrimSpace(info.ProviderRoute) == "" {
		info.ProviderRoute = inferProviderRoute(message)
	}
	return info
}

func extractProviderDetail(message string) string {
	msg := strings.TrimSpace(message)
	if msg == "" {
		return ""
	}
	const marker = "provider_detail="
	idx := strings.Index(msg, marker)
	if idx < 0 {
		return ""
	}
	s := msg[idx+len(marker):]
	if end := strings.Index(s, ")"); end >= 0 {
		s = s[:end]
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"") && len(s) >= 2 {
		s = strings.Trim(s, "\"")
	}
	return strings.TrimSpace(s)
}

func inferProviderRoute(message string) string {
	msg := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(msg, "chatgpt.com/backend-api/codex"):
		return "chatgpt_codex"
	case strings.Contains(msg, "openrouter.ai"):
		return "openrouter"
	case strings.Contains(msg, "api.openai.com"):
		return "openai_api"
	default:
		return ""
	}
}
