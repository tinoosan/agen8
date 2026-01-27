package agent

import (
	"context"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// Ensure DefaultAgent satisfies Agent.
var _ Agent = (*DefaultAgent)(nil)

func (a *DefaultAgent) ExecHostOp(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if a == nil || a.Exec == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "agent exec not configured"}
	}
	return a.Exec.Exec(ctx, req)
}

func (a *DefaultAgent) GetModel() string                { return a.Model }
func (a *DefaultAgent) SetModel(v string)               { a.Model = v }
func (a *DefaultAgent) WebSearchEnabled() bool          { return a.EnableWebSearch }
func (a *DefaultAgent) SetEnableWebSearch(v bool)       { a.EnableWebSearch = v }
func (a *DefaultAgent) GetApprovalsMode() string        { return a.ApprovalsMode }
func (a *DefaultAgent) SetApprovalsMode(v string)       { a.ApprovalsMode = v }
func (a *DefaultAgent) GetReasoningEffort() string      { return a.ReasoningEffort }
func (a *DefaultAgent) SetReasoningEffort(v string)     { a.ReasoningEffort = v }
func (a *DefaultAgent) GetReasoningSummary() string     { return a.ReasoningSummary }
func (a *DefaultAgent) SetReasoningSummary(v string)    { a.ReasoningSummary = v }
func (a *DefaultAgent) GetSystemPrompt() string         { return a.SystemPrompt }
func (a *DefaultAgent) SetSystemPrompt(v string)        { a.SystemPrompt = v }
func (a *DefaultAgent) GetHooks() *Hooks                { return &a.Hooks }
func (a *DefaultAgent) SetHooks(h Hooks)                { a.Hooks = h }
func (a *DefaultAgent) GetToolRegistry() *ToolRegistry  { return a.ToolRegistry }
func (a *DefaultAgent) SetToolRegistry(r *ToolRegistry) { a.ToolRegistry = r }
func (a *DefaultAgent) GetExtraTools() []llm.Tool       { return a.ExtraTools }
func (a *DefaultAgent) SetExtraTools(tools []llm.Tool)  { a.ExtraTools = tools }
func (a *DefaultAgent) Clone() Agent {
	if a == nil {
		return nil
	}
	cl := *a
	if a.ToolRegistry != nil {
		cl.ToolRegistry = a.ToolRegistry.Clone()
	}
	if a.ExtraTools != nil {
		cl.ExtraTools = append([]llm.Tool(nil), a.ExtraTools...)
	}
	return &cl
}
