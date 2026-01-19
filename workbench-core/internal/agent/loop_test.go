package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/workbench-core/internal/types"
)

type fakeLLM struct {
	Replies []string
	i       int
}

func (f *fakeLLM) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	if f.i >= len(f.Replies) {
		return types.LLMResponse{Text: `{"op":"final","text":"no more replies"}`}, nil
	}
	out := types.LLMResponse{Text: f.Replies[f.i]}
	f.i++
	return out, nil
}

type fakeStreamingLLM struct {
	Chunks []string
	Final  string
}

func (f *fakeStreamingLLM) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	// Should not be used when streaming is supported.
	return types.LLMResponse{Text: f.Final}, nil
}

func (f *fakeStreamingLLM) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	for _, c := range f.Chunks {
		if cb != nil {
			if err := cb(types.LLMStreamChunk{Text: c}); err != nil {
				return types.LLMResponse{}, err
			}
		}
	}
	return types.LLMResponse{Text: f.Final}, nil
}

func TestAgentLoopV0_Run_ExecutesOpsUntilFinal(t *testing.T) {
	llm := &fakeLLM{
		Replies: []string{
			`{"op":"fs.list","path":"/tools"}`,
			`{"op":"final","text":"done"}`,
		},
	}

	var called []types.HostOpRequest
	exec := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		_ = ctx
		called = append(called, req)
		b, _ := json.Marshal(req)
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: string(b)}
	}

	a := &Agent{LLM: llm, Exec: HostExecFunc(exec), Model: "test-model", MaxSteps: 5}
	final, err := a.Run(context.Background(), "goal")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final != "done" {
		t.Fatalf("unexpected final %q", final)
	}
	if len(called) != 1 || called[0].Op != types.HostOpFSList || called[0].Path != "/tools" {
		t.Fatalf("unexpected calls: %+v", called)
	}
}

func TestAgentLoopV0_Run_StreamsFinalTextOnly(t *testing.T) {
	// Streamed JSON split across chunks; includes an escaped newline that must be decoded.
	finalJSON := `{"op":"final","text":"hello\nworld"}`
	llm := &fakeStreamingLLM{
		Chunks: []string{
			`{"op":"final","text":"hel`,
			`lo\nwo`,
			`rld"}`,
		},
		Final: finalJSON,
	}

	var streamed string
	a := &Agent{
		LLM:      llm,
		Exec:     HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse { _ = ctx; _ = req; return types.HostOpResponse{Ok: true} }),
		Model:    "test-model",
		MaxSteps: 2,
		Hooks: Hooks{
			OnToken: func(step int, text string) {
				_ = step
				streamed += text
			},
		},
	}

	final, err := a.Run(context.Background(), "goal")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final != "hello\nworld" {
		t.Fatalf("unexpected final %q", final)
	}
	if streamed != "hello\nworld" {
		t.Fatalf("unexpected streamed %q", streamed)
	}
}
