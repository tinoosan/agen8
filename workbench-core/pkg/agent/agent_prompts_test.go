package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// MockExecutor implements HostExecutor for testing
type MockExecutor struct{}

func (m MockExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	return types.HostOpResponse{}
}

// MockClient implements llm.LLMClient for testing
type MockClient struct{}

func (m MockClient) Generate(ctx context.Context, req llm.LLMRequest) (llm.LLMResponse, error) {
	return llm.LLMResponse{}, nil
}

func TestSystemPromptSuppression(t *testing.T) {
	// 1. Test that empty config uses default prompt
	t.Run("EmptySystemPrompt_UsesDefault", func(t *testing.T) {
		cfg := Config{
			LLM:   MockClient{},
			Exec:  MockExecutor{},
			Model: "test-model",
		}
		a, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		if !strings.Contains(a.Config().SystemPrompt, "COMPLEX TASKS REQUIRE A PLAN") {
			t.Errorf("Expected default prompt to contain planning rules, but it didn't.\nPrompt: %s", a.Config().SystemPrompt)
		}
	})

	// 2. Test that explicitly combining DefaultSystemPrompt + Context works (The Fix)
	t.Run("ExplicitCombination_IncludesBoth", func(t *testing.T) {
		contextData := "<context>Session Data</context>"
		// mimicking the fix in chat_setup.go/chat_tui.go
		fullPrompt := DefaultSystemPrompt() + "\n\n" + contextData

		cfg := Config{
			LLM:          MockClient{},
			Exec:         MockExecutor{},
			Model:        "test-model",
			SystemPrompt: fullPrompt,
		}
		a, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create agent: %v", err)
		}

		if !strings.Contains(a.Config().SystemPrompt, "COMPLEX TASKS REQUIRE A PLAN") {
			t.Errorf("Expected prompt to contain planning rules, but it didn't.")
		}
		if !strings.Contains(a.Config().SystemPrompt, contextData) {
			t.Errorf("Expected prompt to contain context data, but it didn't.")
		}
	})
}
