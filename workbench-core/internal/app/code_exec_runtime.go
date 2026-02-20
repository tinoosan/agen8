package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/runtime"
	"github.com/tinoosan/workbench-core/pkg/types"
)

var codeExecWarningState = struct {
	mu   sync.Mutex
	seen map[string]struct{}
}{
	seen: map[string]struct{}{},
}

func configureCodeExecRuntime(
	ctx context.Context,
	rt *runtime.Runtime,
	cfg config.Config,
	modelRegistry *agent.HostToolRegistry,
	bridgeRegistry *agent.HostToolRegistry,
	resolvedRequiredImports []string,
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
	provisioned, err := ensureCodeExecPythonEnv(ctx, cfg, strings.TrimSpace(rt.CodeExec.PythonBin), resolvedRequiredImports)
	if err != nil {
		if emit != nil {
			data := map[string]string{
				"tool":  "code_exec",
				"error": strings.TrimSpace(err.Error()),
			}
			var envErr *codeExecEnvError
			if errors.As(err, &envErr) && envErr != nil {
				data["stage"] = strings.TrimSpace(envErr.Stage)
				if len(envErr.MissingMods) > 0 {
					data["missingPackages"] = strings.Join(envErr.MissingMods, ",")
				}
			}
			emitCodeExecWarningOnce(emit, events.Event{
				Type:    "code_exec.env.reconcile_failed",
				Message: "code_exec environment reconcile failed; update [code_exec].required_packages in config.toml",
				Data:    data,
			})
		}
	} else {
		if bin := strings.TrimSpace(provisioned.PythonBin); bin != "" {
			rt.CodeExec.PythonBin = bin
		}
		if emit != nil {
			data := map[string]string{
				"tool":     "code_exec",
				"venvPath": strings.TrimSpace(provisioned.VenvPath),
				"python":   strings.TrimSpace(provisioned.PythonBin),
			}
			if len(provisioned.InstalledMods) > 0 {
				data["installedPackages"] = strings.Join(provisioned.InstalledMods, ",")
			}
			emitCodeExecWarningOnce(emit, events.Event{
				Type:    "code_exec.env.reconciled",
				Message: "code_exec environment reconciled",
				Data:    data,
			})
		}
	}
	rt.CodeExec.SetRequiredImports(nil)
	if err := rt.CodeExec.EnsureReady(ctx); err != nil {
		if emit != nil {
			data := map[string]string{
				"tool":  "code_exec",
				"error": strings.TrimSpace(err.Error()),
			}
			if len(resolvedRequiredImports) > 0 {
				data["requiredImports"] = strings.Join(resolvedRequiredImports, ",")
			}
			if missing := parseMissingPythonModules(err); len(missing) > 0 {
				data["missingModules"] = strings.Join(missing, ",")
			}
			emitCodeExecWarningOnce(emit, events.Event{
				Type:    "daemon.warning",
				Message: "code_exec runtime preflight failed; check python runtime and update [code_exec].required_packages in config.toml",
				Data:    data,
			})
		}
	}
	rt.CodeExec.SetDispatcher(func(ctx context.Context, toolName string, args json.RawMessage) (types.HostOpRequest, error) {
		return bridgeRegistry.Dispatch(ctx, toolName, args)
	})
	rt.CodeExec.SetToolAllowlist(agent.SortedToolNamesFromRegistry(bridgeRegistry))
	return nil
}

func emitCodeExecWarningOnce(emit func(context.Context, events.Event), ev events.Event) {
	if emit == nil {
		return
	}
	key := codeExecWarningKey(ev)
	if key == "" {
		emit(context.Background(), ev)
		return
	}
	codeExecWarningState.mu.Lock()
	if _, ok := codeExecWarningState.seen[key]; ok {
		codeExecWarningState.mu.Unlock()
		return
	}
	codeExecWarningState.seen[key] = struct{}{}
	codeExecWarningState.mu.Unlock()
	emit(context.Background(), ev)
}

func codeExecWarningKey(ev events.Event) string {
	typ := strings.TrimSpace(ev.Type)
	msg := strings.TrimSpace(ev.Message)
	if typ == "" || msg == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(typ)
	b.WriteString("|")
	b.WriteString(msg)
	if len(ev.Data) == 0 {
		return b.String()
	}
	keys := make([]string, 0, len(ev.Data))
	for k := range ev.Data {
		keys = append(keys, k)
	}
	// Stable key generation for deterministic warning de-duplication.
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString("|")
		b.WriteString(strings.TrimSpace(k))
		b.WriteString("=")
		b.WriteString(strings.TrimSpace(ev.Data[k]))
	}
	return b.String()
}

func resetCodeExecWarningStateForTests() {
	codeExecWarningState.mu.Lock()
	defer codeExecWarningState.mu.Unlock()
	codeExecWarningState.seen = map[string]struct{}{}
}
