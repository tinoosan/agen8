package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func configureCodeExecRuntime(
	ctx context.Context,
	rt *runtime.Runtime,
	modelRegistry *agent.HostToolRegistry,
	bridgeRegistry *agent.HostToolRegistry,
	required bool,
	emit func(context.Context, events.Event),
) error {
	if rt == nil || modelRegistry == nil || bridgeRegistry == nil {
		return nil
	}
	if rt.CodeExec == nil {
		modelRegistry.Remove("code_exec")
		if required {
			return fmt.Errorf("code_exec is required but runtime invoker is not configured")
		}
		return nil
	}
	if err := rt.CodeExec.EnsureReady(ctx); err != nil {
		modelRegistry.Remove("code_exec")
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
		if required {
			return fmt.Errorf("code_exec is required but preflight failed: %w", err)
		}
		return nil
	}
	rt.CodeExec.SetDispatcher(func(ctx context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error) {
		return bridgeRegistry.Dispatch(ctx, toolName, args)
	})
	rt.CodeExec.SetToolAllowlist(agent.SortedToolNamesFromRegistry(bridgeRegistry))
	return nil
}
