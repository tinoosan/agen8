package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

func (m *monitorModel) appendAgentOutput(line string) {
	m.appendAgentOutputItem(AgentOutputItem{
		Timestamp: time.Now(),
		Type:      "info",
		Content:   line,
	})
}

func (m *monitorModel) appendAgentOutputForRun(line, runID string) {
	m.appendAgentOutputItem(AgentOutputItem{
		Timestamp: time.Now(),
		Type:      "info",
		Content:   line,
		RunID:     runID,
	})
}

func (m *monitorModel) appendAgentOutputItem(item AgentOutputItem) int {
	item.Content = strings.TrimSpace(item.Content)
	if item.Content == "" {
		return -1
	}
	m.agentOutput = append(m.agentOutput, item)
	m.agentOutputRunID = append(m.agentOutputRunID, strings.TrimSpace(item.RunID))
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
	h := len(m.renderAgentOutputItemLogicalLines(item, w))
	if h < 1 {
		h = 1
	}
	m.agentOutputLineStarts = append(m.agentOutputLineStarts, start)
	m.agentOutputLineHeights = append(m.agentOutputLineHeights, h)
	m.agentOutputTotalLines += h
	return len(m.agentOutput) - 1
}

func (m *monitorModel) appendAgentOutputLine(line, runID string) int {
	return m.appendAgentOutputItem(AgentOutputItem{
		Timestamp: time.Now(),
		Type:      "info",
		Content:   line,
		RunID:     runID,
	})
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
	kept := append([]AgentOutputItem(nil), m.agentOutput[drop:]...)
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
	case "error", "daemon.error", "daemon.runner.error", "agent.error", "task.quarantined":
		return kit.StyleErr
	case "task.done", "task.delivered", "agent.turn.complete":
		return kit.StyleOK
	case "task.start", "task.queued":
		return kit.StyleAccent
	case "control", "control.check", "control.success", "control.error":
		return kit.StylePending
	case "daemon.start", "daemon.stop", "daemon.control", "daemon.warning", "run.start":
		return kit.StyleThinking
	default:
		return kit.StyleDim
	}
}

func (m *monitorModel) currentAgentOutputItems() []AgentOutputItem {
	if m == nil {
		return nil
	}
	targetRunID := strings.TrimSpace(m.focusedRunID)
	out := make([]AgentOutputItem, 0, len(m.agentOutput))
	for _, item := range m.agentOutput {
		if targetRunID != "" {
			itemRunID := strings.TrimSpace(item.RunID)
			if itemRunID != "" && itemRunID != targetRunID {
				continue
			}
		}
		if !m.showThoughts && item.Type == "thought" {
			continue
		}
		out = append(out, item)
	}
	m.agentOutputFilteredCache = out
	return m.agentOutputFilteredCache
}

func (m *monitorModel) renderAgentOutputItemLogicalLines(item AgentOutputItem, width int) []string {
	content := strings.TrimSpace(item.Content)
	if content == "" {
		return []string{""}
	}

	// For now, reuse the existing logic but wrap the item data.
	// In the next step, we'll implement richer rendering based on item.Type.

	if summary, ok := parseAgentOutputSummaryLine(content); ok {
		rendered := summary
		if m.renderer != nil {
			rendered = m.renderer.RenderAgentMarkdown(summary, width)
		}
		rendered = wrapViewportText(rendered, max(10, width))
		rendered = strings.TrimRight(rendered, "\n")
		if rendered == "" {
			return []string{""}
		}
		return strings.Split(rendered, "\n")
	}

	style := outputLineStyle(content)
	prefix := ""
	if item.Type == "thought" {
		// Use the specialized markdown renderer for thoughts
		content = m.renderer.RenderThinkingMarkdown(content, width-4) // -4 for margin/prefix

		// The markdown renderer handles wrapping, so we might not need wordwrap.String here
		// IF we pass the correct width. However, RenderThinkingMarkdown returns a string
		// that might contain ANSI codes and newlines.
		// We need to split it into lines.
		// The loop below handles splitting by newline, so we just need to ensure 'content'
		// is the fully rendered markdown.

		// Add timestamp prefix to the first line if needed, but for markdown blocks
		// it might be better to visually separate the timestamp or put it in a header.
		// For now, let's keep the timestamp logic simple:
		// We'll prepend the timestamp to the *first line* of the rendered markdown?
		// Or maybe render it separately?
		// Let's stick to the prefix approach for consistency, but apply it carefully.

		ts := fmt.Sprintf("[%s] ", item.Timestamp.Local().Format("15:04:05"))
		// We can't easily prepend to ANSI-styled markdown without breaking layout sometimes.
		// A safer bet: rendered markdown is a block. We can put the timestamp above it
		// or try to hack it in.
		// Given the user wants "just the thoughts", a subtle timestamp above or to the left is good.
		// Let's prepend it to the raw content before rendering? No, that messes up markdown parsing.

		// Let's render the markdown first.
		rendered := m.renderer.RenderThinkingMarkdown(item.Content, width-len(ts))

		// Now we have a block of text. We want the first line to have the timestamp.
		// But 'rendered' has ANSI codes.
		// Let's simply output the timestamp as a separate logical line (or just part of the first line).
		// Actually, let's just use the rendered content. The user wants "thoughts or nothing".
		// We can put the timestamp in the metadata or just style it simply.

		// Let's prepend the timestamp to the raw text inside a blockquote or bold?
		// No, let's stick to the rendered output.
		content = rendered

		// We previously added a prefix. Let's re-add it but we need to be careful.
		// If we just set style/prefix, the logic below does:
		// if prefix != "" { content = prefix + content }
		// wrapped := wordwrap.String(content, width)

		// We DO NOT want to wrap already-rendered markdown (glamour does wrapping).
		// So for "thought", we should bypass the wordwrap logic below.

		out := make([]string, 0, 1+strings.Count(content, "\n"))
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			// Trim right to avoid trailing spaces from glamour's padding
			line = strings.TrimRight(line, " ")
			if i == 0 {
				line = kit.StyleDim.Italic(true).Render(ts) + line
			} else {
				// indention to align with timestamp?
				line = strings.Repeat(" ", len(ts)) + line
			}
			out = append(out, line)
		}
		return out

	} else if item.Type == "error" {
		style = kit.StyleErr
	} else if item.Type == "user" {
		style = kit.StyleBold.Foreground(kit.ColorAccent)
		prefix = fmt.Sprintf("[%s] ", item.Timestamp.Local().Format("15:04:05"))
	} else if item.Type == "tool_call" || item.Type == "tool_result" {
		prefix = fmt.Sprintf("[%s] ", item.Timestamp.Local().Format("15:04:05"))
	}

	// For thought/user/tool items, we want the timestamp to be part of the first line
	// but styled consistently (or dim). Let's just prefix it to the content for wrapping.
	if prefix != "" {
		content = prefix + content
	}

	wrapped := wordwrap.String(content, width)
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
