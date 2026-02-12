package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

func (m *monitorModel) appendAgentOutput(line string) {
	m.appendAgentOutputForRun(line, "")
}

func (m *monitorModel) appendAgentOutputForRun(line, runID string) {
	_ = m.appendAgentOutputLine(line, runID)
}

func (m *monitorModel) appendAgentOutputLine(line, runID string) int {
	line = strings.TrimSpace(line)
	if line == "" {
		return -1
	}
	m.agentOutput = append(m.agentOutput, line)
	m.agentOutputRunID = append(m.agentOutputRunID, strings.TrimSpace(runID))
	m.trimAgentOutputBuffer()
	m.dirtyAgentOutput = true
	// Incrementally maintain layout metadata when possible (for virtualization).
	w := m.agentOutputVP.Width
	if w <= 0 {
		w = 80
	}
	// If width changed or metadata is out of sync, fall back to recompute on next refresh.
	if m.agentOutputLayoutWidth != w || len(m.agentOutputLineStarts) != len(m.agentOutput)-1 || len(m.agentOutputLineHeights) != len(m.agentOutput)-1 {
		m.agentOutputLayoutWidth = 0
		return len(m.agentOutput) - 1
	}
	start := m.agentOutputTotalLines
	h := len(m.renderAgentOutputLogicalLines(line, w))
	if h < 1 {
		h = 1
	}
	m.agentOutputLineStarts = append(m.agentOutputLineStarts, start)
	m.agentOutputLineHeights = append(m.agentOutputLineHeights, h)
	m.agentOutputTotalLines += h
	return len(m.agentOutput) - 1
}

func (m *monitorModel) appendThinkingEntry(runID, role string, summary string) {
	if m == nil {
		return
	}
	summary = normalizeThinkingSummary(summary)
	if summary == "" {
		return
	}
	m.thinkingEntries = append(m.thinkingEntries, thinkingEntry{
		RunID:   strings.TrimSpace(runID),
		Role:    strings.TrimSpace(role),
		Summary: summary,
	})
	if maxThinkingEntries > 0 && len(m.thinkingEntries) > maxThinkingEntries {
		start := len(m.thinkingEntries) - maxThinkingEntries
		m.thinkingEntries = append([]thinkingEntry(nil), m.thinkingEntries[start:]...)
	}
	m.dirtyThinking = true
}

func normalizeThinkingSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	// Some providers emit adjacent reasoning sections without a separator, e.g.
	// "...in planning.Listing capabilities". Split those into distinct blocks.
	summary = gluedReasoningSectionRE.ReplaceAllString(summary, "$1\n\n$2$3")
	summary = strings.ReplaceAll(summary, "\r\n", "\n")
	for strings.Contains(summary, "\n\n\n") {
		summary = strings.ReplaceAll(summary, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(summary)
}

func reasoningStepKey(runID, role, step string) string {
	return strings.TrimSpace(runID) + "|" + strings.TrimSpace(role) + "|" + strings.TrimSpace(step)
}

func (m *monitorModel) trimAgentOutputBuffer() {
	if m == nil {
		return
	}
	maxLines := agentOutputMaxLines
	if maxLines <= 0 {
		return
	}
	if len(m.agentOutput) <= maxLines {
		return
	}

	// Drop a chunk at a time to amortize copying costs once we hit the limit.
	drop := len(m.agentOutput) - (maxLines - agentOutputDropChunk)
	if agentOutputDropChunk <= 0 {
		drop = len(m.agentOutput) - maxLines
	}
	if drop <= 0 {
		drop = 1
	}
	if drop >= len(m.agentOutput) {
		m.agentOutput = nil
		m.agentOutputRunID = nil
		m.agentOutputFilteredCache = nil
		m.agentOutputPending = nil
		m.agentOutputPendingFallback = nil
		m.agentOutputLayoutWidth = 0
		m.agentOutputLogicalYOffset = 0
		m.agentOutputFollow = true
		return
	}

	removedLogicalLines := 0
	// appendAgentOutputLine() trims before it appends incremental layout metadata, so
	// lineHeights may be for the previous length (len(agentOutput)-1) or the current length.
	if m.agentOutputLayoutWidth != 0 && (len(m.agentOutputLineHeights) == len(m.agentOutput) || len(m.agentOutputLineHeights) == len(m.agentOutput)-1) && drop <= len(m.agentOutputLineHeights) {
		for i := 0; i < drop; i++ {
			removedLogicalLines += m.agentOutputLineHeights[i]
		}
	}

	// Re-slice into a fresh backing array so we don't retain references to the dropped prefix.
	kept := append([]string(nil), m.agentOutput[drop:]...)
	m.agentOutput = kept
	if drop >= len(m.agentOutputRunID) {
		m.agentOutputRunID = nil
	} else {
		m.agentOutputRunID = append([]string(nil), m.agentOutputRunID[drop:]...)
	}
	m.agentOutputFilteredCache = nil

	// Adjust pending op indices so responses still update the correct lines when possible.
	if m.agentOutputPending != nil {
		for opID, entry := range m.agentOutputPending {
			entry.index -= drop
			if entry.index < 0 {
				delete(m.agentOutputPending, opID)
				continue
			}
			m.agentOutputPending[opID] = entry
		}
		if len(m.agentOutputPending) == 0 {
			m.agentOutputPending = nil
		}
	}
	if m.agentOutputPendingFallback != nil {
		e := *m.agentOutputPendingFallback
		e.index -= drop
		if e.index < 0 {
			m.agentOutputPendingFallback = nil
		} else {
			m.agentOutputPendingFallback = &e
		}
	}

	// Preserve scroll position as best-effort by shifting the logical y-offset down by the number
	// of wrapped lines we removed (when known). Always clamp on refresh.
	if removedLogicalLines > 0 && m.agentOutputLogicalYOffset > 0 {
		m.agentOutputLogicalYOffset = max(0, m.agentOutputLogicalYOffset-removedLogicalLines)
	}

	// Invalidate layout metadata; it no longer matches after dropping a prefix.
	m.agentOutputLayoutWidth = 0
}

func (m *monitorModel) takeAgentOutputPending(opID string) (agentOutputPendingEntry, bool) {
	if opID != "" && len(strings.TrimSpace(opID)) > 0 {
		if m.agentOutputPending != nil {
			if entry, ok := m.agentOutputPending[opID]; ok {
				delete(m.agentOutputPending, opID)
				return entry, true
			}
		}
		return agentOutputPendingEntry{}, false
	}
	if m.agentOutputPendingFallback != nil {
		entry := *m.agentOutputPendingFallback
		m.agentOutputPendingFallback = nil
		return entry, true
	}
	return agentOutputPendingEntry{}, false
}

func outputLineStyle(line string) lipgloss.Style {
	line = strings.TrimSpace(line)
	if line == "" {
		return lipgloss.NewStyle()
	}
	// formatEventLine: "[15:04:05] <type>: <message>"
	eventType := ""
	if strings.HasPrefix(line, "[") {
		if end := strings.Index(line, "]"); end != -1 {
			inside := strings.TrimSpace(line[1:end])
			rest := strings.TrimSpace(line[end+1:])
			// If the bracket looks like a timestamp, parse the event type after it.
			if strings.Count(inside, ":") >= 2 {
				if strings.HasPrefix(rest, "[") {
					if rb := strings.Index(rest, "]"); rb != -1 && len(rest) > rb+1 {
						rest = strings.TrimSpace(rest[rb+1:])
					}
				}
				if colon := strings.Index(rest, ":"); colon != -1 {
					eventType = strings.TrimSpace(rest[:colon])
				}
			} else {
				// Monitor-local status lines like "[error]" or "[control] ..."
				eventType = inside
				if eventType == "queued" {
					eventType = "task.queued"
				}
			}
		}
	}
	switch eventType {
	case "error", "daemon.error", "daemon.runner.error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	case "agent.error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	case "task.done", "task.delivered":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950"))
	case "task.start", "task.queued":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6bbcff"))
	case "control", "control.check", "control.success", "control.error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922"))
	case "agent.turn.complete":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950"))
	case "daemon.start", "daemon.stop", "daemon.control", "daemon.warning":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#a371f7"))
	case "task.quarantined":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	default:
		return kit.StyleDim
	}
}

func (m *monitorModel) currentAgentOutputLines() []string {
	if m == nil {
		return nil
	}
	if strings.TrimSpace(m.teamID) == "" || strings.TrimSpace(m.focusedRunID) == "" {
		return m.agentOutput
	}
	targetRunID := strings.TrimSpace(m.focusedRunID)
	out := make([]string, 0, len(m.agentOutput))
	for i, line := range m.agentOutput {
		if i >= len(m.agentOutputRunID) {
			break
		}
		entryRunID := strings.TrimSpace(m.agentOutputRunID[i])
		if entryRunID != "" && entryRunID != targetRunID {
			continue
		}
		out = append(out, line)
	}
	m.agentOutputFilteredCache = out
	return m.agentOutputFilteredCache
}

func (m *monitorModel) renderAgentOutputLogicalLines(rawLine string, width int) []string {
	rawLine = strings.TrimSpace(rawLine)
	if rawLine == "" {
		return []string{""}
	}
	if summary, ok := parseAgentOutputSummaryLine(rawLine); ok {
		rendered := summary
		if m.renderer != nil {
			rendered = m.renderer.RenderAgentMarkdown(summary, width)
		}
		// Guard rail for narrow terminals: ensure markdown output never exceeds viewport width.
		rendered = wrapViewportText(rendered, max(10, width))
		rendered = strings.TrimRight(rendered, "\n")
		if rendered == "" {
			return []string{""}
		}
		return strings.Split(rendered, "\n")
	}
	style := outputLineStyle(rawLine)
	wrapped := wordwrap.String(rawLine, width)
	out := make([]string, 0, 1+strings.Count(wrapped, "\n"))
	for _, sub := range strings.Split(wrapped, "\n") {
		sub = strings.TrimRight(sub, " ")
		if sub == "" {
			out = append(out, "")
			continue
		}
		out = append(out, style.Render(sub))
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}
