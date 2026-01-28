package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/orchestrator"
)

// SetSwarmMode switches the current run between default and orchestrator behavior.
func (r *tuiTurnRunner) SetSwarmMode(enabled bool) error {
	// #region agent log
	debugLogSwarmMode("app/swarm_mode.go:14", "SetSwarmMode enter", map[string]any{
		"enabled":          enabled,
		"swarmModeEnabled": func() bool { return r != nil && r.swarmModeEnabled }(),
		"hasLLMClient":     r != nil && r.llmClient != nil,
		"hasBaseExecutor":  r != nil && r.baseExecutor != nil,
		"runId":            func() string { if r == nil { return "" }; return r.run.RunId }(),
		"sessionId":        func() string { if r == nil { return "" }; return r.run.SessionID }(),
	})
	// #endregion
	if r == nil {
		return fmt.Errorf("runner is nil")
	}
	if r.swarmModeEnabled == enabled {
		return nil
	}
	if r.llmClient == nil || r.baseExecutor == nil {
		return fmt.Errorf("agent runtime not initialized")
	}
	if enabled {
		if err := EnsurePlanGate(r.fs); err != nil {
			return fmt.Errorf("ensure plan gate: %w", err)
		}
	}
	exec := r.baseExecutor
	basePrompt := agent.DefaultSystemPrompt()
	ctxSrc := agent.ContextSource(r.constructor)
	cfg := agent.DefaultConfig()
	if enabled {
		basePrompt = agent.OrchestratorSystemPrompt()
		exec = &orchestratorExecutor{base: r.baseExecutor, runner: r}
		cfg = agent.OrchestratorConfig()
		ctxSrc = agent.ContextSourceFunc(func(ctx context.Context, base string, step int) (string, error) {
			if r.constructor == nil {
				return base, nil
			}
			updated, err := r.constructor.SystemPrompt(ctx, base, step)
			if err != nil {
				return updated, err
			}
			summary := strings.TrimSpace(r.getSwarmSummary())
			if summary == "" {
				return updated, nil
			}
			return updated + "\n\n<swarm>\n" + summary + "\n</swarm>", nil
		})
	}
	cfg.Model = r.model
	cfg.ReasoningEffort = strings.TrimSpace(r.opts.ReasoningEffort)
	cfg.ReasoningSummary = strings.TrimSpace(r.opts.ReasoningSummary)
	cfg.ApprovalsMode = strings.TrimSpace(r.opts.ApprovalsMode)
	cfg.EnableWebSearch = r.opts.WebSearchEnabled
	cfg.SystemPrompt = basePrompt
	cfg.Context = ctxSrc
	cfg.ToolManifests = r.toolManifests
	if enabled {
		registry, extraTools, err := agent.OrchestratorToolRegistry(r.toolManifests)
		if err != nil {
			return err
		}
		cfg.ToolRegistry = registry
		cfg.ExtraTools = extraTools
	}
	a, err := agent.NewAgent(r.llmClient, exec, cfg)
	if err != nil {
		return err
	}
	r.runner = a
	r.configurable = a
	r.baseSystemPrompt = basePrompt
	r.swarmModeEnabled = enabled
	if enabled {
		r.startOrchestratorSyncLoop()
		syncErr := orchestrator.SyncRegistry(r.cfg, r.run.RunId)
		loadErr := error(nil)
		reg := orchestrator.Registry{}
		if syncErr == nil {
			reg, loadErr = orchestrator.LoadRegistryFile(r.cfg, r.run.RunId)
			if loadErr == nil {
				r.setSwarmSummary(renderSwarmSummary(reg))
			}
		}
		// #region agent log
		debugLogSwarmMode("app/swarm_mode.go:78", "SyncRegistry on enable", map[string]any{
			"syncErr":    errString(syncErr),
			"loadErr":    errString(loadErr),
			"agentCount": len(reg.Agents),
			"runId":      r.run.RunId,
			"sessionId":  r.run.SessionID,
		})
		// #endregion
	} else {
		r.stopOrchestratorSyncLoop()
	}
	return nil
}

func (r *tuiTurnRunner) startOrchestratorSyncLoop() {
	if r == nil {
		return
	}
	if r.swarmSyncCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.swarmSyncCancel = cancel
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				syncErr := orchestrator.SyncRegistry(r.cfg, r.run.RunId)
				reg, loadErr := orchestrator.LoadRegistryFile(r.cfg, r.run.RunId)
				if loadErr == nil {
					r.setSwarmSummary(renderSwarmSummary(reg))
				}
				// #region agent log
				debugLogSwarmMode("app/swarm_mode.go:106", "SyncRegistry tick", map[string]any{
					"syncErr":    errString(syncErr),
					"loadErr":    errString(loadErr),
					"agentCount": len(reg.Agents),
					"runId":      r.run.RunId,
					"sessionId":  r.run.SessionID,
				})
				// #endregion
			}
		}
	}()
}

func (r *tuiTurnRunner) stopOrchestratorSyncLoop() {
	if r == nil {
		return
	}
	if r.swarmSyncCancel != nil {
		r.swarmSyncCancel()
		r.swarmSyncCancel = nil
	}
	r.setSwarmSummary("")
}

func renderSwarmSummary(reg orchestrator.Registry) string {
	if len(reg.Agents) == 0 {
		return ""
	}
	lines := []string{"Workers:"}
	for _, a := range reg.Agents {
		status := strings.TrimSpace(a.Status)
		if status == "" {
			status = "unknown"
		}
		line := "- " + a.RunID + " (" + status + ")"
		if a.LastMessage != nil {
			msg := a.LastMessage
			title := strings.TrimSpace(msg.Title)
			body := strings.TrimSpace(msg.Body)
			if title != "" {
				line += ": " + title
			} else if body != "" {
				line += ": " + body
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func debugLogSwarmMode(location, message string, data map[string]any) {
	payload := map[string]any{
		"sessionId":    "debug-session",
		"runId":        "pre-fix",
		"hypothesisId": "H1",
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	f, err := os.OpenFile("/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(b, '\n'))
	_ = f.Close()
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
