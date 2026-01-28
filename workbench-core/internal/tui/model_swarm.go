package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/orchestrator"
)

func (m *Model) refreshSwarmView() {
	if !m.showDetails || m.swarmViewport.Width == 0 {
		return
	}
	w := max(24, m.swarmViewport.Width-4)

	reg, regErr := m.loadSwarmRegistry()
	metrics, metricsErr := m.loadSwarmMetrics()

	if regErr != "" {
		m.swarmLoadErr = regErr
	}

	fallbackSummary := ""
	if provider, ok := m.runner.(swarmSummaryProvider); ok {
		fallbackSummary = strings.TrimSpace(provider.GetSwarmSummary())
	}
	content := ""
	if fallbackSummary != "" && (strings.TrimSpace(regErr) != "" || len(reg.Agents) == 0) {
		content = "### Swarm\n\n_Swarm (last known):_\n\n" + fallbackSummary
	} else {
		content = m.renderSwarmContent(reg, regErr, metrics, metricsErr)
	}
	// #region agent log
	debugLogSwarmView("tui/model_swarm.go:27", "refreshSwarmView", map[string]any{
		"regErr":            regErr,
		"metricsErr":        metricsErr,
		"agentCount":        len(reg.Agents),
		"fallbackSummary":   fallbackSummary != "",
		"fallbackSummaryLn": len(fallbackSummary),
		"runnerType":        fmt.Sprintf("%T", m.runner),
		"hasVFSAccessor":    func() bool { _, ok := m.runner.(vfsAccessor); return ok }(),
	})
	// #endregion
	if strings.TrimSpace(content) == "" {
		if strings.TrimSpace(m.swarmContent) != "" {
			content = m.swarmContent // keep last good view instead of flickering blank
		} else {
			content = "_Swarm view is preparing…_"
		}
	}
	rendered := strings.TrimRight(m.renderer.RenderMarkdown(content, w), "\n")
	m.swarmContent = content
	m.swarmViewport.SetContent(rendered)
}

func (m *Model) loadSwarmRegistry() (orchestrator.Registry, string) {
	accessor, ok := m.runner.(vfsAccessor)
	if !ok {
		return orchestrator.Registry{}, "VFS access unavailable"
	}
	text, _, _, err := accessor.ReadVFS(m.ctx, "/agents/registry.json", 128*1024)
	if err != nil {
		// #region agent log
		debugLogSwarmView("tui/model_swarm.go:44", "loadSwarmRegistry read error", map[string]any{
			"readErr": err.Error(),
		})
		// #endregion
		return orchestrator.Registry{}, err.Error()
	}
	if strings.TrimSpace(text) == "" {
		// #region agent log
		debugLogSwarmView("tui/model_swarm.go:49", "loadSwarmRegistry empty", map[string]any{
			"textLen": len(text),
		})
		// #endregion
		return orchestrator.Registry{}, ""
	}
	var reg orchestrator.Registry
	if err := json.Unmarshal([]byte(text), &reg); err != nil {
		// #region agent log
		debugLogSwarmView("tui/model_swarm.go:55", "loadSwarmRegistry unmarshal error", map[string]any{
			"unmarshalErr": err.Error(),
			"textLen":      len(text),
		})
		// #endregion
		return orchestrator.Registry{}, "invalid registry.json"
	}
	return reg, ""
}

func (m *Model) loadSwarmMetrics() (orchestrator.Metrics, string) {
	accessor, ok := m.runner.(vfsAccessor)
	if !ok {
		return orchestrator.Metrics{}, "VFS access unavailable"
	}
	text, _, _, err := accessor.ReadVFS(m.ctx, "/agents/metrics.json", 128*1024)
	if err != nil {
		// #region agent log
		debugLogSwarmView("tui/model_swarm.go:66", "loadSwarmMetrics read error", map[string]any{
			"readErr": err.Error(),
		})
		// #endregion
		return orchestrator.Metrics{}, err.Error()
	}
	if strings.TrimSpace(text) == "" {
		// #region agent log
		debugLogSwarmView("tui/model_swarm.go:71", "loadSwarmMetrics empty", map[string]any{
			"textLen": len(text),
		})
		// #endregion
		return orchestrator.Metrics{}, ""
	}
	var metrics orchestrator.Metrics
	if err := json.Unmarshal([]byte(text), &metrics); err != nil {
		// #region agent log
		debugLogSwarmView("tui/model_swarm.go:77", "loadSwarmMetrics unmarshal error", map[string]any{
			"unmarshalErr": err.Error(),
			"textLen":      len(text),
		})
		// #endregion
		return orchestrator.Metrics{}, "invalid metrics.json"
	}
	return metrics, ""
}

