package agent

import (
	"context"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// HostExecutor is the host boundary for executing one host primitive.
type HostExecutor interface {
	Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse
}

// HostExecFunc adapts a function to HostExecutor so hosts can pass a standalone function.
type HostExecFunc func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse

func (f HostExecFunc) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	return f(ctx, req)
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
	OnLLMUsage    func(step int, usage llm.LLMUsage)
	OnWebSearch   func(step int, citations []llm.LLMCitation)
	OnToken       func(step int, text string)
	OnStreamChunk func(step int, chunk llm.LLMStreamChunk)
	Logf          func(format string, args ...any)
}
