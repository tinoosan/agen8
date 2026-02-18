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
	modelRegistry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default registry: %v", err)
	}
	bridgeRegistry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default bridge registry: %v", err)
	}
	if _, ok := modelRegistry.Get("code_exec"); !ok {
		t.Fatalf("expected code_exec in default registry")
	}

	inv := builtins.NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv.PythonBin = "missing-python-for-code-exec-preflight-tests"
	rt := &runtime.Runtime{CodeExec: inv}

	emitted := make([]events.Event, 0, 1)
	err = configureCodeExecRuntime(context.Background(), rt, modelRegistry, bridgeRegistry, false, func(_ context.Context, ev events.Event) {
		emitted = append(emitted, ev)
	})
	if err != nil {
		t.Fatalf("configureCodeExecRuntime: %v", err)
	}

	if _, ok := modelRegistry.Get("code_exec"); ok {
		t.Fatalf("expected code_exec to be removed when preflight fails")
	}
	if len(emitted) == 0 {
		t.Fatalf("expected warning event when disabling code_exec")
	}
	if got := strings.TrimSpace(emitted[0].Data["error"]); got == "" {
		t.Fatalf("expected warning event to include preflight error")
	}
}

func TestConfigureCodeExecRuntime_RequiredModeFailsWhenPreflightFails(t *testing.T) {
	modelRegistry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default registry: %v", err)
	}
	bridgeRegistry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default bridge registry: %v", err)
	}

	inv := builtins.NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv.PythonBin = "missing-python-for-code-exec-preflight-tests"
	rt := &runtime.Runtime{CodeExec: inv}

	err = configureCodeExecRuntime(context.Background(), rt, modelRegistry, bridgeRegistry, true, nil)
	if err == nil {
		t.Fatalf("expected error when required code_exec preflight fails")
	}
}

func TestConfigureCodeExecRuntime_ConfiguresDispatcherAndAllowlist(t *testing.T) {
	pythonBin, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}

	baseRegistry, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default registry: %v", err)
	}
	modelRegistry, bridgeRegistry, err := resolveToolRegistries(baseRegistry, []string{"fs_list"}, true)
	if err != nil {
		t.Fatalf("resolveToolRegistries: %v", err)
	}
	if _, ok := modelRegistry.Get("fs_list"); ok {
		t.Fatalf("expected fs_list hidden from model registry in code_exec_only mode")
	}
	if _, ok := bridgeRegistry.Get("fs_list"); !ok {
		t.Fatalf("expected fs_list present in bridge registry")
	}
	if _, ok := modelRegistry.Get("code_exec"); !ok {
		t.Fatalf("expected code_exec present in model registry")
	}

	inv := builtins.NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{
		"workspace": t.TempDir(),
		"project":   t.TempDir(),
	})
	inv.PythonBin = pythonBin
	rt := &runtime.Runtime{CodeExec: inv}

	err = configureCodeExecRuntime(context.Background(), rt, modelRegistry, bridgeRegistry, true, nil)
	if err != nil {
		t.Fatalf("configureCodeExecRuntime: %v", err)
	}
	if _, ok := modelRegistry.Get("code_exec"); !ok {
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
