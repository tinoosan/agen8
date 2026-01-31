package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/orchestrator"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// SetSwarmMode switches the current run between default and orchestrator behavior.
func (r *tuiTurnRunner) SetSwarmMode(enabled bool) error {
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
		if err := orchestrator.SyncRegistry(r.cfg, r.run.RunId); err == nil {
			if reg, err := orchestrator.LoadRegistryFile(r.cfg, r.run.RunId); err == nil {
				r.setSwarmSummary(renderSwarmSummary(reg))
				r.setSwarmRegistry(reg)
				if metrics, err := loadMetricsFile(r.cfg, r.run.RunId); err == nil {
					r.setSwarmMetrics(metrics)
				}
				if r.mustEmit != nil {
					r.mustEmit(context.Background(), events.Event{
						Type:    "swarm.registry.updated",
						Message: "Swarm registry updated",
						Data: map[string]string{
							"runId":     r.run.RunId,
							"sessionId": r.run.SessionID,
						},
					})
				}
			}
		}
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
		lastSummaryAt := time.Time{}
		lastSummaryText := ""
		finalSummarySent := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = orchestrator.SyncRegistry(r.cfg, r.run.RunId)
				reg, err := orchestrator.LoadRegistryFile(r.cfg, r.run.RunId)
				if err == nil {
					r.setSwarmSummary(renderSwarmSummary(reg))
					r.setSwarmRegistry(reg)
					if metrics, err := loadMetricsFile(r.cfg, r.run.RunId); err == nil {
						r.setSwarmMetrics(metrics)
					}
					if r.mustEmit != nil {
						r.mustEmit(context.Background(), events.Event{
							Type:    "swarm.registry.updated",
							Message: "Swarm registry updated",
							Data: map[string]string{
								"runId":     r.run.RunId,
								"sessionId": r.run.SessionID,
							},
						})
					}
					r.emitSwarmProgress(reg, &lastSummaryAt, &lastSummaryText, &finalSummarySent)
				}
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
	r.clearSwarmState()
}

