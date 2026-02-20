package app

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/tools/builtins"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestConfigureCodeExecRuntime_KeepsToolAndReconcilesEnv(t *testing.T) {
	resetCodeExecWarningStateForTests()
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
	err = configureCodeExecRuntime(context.Background(), rt, config.Default(), modelRegistry, bridgeRegistry, []string{"pandas"}, false, func(_ context.Context, ev events.Event) {
		emitted = append(emitted, ev)
	})
	if err != nil {
		t.Fatalf("configureCodeExecRuntime: %v", err)
	}

	if _, ok := modelRegistry.Get("code_exec"); !ok {
		t.Fatalf("expected code_exec to remain registered when preflight fails")
	}
	if len(emitted) == 0 {
		t.Fatalf("expected at least one code_exec env event")
	}
}

func TestConfigureCodeExecRuntime_DedupesRepeatedWarnings(t *testing.T) {
	resetCodeExecWarningStateForTests()
	modelRegistry1, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default registry #1: %v", err)
	}
	bridgeRegistry1, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default bridge registry #1: %v", err)
	}
	modelRegistry2, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default registry #2: %v", err)
	}
	bridgeRegistry2, err := agent.DefaultHostToolRegistry()
	if err != nil {
		t.Fatalf("default bridge registry #2: %v", err)
	}

	inv1 := builtins.NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv1.PythonBin = "missing-python-for-code-exec-preflight-tests"
	rt1 := &runtime.Runtime{CodeExec: inv1}
	inv2 := builtins.NewBuiltinCodeExecInvoker(t.TempDir(), map[string]string{"workspace": t.TempDir()})
	inv2.PythonBin = "missing-python-for-code-exec-preflight-tests"
	rt2 := &runtime.Runtime{CodeExec: inv2}

	emitted := make([]events.Event, 0, 2)
	emit := func(_ context.Context, ev events.Event) {
		emitted = append(emitted, ev)
	}
	if err := configureCodeExecRuntime(context.Background(), rt1, config.Default(), modelRegistry1, bridgeRegistry1, []string{"pandas"}, false, emit); err != nil {
		t.Fatalf("configure #1: %v", err)
	}
	if err := configureCodeExecRuntime(context.Background(), rt2, config.Default(), modelRegistry2, bridgeRegistry2, []string{"pandas"}, false, emit); err != nil {
		t.Fatalf("configure #2: %v", err)
	}

	if got := len(emitted); got == 0 || got > 2 {
		t.Fatalf("expected 1-2 deduped warning events, got %d", got)
	}
}

func TestConfigureCodeExecRuntime_RequiredModeDoesNotFailWhenPreflightFails(t *testing.T) {
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

	err = configureCodeExecRuntime(context.Background(), rt, config.Default(), modelRegistry, bridgeRegistry, nil, true, nil)
	if err != nil {
		t.Fatalf("did not expect error when preflight fails in required mode: %v", err)
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

	err = configureCodeExecRuntime(context.Background(), rt, config.Default(), modelRegistry, bridgeRegistry, []string{"re", "json"}, true, nil)
	if err != nil {
		t.Fatalf("configureCodeExecRuntime: %v", err)
	}
	if got := strings.Join(inv.RequiredImports, ","); got != "" {
		t.Fatalf("expected required imports to be runtime-empty after package reconciliation, got %q", got)
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
