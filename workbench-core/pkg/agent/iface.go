package agent

import (
	"context"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// Runner is the agent execution interface (goal/conversation). It executes goal-oriented
// agent work without exposing configuration surface area.
//
// Distinct from tools.Orchestrator (tool runner) and internal/tui.TurnRunner (TUI turn).
// The interactive TUI uses TurnRunner for a single user turn plus resume/session helpers.
type Runner interface {
	Run(ctx context.Context, goal string) (string, error)
	RunConversation(ctx context.Context, msgs []llm.LLMMessage) (final string, updated []llm.LLMMessage, steps int, err error)
	ExecHostOp(ctx context.Context, req types.HostOpRequest) types.HostOpResponse
}

// Configurable exposes agent configuration and cloning helpers for callers that need them.
type Configurable interface {
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

	// Config returns a snapshot of the agent configuration.
	Config() AgentConfig

	// CloneWithConfig builds a new agent using the supplied configuration.
	CloneWithConfig(cfg AgentConfig) (Agent, error)
}

// Agent is the public interface implemented by all agent roles.
type Agent interface {
	Runner
	Configurable
}
