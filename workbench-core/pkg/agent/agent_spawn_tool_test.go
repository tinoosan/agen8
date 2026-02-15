package agent

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type spawnTestLLM struct {
	mu       sync.Mutex
	requests []llmtypes.LLMRequest
	respText string
	err      error
}

func (f *spawnTestLLM) Generate(_ context.Context, req llmtypes.LLMRequest) (llmtypes.LLMResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
	if f.err != nil {
		return llmtypes.LLMResponse{}, f.err
	}
	return llmtypes.LLMResponse{Text: f.respText}, nil
}

func (f *spawnTestLLM) SupportsStreaming() bool { return true }

func (f *spawnTestLLM) GenerateStream(_ context.Context, req llmtypes.LLMRequest, cb llmtypes.LLMStreamCallback) (llmtypes.LLMResponse, error) {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	err := f.err
	respText := f.respText
	f.mu.Unlock()

	if err != nil {
		return llmtypes.LLMResponse{}, err
	}
	if cb != nil {
		if err := cb(llmtypes.LLMStreamChunk{Text: respText}); err != nil {
			return llmtypes.LLMResponse{}, err
		}
		if err := cb(llmtypes.LLMStreamChunk{Done: true}); err != nil {
			return llmtypes.LLMResponse{}, err
		}
	}
	return llmtypes.LLMResponse{Text: respText}, nil
}

func (f *spawnTestLLM) recorded() []llmtypes.LLMRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]llmtypes.LLMRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

type cancelAwareSpawnLLM struct{}

func (c *cancelAwareSpawnLLM) Generate(context.Context, llmtypes.LLMRequest) (llmtypes.LLMResponse, error) {
	return llmtypes.LLMResponse{}, context.Canceled
}

func (c *cancelAwareSpawnLLM) SupportsStreaming() bool { return true }

func (c *cancelAwareSpawnLLM) GenerateStream(ctx context.Context, _ llmtypes.LLMRequest, _ llmtypes.LLMStreamCallback) (llmtypes.LLMResponse, error) {
	<-ctx.Done()
	return llmtypes.LLMResponse{}, ctx.Err()
}

func newSpawnParentAgent(t *testing.T, llm llmtypes.LLMClient) Agent {
	t.Helper()
	parent, err := NewAgent(llm, types.HostExecFunc(func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: req.Text}
	}), AgentConfig{Model: "gpt-5"})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	return parent
}

func TestAgentSpawnTool_ExecuteReturnsNoopWithChildResult(t *testing.T) {
	llm := &spawnTestLLM{respText: "42"}
	parent := newSpawnParentAgent(t, llm)
	tool := &AgentSpawnTool{
		ParentAgent:  parent,
		MaxDepth:     3,
		CurrentDepth: 0,
		MaxTokens:    512,
	}

	args, _ := json.Marshal(map[string]any{
		"goal":               "Compute the answer",
		"background_context": []string{"prior result: 41", "check arithmetic"},
	})
	req, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpNoop {
		t.Fatalf("req.Op=%q, want %q", req.Op, types.HostOpNoop)
	}
	if req.Action != agentSpawnToolName {
		t.Fatalf("req.Action=%q, want %q", req.Action, agentSpawnToolName)
	}
	var meta struct {
		Goal            string `json:"goal"`
		BackgroundCount int    `json:"backgroundCount"`
		MaxDepth        int    `json:"maxDepth"`
	}
	if err := json.Unmarshal(req.Input, &meta); err != nil {
		t.Fatalf("expected spawn metadata json, got error: %v", err)
	}
	if meta.Goal != "Compute the answer" {
		t.Fatalf("meta.Goal=%q, want %q", meta.Goal, "Compute the answer")
	}
	if meta.BackgroundCount != 2 {
		t.Fatalf("meta.BackgroundCount=%d, want %d", meta.BackgroundCount, 2)
	}
	if meta.MaxDepth != 3 {
		t.Fatalf("meta.MaxDepth=%d, want %d", meta.MaxDepth, 3)
	}
	if req.Text != "42" {
		t.Fatalf("req.Text=%q, want %q", req.Text, "42")
	}

	requests := llm.recorded()
	if len(requests) != 1 {
		t.Fatalf("len(requests)=%d, want 1", len(requests))
	}
	msgs := requests[0].Messages
	if len(msgs) != 1 || strings.TrimSpace(msgs[0].Role) != "user" {
		t.Fatalf("unexpected child messages: %+v", msgs)
	}
	if !strings.Contains(msgs[0].Content, "prior result: 41") {
		t.Fatalf("expected background context in child prompt")
	}
	if !strings.Contains(msgs[0].Content, "Compute the answer") {
		t.Fatalf("expected goal in child prompt")
	}
	if requests[0].MaxTokens != 512 {
		t.Fatalf("child MaxTokens=%d, want %d", requests[0].MaxTokens, 512)
	}
}

func TestAgentSpawnTool_RejectsDepthLimit(t *testing.T) {
	llm := &spawnTestLLM{respText: "ok"}
	parent := newSpawnParentAgent(t, llm)
	tool := &AgentSpawnTool{
		ParentAgent:  parent,
		MaxDepth:     1,
		CurrentDepth: 1,
	}
	args, _ := json.Marshal(map[string]any{"goal": "blocked"})
	if _, err := tool.Execute(context.Background(), args); err == nil {
		t.Fatalf("expected depth-limit error")
	}
}

func TestAgentSpawnTool_ChildErrorReturnsNoopText(t *testing.T) {
	llm := &spawnTestLLM{err: context.DeadlineExceeded}
	parent := newSpawnParentAgent(t, llm)
	tool := &AgentSpawnTool{
		ParentAgent:  parent,
		MaxDepth:     3,
		CurrentDepth: 0,
	}
	args, _ := json.Marshal(map[string]any{"goal": "fail"})
	req, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpNoop {
		t.Fatalf("req.Op=%q, want %q", req.Op, types.HostOpNoop)
	}
	if req.Action != agentSpawnToolName {
		t.Fatalf("req.Action=%q, want %q", req.Action, agentSpawnToolName)
	}
	if !strings.Contains(req.Text, "agent_spawn error:") {
		t.Fatalf("expected propagated child error text, got %q", req.Text)
	}
}

func TestAgentSpawnTool_ContextCancellationPropagatesToChild(t *testing.T) {
	parent := newSpawnParentAgent(t, &cancelAwareSpawnLLM{})
	tool := &AgentSpawnTool{
		ParentAgent:  parent,
		MaxDepth:     3,
		CurrentDepth: 0,
	}
	args, _ := json.Marshal(map[string]any{"goal": "cancel me"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Op != types.HostOpNoop {
		t.Fatalf("req.Op=%q, want %q", req.Op, types.HostOpNoop)
	}
	if req.Action != agentSpawnToolName {
		t.Fatalf("req.Action=%q, want %q", req.Action, agentSpawnToolName)
	}
	if !strings.Contains(strings.ToLower(req.Text), "context canceled") {
		t.Fatalf("expected cancellation in noop text, got %q", req.Text)
	}
}
