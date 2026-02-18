package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/prompts"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// MockExecutor implements HostExecutor for testing
type MockExecutor struct{}

func (m MockExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	return types.HostOpResponse{}
}

// MockClient implements llm.LLMClient for testing
type MockClient struct{}

func (m MockClient) Generate(ctx context.Context, req llmtypes.LLMRequest) (llmtypes.LLMResponse, error) {
	return llmtypes.LLMResponse{}, nil
}

func (m MockClient) SupportsStreaming() bool { return false }

type promptTestTool struct {
	name string
	desc string
}

func (t *promptTestTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name:        t.name,
			Description: t.desc,
		},
	}
}

func (t *promptTestTool) Execute(context.Context, json.RawMessage) (types.HostOpRequest, error) {
	return types.HostOpRequest{Op: types.HostOpNoop}, nil
}

func TestSystemPromptSuppression(t *testing.T) {
	// 1. Test that empty config uses default prompt
	t.Run("EmptySystemPrompt_UsesDefault", func(t *testing.T) {
		a, err := NewAgent(MockClient{}, MockExecutor{}, AgentConfig{Model: "test-model"})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		if !strings.Contains(a.Config().SystemPrompt, "fs_list") {
			t.Errorf("Expected default prompt to contain core tool list, but it didn't.\nPrompt: %s", a.Config().SystemPrompt)
		}
	})

	// 2. Test that explicitly combining DefaultSystemPrompt + Context works (The Fix)
	t.Run("ExplicitCombination_IncludesBoth", func(t *testing.T) {
		contextData := "<context>Session Data</context>"
		// mimicking the fix in chat_setup.go/chat_tui.go
		fullPrompt := prompts.DefaultSystemPrompt() + "\n\n" + contextData

		a, err := NewAgent(MockClient{}, MockExecutor{}, AgentConfig{Model: "test-model", SystemPrompt: fullPrompt})
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		if !strings.Contains(a.Config().SystemPrompt, "fs_list") {
			t.Errorf("Expected prompt to contain core tool list, but it didn't.")
		}
		if !strings.Contains(a.Config().SystemPrompt, contextData) {
			t.Errorf("Expected prompt to contain context data, but it didn't.")
		}
	})
}

func TestNewAgent_DefaultPrompt_UsesResolvedTools(t *testing.T) {
	reg := NewHostToolRegistry()
	if err := reg.Register(&promptTestTool{name: "zeta_tool", desc: "Zeta"}); err != nil {
		t.Fatalf("register zeta: %v", err)
	}
	if err := reg.Register(&promptTestTool{name: "alpha_tool", desc: "Alpha"}); err != nil {
		t.Fatalf("register alpha: %v", err)
	}
	a, err := NewAgent(MockClient{}, MockExecutor{}, AgentConfig{
		Model:            "test-model",
		HostToolRegistry: reg,
	})
	if err != nil {
		t.Fatalf("NewAgent: %v", err)
	}
	prompt := a.Config().SystemPrompt
	if !strings.Contains(prompt, "alpha_tool") || !strings.Contains(prompt, "zeta_tool") {
		t.Fatalf("expected injected tool names in prompt, got: %s", prompt)
	}
	if strings.Contains(prompt, `<op name="fs_list">`) {
		t.Fatalf("did not expect default core direct_ops list when registry is custom, got: %s", prompt)
	}
	if strings.Index(prompt, "alpha_tool") > strings.Index(prompt, "zeta_tool") {
		t.Fatalf("expected deterministic lexical order")
	}
}
