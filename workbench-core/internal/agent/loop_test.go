package agent

import (
	"context"
	"encoding/json"
	"testing"
	"strings"

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

func TestAgentLoopV0_RunConversation_BatchSequential_ExecutesOpsInOrder(t *testing.T) {
	llm := &fakeLLM{
		Replies: []string{
			`{"op":"batch","operations":[{"op":"fs.read","path":"/a"},{"op":"fs.read","path":"/b"}]}`,
			`{"op":"final","text":"done"}`,
		},
	}

	var called []types.HostOpRequest
	exec := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		_ = ctx
		called = append(called, req)
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: req.Path}
	}

	a := &Agent{LLM: llm, Exec: HostExecFunc(exec), Model: "test-model", MaxSteps: 5}
	final, msgs, _, err := a.RunConversation(context.Background(), []types.LLMMessage{{Role: "user", Content: "goal"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if final != "done" {
		t.Fatalf("unexpected final %q", final)
	}
	if len(called) != 2 {
		t.Fatalf("expected 2 calls, got %d (%+v)", len(called), called)
	}
	if called[0].Op != types.HostOpFSRead || called[0].Path != "/a" {
		t.Fatalf("unexpected call[0]: %+v", called[0])
	}
	if called[1].Op != types.HostOpFSRead || called[1].Path != "/b" {
		t.Fatalf("unexpected call[1]: %+v", called[1])
	}

	br := mustParseBatchResponse(t, msgs)
	if !br.Ok {
		t.Fatalf("expected batch ok=true, got false (resp=%+v)", br)
	}
	if len(br.Results) != 2 {
		t.Fatalf("expected 2 results, got %d (resp=%+v)", len(br.Results), br)
	}
	if br.Results[0].Op != types.HostOpFSRead || br.Results[0].Text != "/a" {
		t.Fatalf("unexpected result[0]: %+v", br.Results[0])
	}
	if br.Results[1].Op != types.HostOpFSRead || br.Results[1].Text != "/b" {
		t.Fatalf("unexpected result[1]: %+v", br.Results[1])
	}
}

func TestAgentLoopV0_RunConversation_BatchParallel_PreservesResultOrder(t *testing.T) {
	llm := &fakeLLM{
		Replies: []string{
			`{"op":"batch","parallel":true,"operations":[{"op":"fs.read","path":"/a"},{"op":"fs.read","path":"/b"}]}`,
			`{"op":"final","text":"done"}`,
		},
	}

	var called []types.HostOpRequest
	exec := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		_ = ctx
		called = append(called, req)
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: req.Path}
	}

	a := &Agent{LLM: llm, Exec: HostExecFunc(exec), Model: "test-model", MaxSteps: 5}
	final, msgs, _, err := a.RunConversation(context.Background(), []types.LLMMessage{{Role: "user", Content: "goal"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if final != "done" {
		t.Fatalf("unexpected final %q", final)
	}
	if len(called) != 2 {
		t.Fatalf("expected 2 calls, got %d (%+v)", len(called), called)
	}

	seen := map[string]bool{}
	for _, c := range called {
		seen[c.Path] = true
	}
	if !seen["/a"] || !seen["/b"] {
		t.Fatalf("expected calls to /a and /b, got %+v", called)
	}

	br := mustParseBatchResponse(t, msgs)
	if !br.Ok {
		t.Fatalf("expected batch ok=true, got false (resp=%+v)", br)
	}
	if len(br.Results) != 2 {
		t.Fatalf("expected 2 results, got %d (resp=%+v)", len(br.Results), br)
	}
	if br.Results[0].Text != "/a" {
		t.Fatalf("expected result[0].text /a, got %+v", br.Results[0])
	}
	if br.Results[1].Text != "/b" {
		t.Fatalf("expected result[1].text /b, got %+v", br.Results[1])
	}
}

func TestAgentLoopV0_RunConversation_InvalidBatch_DoesNotExecuteOps(t *testing.T) {
	llm := &fakeLLM{
		Replies: []string{
			`{"op":"batch","operations":[]}`,
			`{"op":"final","text":"done"}`,
		},
	}

	var called []types.HostOpRequest
	exec := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		_ = ctx
		called = append(called, req)
		return types.HostOpResponse{Op: req.Op, Ok: true}
	}

	a := &Agent{LLM: llm, Exec: HostExecFunc(exec), Model: "test-model", MaxSteps: 5}
	final, msgs, _, err := a.RunConversation(context.Background(), []types.LLMMessage{{Role: "user", Content: "goal"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if final != "done" {
		t.Fatalf("unexpected final %q", final)
	}
	if len(called) != 0 {
		t.Fatalf("expected no host calls, got %d (%+v)", len(called), called)
	}

	seenErr := false
	for _, m := range msgs {
		if m.Role == "user" && strings.HasPrefix(m.Content, "Your last JSON op was invalid:") {
			seenErr = true
			break
		}
	}
	if !seenErr {
		t.Fatalf("expected invalid-op error feedback in messages")
	}
}

func mustParseBatchResponse(t *testing.T, msgs []types.LLMMessage) types.HostOpBatchResponse {
	t.Helper()

	const prefix = "HostOpBatchResponse:\n"
	const suffix = "\n\nReturn the next HostOpRequest"

	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != "user" {
			continue
		}
		if !strings.HasPrefix(m.Content, prefix) {
			continue
		}
		rest := strings.TrimPrefix(m.Content, prefix)
		end := strings.Index(rest, suffix)
		if end < 0 {
			end = len(rest)
		}
		raw := strings.TrimSpace(rest[:end])
		var br types.HostOpBatchResponse
		if err := json.Unmarshal([]byte(raw), &br); err != nil {
			t.Fatalf("unmarshal HostOpBatchResponse: %v; raw=%q", err, raw)
		}
		return br
	}

	t.Fatalf("HostOpBatchResponse not found in messages")
	return types.HostOpBatchResponse{}
}
