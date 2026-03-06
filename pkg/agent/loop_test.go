package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

type scriptedStreamingLLM struct {
	responses []llmtypes.LLMResponse
	index     int
	lastReq   llmtypes.LLMRequest
}

func (s *scriptedStreamingLLM) Generate(context.Context, llmtypes.LLMRequest) (llmtypes.LLMResponse, error) {
	return llmtypes.LLMResponse{}, nil
}

func (s *scriptedStreamingLLM) SupportsStreaming() bool { return true }

func (s *scriptedStreamingLLM) GenerateStream(_ context.Context, req llmtypes.LLMRequest, _ llmtypes.LLMStreamCallback) (llmtypes.LLMResponse, error) {
	s.lastReq = req
	if len(s.responses) == 0 {
		return llmtypes.LLMResponse{}, nil
	}
	if s.index >= len(s.responses) {
		return s.responses[len(s.responses)-1], nil
	}
	resp := s.responses[s.index]
	s.index++
	return resp, nil
}

type fixedToolRegistry struct {
	dispatch func(ctx context.Context, name string, args json.RawMessage) (types.HostOpRequest, error)
}

func (f *fixedToolRegistry) Definitions() []llmtypes.Tool {
	return []llmtypes.Tool{
		{
			Type: "function",
			Function: llmtypes.ToolFunction{
				Name: "task_create",
			},
		},
	}
}

func (f *fixedToolRegistry) Dispatch(ctx context.Context, name string, args json.RawMessage) (types.HostOpRequest, error) {
	if f.dispatch != nil {
		return f.dispatch(ctx, name, args)
	}
	return types.HostOpRequest{Op: types.HostOpNoop}, nil
}

type fixedHostExecutor struct {
	resp types.HostOpResponse
}

func (f fixedHostExecutor) Exec(context.Context, types.HostOpRequest) types.HostOpResponse {
	return f.resp
}

func TestDefaultAgent_RunConversation_RepeatedInvalidToolCallsTripCircuitBreaker(t *testing.T) {
	prevThreshold := repeatedInvalidToolCallThreshold
	repeatedInvalidToolCallThreshold = 3
	t.Cleanup(func() {
		repeatedInvalidToolCallThreshold = prevThreshold
	})

	llm := &scriptedStreamingLLM{
		responses: []llmtypes.LLMResponse{
			{
				ToolCalls: []llmtypes.ToolCall{
					{
						ID:   "tc-1",
						Type: "function",
						Function: llmtypes.ToolCallFunction{
							Name:      "task_create",
							Arguments: `{"goal":"delegate without role"}`,
						},
					},
				},
			},
		},
	}
	reg := &fixedToolRegistry{
		dispatch: func(_ context.Context, _ string, _ json.RawMessage) (types.HostOpRequest, error) {
			return types.HostOpRequest{}, errors.New("task_create.assignedRole is required for coordinators in team mode")
		},
	}
	a := &DefaultAgent{
		LLM:          llm,
		Exec:         fixedHostExecutor{resp: types.HostOpResponse{Ok: true}},
		Model:        "test-model",
		ToolRegistry: reg,
	}

	_, _, _, err := a.RunConversation(context.Background(), []llmtypes.LLMMessage{{Role: "user", Content: "delegate"}})
	if err == nil {
		t.Fatalf("expected repeated invalid tool-call error")
	}
	var typedErr *RepeatedInvalidToolCallError
	if !errors.As(err, &typedErr) {
		t.Fatalf("expected RepeatedInvalidToolCallError, got %T (%v)", err, err)
	}
	if !typedErr.Coordinator {
		t.Fatalf("expected coordinator hint to be set")
	}
	if typedErr.Count != 3 {
		t.Fatalf("typedErr.Count=%d, want 3", typedErr.Count)
	}
}

