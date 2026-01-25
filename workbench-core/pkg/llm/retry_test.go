package llm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/tinoosan/workbench-core/pkg/llm/types"
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
