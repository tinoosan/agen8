package llm

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/tinoosan/agen8/pkg/llm/types"
)

type fakeRetryLLM struct {
	failures int
	calls    int
	err      error
	out      types.LLMResponse
}

func (f *fakeRetryLLM) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	f.calls++
	if f.calls <= f.failures {
		return types.LLMResponse{}, f.err
	}
	return f.out, nil
}

func (f *fakeRetryLLM) SupportsStreaming() bool {
	return false
}

type fakeRetryStreamingLLM struct {
	failBeforeEmit bool
	failAfterEmit  bool
	calls          int
	err            error
	out            types.LLMResponse
}

func (f *fakeRetryStreamingLLM) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	return f.out, nil
}

func (f *fakeRetryStreamingLLM) SupportsStreaming() bool {
	return true
}

func (f *fakeRetryStreamingLLM) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	f.calls++
	if f.calls == 1 {
		if f.failBeforeEmit {
			return types.LLMResponse{}, f.err
		}
		if f.failAfterEmit {
			if cb != nil {
				_ = cb(types.LLMStreamChunk{Text: "x"})
			}
			return types.LLMResponse{}, f.err
		}
	}
	// Success path: emit something and return final.
	if cb != nil {
		_ = cb(types.LLMStreamChunk{Text: `{"op":"final","text":"ok"}`})
		_ = cb(types.LLMStreamChunk{Done: true})
	}
	return f.out, nil
}