func debugLogSwarmView(location, message string, data map[string]any) {
	payload := map[string]any{
		"sessionId":    "debug-session",
		"runId":        "pre-fix",
		"hypothesisId": "H2",
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

func (m *Model) renderSwarmContent(reg orchestrator.Registry, regErr string, metrics orchestrator.Metrics, metricsErr string) string {
	header := "### Swarm\n\n"
	if strings.TrimSpace(regErr) != "" && strings.TrimSpace(regErr) != "VFS access unavailable" {
		return header + fmt.Sprintf("_Failed to load registry: %s_", regErr)
	}
	if len(reg.Agents) == 0 {
		return header + "_No agents yet._"
	}

	lines := []string{header}
	if metricsErr == "" {
		if metrics.CostUSD.Total > 0 || metrics.Tokens.In > 0 || metrics.Tokens.Out > 0 {
			lines = append(lines, fmt.Sprintf("_Totals: %d in · %d out · $%.4f_\n", metrics.Tokens.In, metrics.Tokens.Out, metrics.CostUSD.Total))
		}
	}

	agentIDs := make([]string, 0, len(reg.Agents))
	for id := range reg.Agents {
		agentIDs = append(agentIDs, id)
	}
	sort.Strings(agentIDs)
	for _, id := range agentIDs {
		agent := reg.Agents[id]
		status := strings.TrimSpace(agent.Status)
		if status == "" {
			status = "unknown"
		}
		line := fmt.Sprintf("- **%s** · %s", id, status)
		if strings.TrimSpace(agent.CurrentTaskID) != "" {
			line += " · task: " + strings.TrimSpace(agent.CurrentTaskID)
		}
		lines = append(lines, line)
		if strings.TrimSpace(agent.CurrentGoal) != "" {
			lines = append(lines, "  - goal: "+strings.TrimSpace(agent.CurrentGoal))
		}
		if len(agent.Plan) > 0 {
			steps := agent.Plan
			if len(steps) > 3 {
				steps = steps[:3]
			}
			for _, step := range steps {
				step = strings.TrimSpace(step)
				if step == "" {
					continue
				}
				lines = append(lines, "  - "+step)
			}
		}
		if agent.LastMessage != nil {
			msg := agent.LastMessage
			title := strings.TrimSpace(msg.Title)
			body := strings.TrimSpace(msg.Body)
			if title != "" {
				lines = append(lines, "  - msg: "+title)
			} else if body != "" {
				lines = append(lines, "  - msg: "+body)
			}
		}
		if agent.Stats.CostUSD > 0 || agent.Stats.TokensIn > 0 || agent.Stats.TokensOut > 0 {
			lines = append(lines, fmt.Sprintf("  - usage: %d in · %d out · $%.4f", agent.Stats.TokensIn, agent.Stats.TokensOut, agent.Stats.CostUSD))
		}
	}
	// Messages summary section.
	msgLines := []string{}
	for _, id := range agentIDs {
		agent := reg.Agents[id]
		if agent.LastMessage == nil {
			continue
		}
		msg := agent.LastMessage
		title := strings.TrimSpace(msg.Title)
		body := strings.TrimSpace(msg.Body)
		if title == "" {
			title = body
		}
		if title == "" {
			continue
		}
		msgLines = append(msgLines, "- "+id+": "+title)
	}
	if len(msgLines) > 0 {
		lines = append(lines, "\n#### Messages")
		lines = append(lines, msgLines...)
	}
	return strings.Join(lines, "\n") + "\n\n_Ctrl+] tabs · Shift+Tab swarm toggle_"
}
