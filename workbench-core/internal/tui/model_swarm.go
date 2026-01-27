package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

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

	content := m.renderSwarmContent(reg, regErr, metrics, metricsErr)
	if strings.TrimSpace(content) == "" {
		content = "_Swarm view is preparing…_"
	}
	m.swarmViewport.SetContent(strings.TrimRight(m.renderer.RenderMarkdown(content, w), "\n"))
}

func (m *Model) loadSwarmRegistry() (orchestrator.Registry, string) {
	accessor, ok := m.runner.(vfsAccessor)
	if !ok {
		return orchestrator.Registry{}, "VFS access unavailable"
	}
	text, _, _, err := accessor.ReadVFS(m.ctx, "/agents/registry.json", 128*1024)
	if err != nil {
		return orchestrator.Registry{}, err.Error()
	}
	if strings.TrimSpace(text) == "" {
		return orchestrator.Registry{}, ""
	}
	var reg orchestrator.Registry
	if err := json.Unmarshal([]byte(text), &reg); err != nil {
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
		return orchestrator.Metrics{}, err.Error()
	}
	if strings.TrimSpace(text) == "" {
		return orchestrator.Metrics{}, ""
	}
	var metrics orchestrator.Metrics
	if err := json.Unmarshal([]byte(text), &metrics); err != nil {
		return orchestrator.Metrics{}, "invalid metrics.json"
	}
	return metrics, ""
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
		if agent.Stats.CostUSD > 0 || agent.Stats.TokensIn > 0 || agent.Stats.TokensOut > 0 {
			lines = append(lines, fmt.Sprintf("  - usage: %d in · %d out · $%.4f", agent.Stats.TokensIn, agent.Stats.TokensOut, agent.Stats.CostUSD))
		}
	}
	return strings.Join(lines, "\n") + "\n\n_Ctrl+] tabs · Shift+Tab swarm toggle_"
}
