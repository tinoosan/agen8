package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/tinoosan/workbench-core/internal/types"
)

type fakeLLM struct {
	Replies []string
	i       int
}

func (f *fakeLLM) Generate(ctx context.Context, req types.LLMRequest) (types.LLMResponse, error) {
	_ = ctx
	_ = req
	if f.i >= len(f.Replies) {
		return types.LLMResponse{Text: `{"op":"final","text":"no more replies"}`}, nil
	}
	out := types.LLMResponse{Text: f.Replies[f.i]}
	f.i++
	return out, nil
}

func TestAgentLoopV0_Run_ExecutesOpsUntilFinal(t *testing.T) {
	llm := &fakeLLM{
		Replies: []string{
			`{"op":"fs.list","path":"/tools"}`,
			`{"op":"final","text":"done"}`,
		},
	}

	var called []types.HostOpRequest
	exec := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		_ = ctx
		called = append(called, req)
		b, _ := json.Marshal(req)
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: string(b)}
	}

	a := &Agent{LLM: llm, Exec: HostExecFunc(exec), Model: "test-model", MaxSteps: 5}
	final, err := a.Run(context.Background(), "goal")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final != "done" {
		t.Fatalf("unexpected final %q", final)
	}
	if len(called) != 1 || called[0].Op != types.HostOpFSList || called[0].Path != "/tools" {
		t.Fatalf("unexpected calls: %+v", called)
	}
}
