package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/orchestrator"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func (m *Model) refreshSwarmView() {
	if !m.showDetails || m.swarmViewport.Width == 0 {
		return
	}
	w := max(24, m.swarmViewport.Width-4)

	reg := orchestrator.Registry{}
	metrics := orchestrator.Metrics{}
	regErr := ""
	metricsErr := ""
	regFromProvider := false
	metricsFromProvider := false
	if provider, ok := m.runner.(swarmRegistryProvider); ok {
		if cached, err := provider.GetSwarmRegistry(); err == nil {
			reg = cached
			regFromProvider = true
		} else {
			regErr = err.Error()
		}
		if cached, err := provider.GetSwarmMetrics(); err == nil {
			metrics = cached
			metricsFromProvider = true
		} else {
			metricsErr = err.Error()
		}
	}
	if !regFromProvider {
		reg, regErr = m.loadSwarmRegistry()
	}
	if !metricsFromProvider {
		metrics, metricsErr = m.loadSwarmMetrics()
	}

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
	} else if strings.TrimSpace(metricsErr) != "VFS access unavailable" {
		lines = append(lines, fmt.Sprintf("_Metrics unavailable: %s_\n", metricsErr))
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
			lines = append(lines, "  - goal: "+truncateSwarm(strings.TrimSpace(agent.CurrentGoal), 120))
		}
		if agent.Stats.CostUSD > 0 || agent.Stats.TokensIn > 0 || agent.Stats.TokensOut > 0 {
			lines = append(lines, fmt.Sprintf("  - usage: %d in · %d out · $%.4f", agent.Stats.TokensIn, agent.Stats.TokensOut, agent.Stats.CostUSD))
		}
	}
	// Messages summary section.
	msgLines := []string{}
	for _, id := range agentIDs {
		agent := reg.Agents[id]
		if agent.LastInboxMsg != nil {
			msg := agent.LastInboxMsg
			if title := messageTitle(msg); title != "" {
				msgLines = append(msgLines, "- to "+id+": "+truncateSwarm(title, 120))
			}
		}
		if agent.LastMessage != nil {
			msg := agent.LastMessage
			if title := messageTitle(msg); title != "" {
				msgLines = append(msgLines, "- from "+id+": "+truncateSwarm(title, 120))
			}
		}
	}
	if len(msgLines) > 0 {
		lines = append(lines, "\n#### Messages")
		lines = append(lines, msgLines...)
	}
	return strings.Join(lines, "\n") + "\n\n_Ctrl+] tabs · Shift+Tab swarm toggle_"
}

func messageTitle(msg *types.Message) string {
	if msg == nil {
		return ""
	}
	title := strings.TrimSpace(msg.Title)
	if title != "" {
		return title
	}
	return strings.TrimSpace(msg.Body)
}

func truncateSwarm(text string, max int) string {
	if max <= 0 {
		return ""
	}
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max-1]) + "…"
}
