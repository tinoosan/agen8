package agent

import (
	"context"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// Agent is the public interface implemented by all agent roles (default, orchestrator, worker).
type Agent interface {
	// Core execution
	Run(ctx context.Context, goal string) (string, error)
	RunConversation(ctx context.Context, msgs []llm.LLMMessage) (final string, updated []llm.LLMMessage, steps int, err error)
	ExecHostOp(ctx context.Context, req types.HostOpRequest) types.HostOpResponse

	// Configuration getters/setters
	GetModel() string
	SetModel(string)
	WebSearchEnabled() bool
	SetEnableWebSearch(bool)
	GetApprovalsMode() string
	SetApprovalsMode(string)
	GetReasoningEffort() string
	SetReasoningEffort(string)
	GetReasoningSummary() string
	SetReasoningSummary(string)
	GetSystemPrompt() string
	SetSystemPrompt(string)

	// Hooks access
	GetHooks() *Hooks
	SetHooks(Hooks)

	// Tools
	GetToolRegistry() *ToolRegistry
	SetToolRegistry(*ToolRegistry)
	GetExtraTools() []llm.Tool
	SetExtraTools([]llm.Tool)

	// Clone returns a shallow copy suitable for per-task customization.
	Clone() Agent
}
