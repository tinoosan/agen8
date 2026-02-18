package app

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/tools/builtins"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestConfigureCodeExecRuntime_DisablesToolWhenPreflightFails(t *testing.T) {
	registry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default registry: %v", err)
	}
	if _, ok := registry.Get("code_exec"); !ok {
		t.Fatalf("expected code_exec in default registry")
	}

	inv := builtins.NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv.PythonBin = "missing-python-for-code-exec-preflight-tests"
	rt := &runtime.Runtime{CodeExec: inv}

	emitted := make([]events.Event, 0, 1)
	configureCodeExecRuntime(context.Background(), rt, registry, func(_ context.Context, ev events.Event) {
		emitted = append(emitted, ev)
	})

	if _, ok := registry.Get("code_exec"); ok {
		t.Fatalf("expected code_exec to be removed when preflight fails")
	}
	if len(emitted) == 0 {
		t.Fatalf("expected warning event when disabling code_exec")
	}
	if got := strings.TrimSpace(emitted[0].Data["error"]); got == "" {
		t.Fatalf("expected warning event to include preflight error")
	}
}

func TestConfigureCodeExecRuntime_ConfiguresDispatcherAndAllowlist(t *testing.T) {
	pythonBin, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}

	registry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default registry: %v", err)
	}
	inv := builtins.NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{
		"workspace": t.TempDir(),
		"project":   t.TempDir(),
	})
	inv.PythonBin = pythonBin
	rt := &runtime.Runtime{CodeExec: inv}

	configureCodeExecRuntime(context.Background(), rt, registry, nil)
	if _, ok := registry.Get("code_exec"); !ok {
		t.Fatalf("expected code_exec to remain registered after successful preflight")
	}
	inv.SetBridge(func(_ context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: `{"entries":[]}`}
	})

	input, err := json.Marshal(map[string]any{
		"language": "python",
		"code":     "import tools\nresult = tools.fs_list(path='/project')",
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	_, err = inv.Invoke(context.Background(), pkgtools.ToolRequest{
		Version:  "v1",
		CallID:   "code_exec",
		ToolID:   pkgtools.ToolID("builtin.code_exec"),
		ActionID: "run",
		Input:    input,
	})
	if err != nil {
		t.Fatalf("expected invoke to succeed after configureCodeExecRuntime, got: %v", err)
	}
}