func TestDefaultAgent_RunConversation_SuccessfulToolProgressStillCompletes(t *testing.T) {
	prevThreshold := repeatedInvalidToolCallThreshold
	repeatedInvalidToolCallThreshold = 3
	t.Cleanup(func() {
		repeatedInvalidToolCallThreshold = prevThreshold
	})

	llm := &scriptedStreamingLLM{
		responses: []llmtypes.LLMResponse{
			{
				ToolCalls: []llmtypes.ToolCall{
					{
						ID:   "tc-1",
						Type: "function",
						Function: llmtypes.ToolCallFunction{
							Name:      "task_create",
							Arguments: `{"goal":"delegate"}`,
						},
					},
				},
			},
			{
				ToolCalls: []llmtypes.ToolCall{
					{
						ID:   "tc-2",
						Type: "function",
						Function: llmtypes.ToolCallFunction{
							Name:      "final_answer",
							Arguments: `{"text":"done","status":"succeeded","error":"","artifacts":[]}`,
						},
					},
				},
			},
		},
	}
	a := &DefaultAgent{
		LLM:          llm,
		Exec:         fixedHostExecutor{resp: types.HostOpResponse{Ok: true}},
		Model:        "test-model",
		ToolRegistry: &fixedToolRegistry{},
	}

	res, _, _, err := a.RunConversation(context.Background(), []llmtypes.LLMMessage{{Role: "user", Content: "delegate"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if res.Status != types.TaskStatusSucceeded {
		t.Fatalf("status=%q, want %q", res.Status, types.TaskStatusSucceeded)
	}
	if res.Text != "done" {
		t.Fatalf("text=%q, want %q", res.Text, "done")
	}
}

func TestDefaultAgent_RunConversation_EmptyPromptSourceFallsBackToBaseSystemPrompt(t *testing.T) {
	llm := &scriptedStreamingLLM{
		responses: []llmtypes.LLMResponse{
			{
				ToolCalls: []llmtypes.ToolCall{
					{
						ID:   "tc-1",
						Type: "function",
						Function: llmtypes.ToolCallFunction{
							Name:      "final_answer",
							Arguments: `{"text":"done","status":"succeeded","error":"","artifacts":[]}`,
						},
					},
				},
			},
		},
	}
	a := &DefaultAgent{
		LLM:          llm,
		Exec:         fixedHostExecutor{resp: types.HostOpResponse{Ok: true}},
		Model:        "test-model",
		SystemPrompt: "HARNESS_PROMPT_SHOULD_BE_PRESENT",
		PromptSource: PromptSourceFunc(func(context.Context, string, int) (string, error) {
			return "   ", nil
		}),
	}

	_, _, _, err := a.RunConversation(context.Background(), []llmtypes.LLMMessage{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if got := llm.lastReq.System; got != "HARNESS_PROMPT_SHOULD_BE_PRESENT" {
		t.Fatalf("system prompt = %q, want harness prompt fallback", got)
	}
}

func TestDefaultAgent_RunConversation_TextOnlyResponseTriggersNudge(t *testing.T) {
	llm := &scriptedStreamingLLM{
		responses: []llmtypes.LLMResponse{
			// First response: text only (triggers nudge)
			{Text: "I need to think about this..."},
			// Second response: proper final_answer (succeeds after nudge)
			{
				ToolCalls: []llmtypes.ToolCall{
					{
						ID:   "tc-1",
						Type: "function",
						Function: llmtypes.ToolCallFunction{
							Name:      "final_answer",
							Arguments: `{"text":"done","status":"succeeded","error":"","artifacts":[]}`,
						},
					},
				},
			},
		},
	}
	a := &DefaultAgent{
		LLM:          llm,
		Exec:         fixedHostExecutor{resp: types.HostOpResponse{Ok: true}},
		Model:        "test-model",
		ToolRegistry: &fixedToolRegistry{},
	}

	res, msgs, _, err := a.RunConversation(context.Background(), []llmtypes.LLMMessage{{Role: "user", Content: "do work"}})
	if err != nil {
		t.Fatalf("RunConversation: %v", err)
	}
	if res.Status != types.TaskStatusSucceeded {
		t.Fatalf("status=%q, want %q", res.Status, types.TaskStatusSucceeded)
	}
	if res.Text != "done" {
		t.Fatalf("text=%q, want %q", res.Text, "done")
	}

	// Verify the developer nudge message was injected into the conversation.
	foundNudge := false
	for _, m := range msgs {
		if m.Role == "developer" && strings.Contains(m.Content, "MUST call the final_answer tool to end your turn") {
			foundNudge = true
			break
		}
	}
	if !foundNudge {
		t.Fatal("expected developer nudge message in conversation history")
	}
}

func TestDefaultAgent_RunConversation_RepeatedTextOnlyTripsCircuitBreaker(t *testing.T) {
	prev := textOnlyNudgeThreshold
	textOnlyNudgeThreshold = 2
	t.Cleanup(func() {
		textOnlyNudgeThreshold = prev
	})

	llm := &scriptedStreamingLLM{
		responses: []llmtypes.LLMResponse{
			// Always returns text only, never calls final_answer.
			{Text: "still thinking..."},
		},
	}
	a := &DefaultAgent{
		LLM:          llm,
		Exec:         fixedHostExecutor{resp: types.HostOpResponse{Ok: true}},
		Model:        "test-model",
		ToolRegistry: &fixedToolRegistry{},
	}

	_, _, _, err := a.RunConversation(context.Background(), []llmtypes.LLMMessage{{Role: "user", Content: "do work"}})
	if err == nil {
		t.Fatal("expected text-only completion error")
	}
	var typedErr *TextOnlyCompletionError
	if !errors.As(err, &typedErr) {
		t.Fatalf("expected TextOnlyCompletionError, got %T (%v)", err, err)
	}
	if typedErr.Count != 2 {
		t.Fatalf("typedErr.Count=%d, want 2", typedErr.Count)
	}
	if !errors.Is(err, ErrTextOnlyCompletion) {
		t.Fatal("expected error to wrap ErrTextOnlyCompletion")
	}
}

func TestDefaultAgent_RunConversation_ToolCallResetsTextOnlyNudgeCounter(t *testing.T) {
	prev := textOnlyNudgeThreshold
	textOnlyNudgeThreshold = 2
	t.Cleanup(func() {
		textOnlyNudgeThreshold = prev
	})

	llm := &scriptedStreamingLLM{
		responses: []llmtypes.LLMResponse{
			// Step 1: text only (nudge counter = 1)
			{Text: "let me think..."},
			// Step 2: successful tool call (resets nudge counter to 0)
			{
				ToolCalls: []llmtypes.ToolCall{
					{
						ID:   "tc-1",
						Type: "function",
						Function: llmtypes.ToolCallFunction{
							Name:      "task_create",
							Arguments: `{"goal":"delegate"}`,
						},
					},
				},
			},
			// Step 3: text only again (nudge counter = 1, NOT 2)
			{Text: "delegated, waiting..."},
			// Step 4: proper final_answer (succeeds)
			{
				ToolCalls: []llmtypes.ToolCall{
					{
						ID:   "tc-2",
						Type: "function",
						Function: llmtypes.ToolCallFunction{
							Name:      "final_answer",
							Arguments: `{"text":"delegated work","status":"succeeded","error":"","artifacts":[]}`,
						},
					},
				},
			},
		},
	}
	a := &DefaultAgent{
		LLM:          llm,
		Exec:         fixedHostExecutor{resp: types.HostOpResponse{Ok: true}},
		Model:        "test-model",
		ToolRegistry: &fixedToolRegistry{},
	}

	// With threshold=2 and NO reset, this would fail at step 3 (2 text-only responses total).
	// With the reset, the tool call at step 2 resets the counter, so step 3 is only count=1.
	res, _, _, err := a.RunConversation(context.Background(), []llmtypes.LLMMessage{{Role: "user", Content: "coordinate"}})
	if err != nil {
		t.Fatalf("RunConversation: %v (expected success because tool call resets nudge counter)", err)
	}
	if res.Status != types.TaskStatusSucceeded {
		t.Fatalf("status=%q, want %q", res.Status, types.TaskStatusSucceeded)
	}
	if res.Text != "delegated work" {
		t.Fatalf("text=%q, want %q", res.Text, "delegated work")
	}
}
