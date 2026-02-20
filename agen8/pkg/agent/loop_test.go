package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

type scriptedStreamingLLM struct {
	responses []llmtypes.LLMResponse
	index     int
}

func (s *scriptedStreamingLLM) Generate(context.Context, llmtypes.LLMRequest) (llmtypes.LLMResponse, error) {
	return llmtypes.LLMResponse{}, nil
}

func (s *scriptedStreamingLLM) SupportsStreaming() bool { return true }

func (s *scriptedStreamingLLM) GenerateStream(_ context.Context, _ llmtypes.LLMRequest, _ llmtypes.LLMStreamCallback) (llmtypes.LLMResponse, error) {
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
				Text: "done",
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