func loadMetricsFile(cfg config.Config, runID string) (orchestrator.Metrics, error) {
	if err := cfg.Validate(); err != nil {
		return orchestrator.Metrics{}, err
	}
	if strings.TrimSpace(runID) == "" {
		return orchestrator.Metrics{}, fmt.Errorf("runID is required")
	}
	path := filepath.Join(fsutil.GetRunDir(cfg.DataDir, runID), "agents", "metrics.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return orchestrator.Metrics{}, err
	}
	var metrics orchestrator.Metrics
	if err := json.Unmarshal(b, &metrics); err != nil {
		return orchestrator.Metrics{}, fmt.Errorf("parse metrics: %w", err)
	}
	return metrics, nil
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

func (r *tuiTurnRunner) emitSwarmProgress(reg orchestrator.Registry, lastSummaryAt *time.Time, lastSummaryText *string, finalSummarySent *bool) {
	if r == nil || r.mustEmit == nil {
		return
	}
	r.ensureSwarmPlanFiles(reg)
	now := time.Now()
	if swarmIsComplete(reg) {
		if finalSummarySent != nil && *finalSummarySent {
			return
		}
		summary := renderSwarmCompletion(reg)
		if summary == "" {
			return
		}
		r.mustEmit(context.Background(), events.Event{
			Type:    "transcript.turn",
			Message: "Swarm completion update",
			Data: map[string]string{
				"user":  "",
				"agent": summary,
			},
		})
		if finalSummarySent != nil {
			*finalSummarySent = true
		}
		if lastSummaryAt != nil {
			*lastSummaryAt = now
		}
		if lastSummaryText != nil {
			*lastSummaryText = summary
		}
		return
	}
	if !swarmHasActiveWork(reg) {
		return
	}
	if lastSummaryAt != nil && !lastSummaryAt.IsZero() && now.Sub(*lastSummaryAt) < 15*time.Second {
		return
	}
	summary := renderSwarmProgress(reg)
	if summary == "" {
		return
	}
	if lastSummaryText != nil && strings.TrimSpace(*lastSummaryText) == strings.TrimSpace(summary) {
		return
	}
	r.mustEmit(context.Background(), events.Event{
		Type:    "transcript.turn",
		Message: "Swarm progress update",
		Data: map[string]string{
			"user":  "",
			"agent": summary,
		},
	})
	if lastSummaryAt != nil {
		*lastSummaryAt = now
	}
	if lastSummaryText != nil {
		*lastSummaryText = summary
	}
}

func swarmHasActiveWork(reg orchestrator.Registry) bool {
	for _, agent := range reg.Agents {
		status := strings.ToLower(strings.TrimSpace(agent.Status))
		switch status {
		case "pending", "busy", "running":
			return true
		}
	}
	for _, task := range reg.Tasks {
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if status == "" || status == "pending" || status == "in_progress" {
			return true
		}
	}
	return false
}

func renderSwarmProgress(reg orchestrator.Registry) string {
	if len(reg.Agents) == 0 {
		return ""
	}
	lines := []string{"Swarm progress:"}
	for _, id := range sortedAgentIDs(reg) {
		agent := reg.Agents[id]
		status := strings.TrimSpace(agent.Status)
		if status == "" {
			status = "unknown"
		}
		line := "- " + agent.RunID + " · " + status
		if strings.TrimSpace(agent.CurrentTaskID) != "" {
			line += " · task: " + strings.TrimSpace(agent.CurrentTaskID)
		}
		if strings.TrimSpace(agent.CurrentGoal) != "" {
			line += " · goal: " + truncateSwarmText(strings.TrimSpace(agent.CurrentGoal), 80)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderSwarmCompletion(reg orchestrator.Registry) string {
	if len(reg.Agents) == 0 {
		return ""
	}
	lines := []string{"Swarm completed:"}
	for _, id := range sortedAgentIDs(reg) {
		agent := reg.Agents[id]
		status := strings.TrimSpace(agent.Status)
		if status == "" {
			status = "unknown"
		}
		line := "- " + agent.RunID + " · " + status
		if agent.LastMessage != nil {
			title := strings.TrimSpace(agent.LastMessage.Title)
			body := strings.TrimSpace(agent.LastMessage.Body)
			if title != "" {
				line += " · result: " + truncateSwarmText(title, 100)
			} else if body != "" {
				line += " · result: " + truncateSwarmText(body, 100)
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func swarmIsComplete(reg orchestrator.Registry) bool {
	if len(reg.Agents) == 0 {
		return false
	}
	for _, agent := range reg.Agents {
		status := strings.ToLower(strings.TrimSpace(agent.Status))
		switch status {
		case "", "pending", "busy", "running":
			return false
		}
	}
	for _, task := range reg.Tasks {
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if status == "" || status == "pending" || status == "in_progress" {
			return false
		}
	}
	return true
}

func truncateSwarmText(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max-1]) + "…"
}

func sortedAgentIDs(reg orchestrator.Registry) []string {
	ids := make([]string, 0, len(reg.Agents))
	for id := range reg.Agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *tuiTurnRunner) ensureSwarmPlanFiles(reg orchestrator.Registry) {
	if r == nil || r.fs == nil || len(reg.Agents) == 0 {
		return
	}
	head := readPlanFile(r.fs, "/plan/HEAD.md")
	if strings.TrimSpace(head) == "" || strings.TrimSpace(head) == strings.TrimSpace(defaultSwarmHead) {
		body := renderSwarmPlanHead(reg)
		_ = r.fs.Write("/plan/HEAD.md", []byte(body))
	}
	checklist := readPlanFile(r.fs, "/plan/CHECKLIST.md")
	if strings.TrimSpace(checklist) == "" || strings.TrimSpace(checklist) == strings.TrimSpace(defaultSwarmChecklist) {
		body := renderSwarmChecklist(reg)
		_ = r.fs.Write("/plan/CHECKLIST.md", []byte(body))
	}
}

func readPlanFile(fs *vfs.FS, path string) string {
	if fs == nil {
		return ""
	}
	b, err := fs.Read(path)
	if err != nil {
		return ""
	}
	return string(b)
}

const defaultSwarmHead = "# Swarm Plan\n\n- [ ] Define goals and scope\n- [ ] Delegate tasks to workers\n- [ ] Collect updates and summarize\n"

const defaultSwarmChecklist = "- [ ] Delegation started\n- [ ] Worker updates reviewed\n- [ ] Final summary ready\n"

func renderSwarmPlanHead(reg orchestrator.Registry) string {
	lines := []string{"# Swarm Plan", "", "## Active agents"}
	ids := make([]string, 0, len(reg.Agents))
	for id := range reg.Agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		agent := reg.Agents[id]
		goal := truncateSwarmText(strings.TrimSpace(agent.CurrentGoal), 140)
		if goal == "" {
			goal = "No goal recorded."
		}
		lines = append(lines, "- "+id+": "+goal)
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderSwarmChecklist(reg orchestrator.Registry) string {
	lines := []string{}
	ids := make([]string, 0, len(reg.Agents))
	for id := range reg.Agents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		agent := reg.Agents[id]
		status := strings.ToLower(strings.TrimSpace(agent.Status))
		checked := " "
		if status == "idle" || status == "succeeded" || status == "completed" {
			checked = "x"
		}
		lines = append(lines, "- ["+checked+"] "+id+" status: "+strings.TrimSpace(agent.Status))
	}
	if len(lines) == 0 {
		lines = append(lines, "- [ ] Awaiting worker delegation")
	}
	return strings.Join(lines, "\n") + "\n"
}
