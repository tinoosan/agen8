package agent

import (
	"context"

	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// HostExecutor is the host boundary for executing one host primitive.
type HostExecutor interface {
	Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse
}

// PromptSource produces an augmented system prompt per agent step.
type PromptSource interface {
	SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error)
}

// PromptSourceFunc adapts a function to PromptSource.
type PromptSourceFunc func(ctx context.Context, basePrompt string, step int) (string, error)

func (f PromptSourceFunc) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	return f(ctx, basePrompt, step)
}

// Hooks are optional observability callbacks invoked by the agent loop.
type Hooks struct {
	OnLLMUsage    func(step int, usage llmtypes.LLMUsage)
	OnWebSearch   func(step int, citations []llmtypes.LLMCitation)
	OnToken       func(step int, text string)
	OnStreamChunk func(step int, chunk llmtypes.LLMStreamChunk)
	OnStep        func(step int, model string, effectiveModel string, reasoningSummary string)
	Logf          func(format string, args ...any)
}
