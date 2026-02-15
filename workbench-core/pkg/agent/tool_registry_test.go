package agent

import (
	"context"
	"encoding/json"
	"testing"

	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type testTool struct {
	name string
}

func (t *testTool) Definition() llmtypes.Tool {
	return llmtypes.Tool{
		Type: "function",
		Function: llmtypes.ToolFunction{
			Name: t.name,
		},
	}
}

func (t *testTool) Execute(context.Context, json.RawMessage) (types.HostOpRequest, error) {
	return types.HostOpRequest{Op: types.HostOpNoop}, nil
}

func TestHostToolRegistry_GetRemoveReplace(t *testing.T) {
	reg := NewHostToolRegistry()
	if err := reg.Register(&testTool{name: "a"}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	tool, ok := reg.Get("a")
	if !ok || tool == nil {
		t.Fatalf("Get returned missing tool")
	}

	reg.Replace("a", &testTool{name: "b"})
	tool, ok = reg.Get("a")
	if !ok || tool == nil {
		t.Fatalf("Get after Replace returned missing tool")
	}
	if got := tool.Definition().Function.Name; got != "b" {
		t.Fatalf("tool name=%q, want %q", got, "b")
	}

	reg.Remove("a")
	if _, ok := reg.Get("a"); ok {
		t.Fatalf("expected tool to be removed")
	}
}
