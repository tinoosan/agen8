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

type fakeLLMChaining struct {
	Replies []string
	IDs     []string

	SeenPreviousResponseIDs []string
	i                      int
}

func (f *fakeLLMChaining) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	_ = ctx
	f.SeenPreviousResponseIDs = append(f.SeenPreviousResponseIDs, req.PreviousResponseID)

	if f.i >= len(f.Replies) {
		return types.LLMResponse{Text: `{"op":"final","text":"no more replies"}`}, nil
	}
	out := types.LLMResponse{Text: f.Replies[f.i]}
	if f.i < len(f.IDs) {
		out.ResponseID = f.IDs[f.i]
	}
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

type fakeStreamingLLMChunks struct {
	Chunks []types.LLMStreamChunk
	Final  string
}

func (f *fakeStreamingLLMChunks) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	return types.LLMResponse{Text: f.Final}, nil
}

func (f *fakeStreamingLLMChunks) GenerateStream(ctx context.Context, req types.LLMRequest, cb types.LLMStreamCallback) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	for _, c := range f.Chunks {
		if cb != nil {
			if err := cb(c); err != nil {
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

func TestAgentLoopV0_Run_PropagatesPreviousResponseIDAcrossSteps(t *testing.T) {
	llm := &fakeLLMChaining{
		Replies: []string{
			`{"op":"fs.list","path":"/tools"}`,
			`{"op":"final","text":"done"}`,
		},
		IDs: []string{"resp_1", "resp_2"},
	}

	exec := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		_ = ctx
		_ = req
		return types.HostOpResponse{Ok: true}
	}

	a := &Agent{LLM: llm, Exec: HostExecFunc(exec), Model: "test-model", MaxSteps: 5}
	final, err := a.Run(context.Background(), "goal")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final != "done" {
		t.Fatalf("unexpected final %q", final)
	}
	if len(llm.SeenPreviousResponseIDs) < 2 {
		t.Fatalf("expected at least 2 LLM calls, got %d", len(llm.SeenPreviousResponseIDs))
	}
	if llm.SeenPreviousResponseIDs[0] != "" {
		t.Fatalf("expected first PreviousResponseID empty, got %q", llm.SeenPreviousResponseIDs[0])
	}
	if llm.SeenPreviousResponseIDs[1] != "resp_1" {
		t.Fatalf("expected second PreviousResponseID resp_1, got %q", llm.SeenPreviousResponseIDs[1])
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
		LLM: llm,
		Exec: HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
			_ = ctx
			_ = req
			return types.HostOpResponse{Ok: true}
		}),
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

func TestAgentLoopV0_Run_ForwardsReasoningToHookAndDoesNotAffectOutput(t *testing.T) {
	finalJSON := `{"op":"final","text":"hello world"}`
	llm := &fakeStreamingLLMChunks{
		Chunks: []types.LLMStreamChunk{
			{IsReasoning: true, Text: "short summary"},
			{Text: `{"op":"final","text":"hello`},
			{Text: ` world"}`},
			{Done: true},
		},
		Final: finalJSON,
	}

	var streamed string
	var reasoning []types.LLMStreamChunk
	a := &Agent{
		LLM: llm,
		Exec: HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
			_ = ctx
			_ = req
			return types.HostOpResponse{Ok: true}
		}),
		Model:    "test-model",
		MaxSteps: 2,
		Hooks: Hooks{
			OnToken: func(step int, text string) {
				_ = step
				streamed += text
			},
			OnStreamChunk: func(step int, chunk types.LLMStreamChunk) {
				_ = step
				reasoning = append(reasoning, chunk)
			},
		},
	}

	final, err := a.Run(context.Background(), "goal")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final != "hello world" {
		t.Fatalf("unexpected final %q", final)
	}
	if streamed != "hello world" {
		t.Fatalf("unexpected streamed %q", streamed)
	}
	if len(reasoning) == 0 {
		t.Fatalf("expected reasoning chunks")
	}
	// Ensure we saw a reasoning chunk and a Done sentinel.
	seenReasoning := false
	seenDone := false
	for _, c := range reasoning {
		if c.IsReasoning {
			seenReasoning = true
		}
		if c.Done {
			seenDone = true
		}
	}
	if !seenReasoning {
		t.Fatalf("expected a reasoning chunk, got %+v", reasoning)
	}
	if !seenDone {
		t.Fatalf("expected a done chunk, got %+v", reasoning)
	}
}
