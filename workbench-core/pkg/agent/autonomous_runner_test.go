package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/llm"
	"github.com/tinoosan/workbench-core/pkg/role"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type countingAgent struct {
	model        string
	systemPrompt string
	approvals    string
	reasonEffort string
	reasonSum    string

	webSearch bool

	hooks     Hooks
	registry  *ToolRegistry
	extra     []llm.Tool
	listCalls atomic.Int64
}

func (a *countingAgent) Run(ctx context.Context, goal string) (string, error) {
	_ = ctx
	_ = goal
	return "", nil
}

func (a *countingAgent) RunConversation(ctx context.Context, msgs []llm.LLMMessage) (final string, updated []llm.LLMMessage, steps int, err error) {
	_ = ctx
	return "", msgs, 0, nil
}

func (a *countingAgent) ExecHostOp(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	_ = ctx
	if req.Op == types.HostOpFSList && req.Path == "/inbox" {
		a.listCalls.Add(1)
		return types.HostOpResponse{Op: req.Op, Ok: true, Entries: nil}
	}
	return types.HostOpResponse{Op: req.Op, Ok: true}
}

func (a *countingAgent) GetModel() string                        { return a.model }
func (a *countingAgent) SetModel(s string)                       { a.model = s }
func (a *countingAgent) WebSearchEnabled() bool                  { return a.webSearch }
func (a *countingAgent) SetEnableWebSearch(b bool)               { a.webSearch = b }
func (a *countingAgent) GetApprovalsMode() string                { return a.approvals }
func (a *countingAgent) SetApprovalsMode(s string)               { a.approvals = s }
func (a *countingAgent) GetReasoningEffort() string              { return a.reasonEffort }
func (a *countingAgent) SetReasoningEffort(s string)             { a.reasonEffort = s }
func (a *countingAgent) GetReasoningSummary() string             { return a.reasonSum }
func (a *countingAgent) SetReasoningSummary(s string)            { a.reasonSum = s }
func (a *countingAgent) GetSystemPrompt() string                 { return a.systemPrompt }
func (a *countingAgent) SetSystemPrompt(s string)                { a.systemPrompt = s }
func (a *countingAgent) GetHooks() *Hooks                        { return &a.hooks }
func (a *countingAgent) SetHooks(h Hooks)                        { a.hooks = h }
func (a *countingAgent) GetToolRegistry() *ToolRegistry          { return a.registry }
func (a *countingAgent) SetToolRegistry(r *ToolRegistry)         { a.registry = r }
func (a *countingAgent) GetExtraTools() []llm.Tool               { return a.extra }
func (a *countingAgent) SetExtraTools(t []llm.Tool)              { a.extra = t }
func (a *countingAgent) Clone() Agent                            { cp := *a; return &cp }
func (a *countingAgent) Config() AgentConfig                     { return AgentConfig{} }
func (a *countingAgent) CloneWithConfig(cfg AgentConfig) (Agent, error) { _ = cfg; return a.Clone(), nil }

func TestAutonomousRunner_DoesNotBusyLoopInboxListWhenIdle(t *testing.T) {
	r, err := NewAutonomousRunner(AutonomousRunnerConfig{
		Agent: &countingAgent{},
		Role: role.Role{
			ID:          "test",
			Description: "test role",
			Obligations: []role.Obligation{
				{ID: "noop", ValidityRaw: "10m", Evidence: "noop"},
			},
			TaskPolicy: role.TaskPolicy{
				CreateTasksOnlyIf: []string{"obligation_unsatisfied"},
				MaxTasksPerCycle:  1,
			},
		},
		InboxPath:         "/inbox",
		OutboxPath:        "/outbox",
		PollInterval:      100 * time.Millisecond,
		ProactiveInterval: time.Hour,
		InitialGoal:       "",
	})
	if err != nil {
		t.Fatalf("NewAutonomousRunner: %v", err)
	}

	// Mark obligations as already satisfied so the runner stays idle and only polls /inbox.
	r.mu.Lock()
	r.lastSatisfied["noop"] = time.Now()
	r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 450*time.Millisecond)
	defer cancel()
	_ = r.Run(ctx)

	calls := r.cfg.Agent.(*countingAgent).listCalls.Load()
	// With a 100ms poll interval and no work, we should not see inbox listing in a tight loop.
	// Allow a small cushion for scheduler timing.
	if calls > 6 {
		t.Fatalf("expected <= 6 fs.list calls while idle; got %d", calls)
	}
}
