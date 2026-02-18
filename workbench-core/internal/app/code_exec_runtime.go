package app

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func configureCodeExecRuntime(ctx context.Context, rt *runtime.Runtime, registry *agent.HostToolRegistry, emit func(context.Context, events.Event)) {
	if rt == nil || registry == nil {
		return
	}
	if rt.CodeExec == nil {
		registry.Remove("code_exec")
		return
	}
	if err := rt.CodeExec.EnsureReady(ctx); err != nil {
		registry.Remove("code_exec")
		if emit != nil {
			emit(context.Background(), events.Event{
				Type:    "daemon.warning",
				Message: "code_exec disabled: runtime preflight failed",
				Data: map[string]string{
					"tool":  "code_exec",
					"error": strings.TrimSpace(err.Error()),
				},
			})
		}
		return
	}
	rt.CodeExec.SetDispatcher(func(ctx context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error) {
		return registry.Dispatch(ctx, toolName, args)
	})
	rt.CodeExec.SetToolAllowlist(agent.SortedToolNamesFromRegistry(registry))
}
