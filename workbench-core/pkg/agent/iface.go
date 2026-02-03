package agent

import (
	"context"
	"encoding/json"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// Runner is the agent execution interface (goal/conversation). It executes goal-oriented
// agent work without exposing configuration surface area.
//
// Distinct from tools.Orchestrator (tool runner) and internal/tui.TurnRunner (TUI turn).
// The interactive TUI uses TurnRunner for a single user turn plus resume/session helpers.
type Runner interface {
	Run(ctx context.Context, goal string) (RunResult, error)
	RunConversation(ctx context.Context, msgs []llmtypes.LLMMessage) (final RunResult, updated []llmtypes.LLMMessage, steps int, err error)
	ExecHostOp(ctx context.Context, req types.HostOpRequest) types.HostOpResponse
}

// ToolRegistryProvider supplies tool definitions and resolves tool calls into host ops.
type ToolRegistryProvider interface {
	Definitions() []llmtypes.Tool
	Dispatch(ctx context.Context, name string, args json.RawMessage) (types.HostOpRequest, error)
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
	GetToolRegistry() ToolRegistryProvider
	SetToolRegistry(ToolRegistryProvider)
	GetExtraTools() []llmtypes.Tool
	SetExtraTools([]llmtypes.Tool)

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

// RunResult is the finalized output of one agent execution.
type RunResult struct {
	Text      string
	Artifacts []string
}