func TestRetryClient_Generate_RetriesTransientOpenAIError(t *testing.T) {
	inner := &fakeRetryLLM{
		failures: 2,
		err:      &openai.Error{StatusCode: 429},
		out:      types.LLMResponse{Text: `{"op":"final","text":"ok"}`},
	}
	r := NewRetryClient(inner, RetryConfig{
		MaxRetries:   3,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
		Multiplier:   2,
	})

	resp, err := r.Generate(context.Background(), types.LLMRequest{Model: "test", Messages: []types.LLMMessage{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text == "" {
		t.Fatalf("expected non-empty response")
	}
	if inner.calls != 3 {
		t.Fatalf("expected 3 calls, got %d", inner.calls)
	}
}

func TestRetryClient_Generate_DoesNotRetryNonRetryableError(t *testing.T) {
	inner := &fakeRetryLLM{
		failures: 5,
		err:      &openai.Error{StatusCode: 400},
		out:      types.LLMResponse{Text: `{"op":"final","text":"ok"}`},
	}
	r := NewRetryClient(inner, RetryConfig{
		MaxRetries:   3,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
		Multiplier:   2,
	})

	_, err := r.Generate(context.Background(), types.LLMRequest{Model: "test", Messages: []types.LLMMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected error")
	}
	if inner.calls != 1 {
		t.Fatalf("expected 1 call, got %d", inner.calls)
	}
}

func TestRetryClient_Generate_DoesNotRetryQuota429(t *testing.T) {
	inner := &fakeRetryLLM{
		failures: 5,
		err:      &openai.Error{StatusCode: 429, Code: "insufficient_quota", Message: "insufficient quota"},
		out:      types.LLMResponse{Text: `{"op":"final","text":"ok"}`},
	}
	r := NewRetryClient(inner, RetryConfig{
		MaxRetries:   3,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
		Multiplier:   2,
	})

	_, err := r.Generate(context.Background(), types.LLMRequest{Model: "test", Messages: []types.LLMMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected error")
	}
	if inner.calls != 1 {
		t.Fatalf("expected 1 call, got %d", inner.calls)
	}
}

func TestRetryClient_GenerateStream_RetriesOnlyBeforeOutputEmitted(t *testing.T) {
	inner := &fakeRetryStreamingLLM{
		failBeforeEmit: true,
		err:            &openai.Error{StatusCode: 429},
		out:            types.LLMResponse{Text: `{"op":"final","text":"ok"}`},
	}
	r := NewRetryClient(inner, RetryConfig{
		MaxRetries:   2,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
		Multiplier:   2,
	})

	resp, err := r.GenerateStream(context.Background(), types.LLMRequest{Model: "test", Messages: []types.LLMMessage{{Role: "user", Content: "hi"}}}, func(chunk types.LLMStreamChunk) error {
		_ = chunk
		return nil
	})
	if err != nil {
		t.Fatalf("GenerateStream: %v", err)
	}
	if resp.Text == "" {
		t.Fatalf("expected non-empty response")
	}
	if inner.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", inner.calls)
	}

	inner2 := &fakeRetryStreamingLLM{
		failAfterEmit: true,
		err:           &openai.Error{StatusCode: 429},
		out:           types.LLMResponse{Text: `{"op":"final","text":"ok"}`},
	}
	r2 := NewRetryClient(inner2, RetryConfig{
		MaxRetries:   2,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
		Multiplier:   2,
	})
	_, err = r2.GenerateStream(context.Background(), types.LLMRequest{Model: "test", Messages: []types.LLMMessage{{Role: "user", Content: "hi"}}}, func(chunk types.LLMStreamChunk) error {
		_ = chunk
		return nil
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if inner2.calls != 1 {
		t.Fatalf("expected 1 call (no retry after output), got %d", inner2.calls)
	}
}

func TestRetryClient_Generate_DoesNotRetrySchemaRejectionSignals(t *testing.T) {
	inner := &fakeRetryLLM{
		failures: 5,
		err:      fmt.Errorf("provider rejected response_format json_schema"),
		out:      types.LLMResponse{Text: `{"op":"final","text":"ok"}`},
	}
	r := NewRetryClient(inner, RetryConfig{
		MaxRetries:   3,
		InitialDelay: time.Nanosecond,
		MaxDelay:     time.Nanosecond,
		Multiplier:   2,
	})
	_, err := r.Generate(context.Background(), types.LLMRequest{Model: "test", Messages: []types.LLMMessage{{Role: "user", Content: "hi"}}})
	if err == nil {
		t.Fatalf("expected error")
	}
	if inner.calls != 1 {
		t.Fatalf("expected 1 call, got %d", inner.calls)
	}
}

func TestClassifyError_OpenAIStatusClassification(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		class     string
		retryable bool
	}{
		{
			name:      "rate-limit 429",
			err:       &openai.Error{StatusCode: 429, Message: "rate limit"},
			class:     "rate_limit",
			retryable: true,
		},
		{
			name:      "quota 429",
			err:       &openai.Error{StatusCode: 429, Code: "insufficient_quota", Message: "insufficient quota"},
			class:     "quota",
			retryable: false,
		},
		{
			name:      "auth 401",
			err:       &openai.Error{StatusCode: 401, Message: "invalid api key"},
			class:     "auth",
			retryable: false,
		},
		{
			name:      "payment required 402",
			err:       &openai.Error{StatusCode: 402, Message: "payment required"},
			class:     "quota",
			retryable: false,
		},
		{
			name:      "permission 403",
			err:       &openai.Error{StatusCode: 403, Message: "forbidden"},
			class:     "permission",
			retryable: false,
		},
		{
			name:      "invalid request 400",
			err:       &openai.Error{StatusCode: 400, Message: "bad request"},
			class:     "invalid_request",
			retryable: false,
		},
		{
			name:      "openrouter data policy",
			err:       &openai.Error{StatusCode: 404, Message: "No endpoints found matching your data policy (Free model publication). Configure: https://openrouter.ai/settings/privacy"},
			class:     "policy",
			retryable: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got.Class != tt.class {
				t.Fatalf("class = %q, want %q", got.Class, tt.class)
			}
			if got.Retryable != tt.retryable {
				t.Fatalf("retryable = %t, want %t", got.Retryable, tt.retryable)
			}
		})
	}
}

func TestClassifyError_NetworkAndTimeout(t *testing.T) {
	timeoutErr := &net.DNSError{IsTimeout: true}
	got := ClassifyError(timeoutErr)
	if got.Class != "timeout" || !got.Retryable {
		t.Fatalf("timeout classify = %+v", got)
	}

	got = ClassifyError(fmt.Errorf("connection refused by host"))
	if got.Class != "network" || !got.Retryable {
		t.Fatalf("network classify = %+v", got)
	}
}

func TestClassifyError_UsageLimitMarkersAreRateLimit(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "openai code usage_limit_reached", err: &openai.Error{StatusCode: 429, Code: "usage_limit_reached", Message: "usage_limit_reached"}},
		{name: "openai code usage_not_included", err: &openai.Error{StatusCode: 429, Code: "usage_not_included", Message: "usage not included for this account"}},
		{name: "generic message rate_limit_exceeded", err: fmt.Errorf("provider returned rate_limit_exceeded")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ClassifyError(tt.err)
			if info.Class != "rate_limit" {
				t.Fatalf("class=%q, want rate_limit", info.Class)
			}
			if !info.Retryable {
				t.Fatalf("retryable=false, want true")
			}
		})
	}
}

func TestClassifyError_ExtractsProviderDetailAndRoute(t *testing.T) {
	err := fmt.Errorf(`POST "https://chatgpt.com/backend-api/codex/responses": 400 Bad Request (provider_detail=Unsupported parameter: max_output_tokens)`)
	info := ClassifyError(err)
	if info.ProviderRoute != "chatgpt_codex" {
		t.Fatalf("provider route = %q, want chatgpt_codex", info.ProviderRoute)
	}
	if info.ProviderDetail != "Unsupported parameter: max_output_tokens" {
		t.Fatalf("provider detail = %q", info.ProviderDetail)
	}
}
