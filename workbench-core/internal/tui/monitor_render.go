package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	layoutmgr "github.com/tinoosan/workbench-core/internal/tui/layout"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func (m *monitorModel) View() string {
	if m.artifactViewerOpen {
		return m.renderArtifactViewer()
	}
	grid := m.layout()
	headerLine := m.renderHeader()

	var base string
	if m.isCompactMode() {
		base = m.renderCompact(grid, headerLine)
	} else {
		base = m.renderDashboard(grid, headerLine)
	}

	// Render modal overlays on top
	if m.helpModalOpen {
		return m.renderHelpModal(base)
	}
	if m.sessionPickerOpen {
		return m.renderSessionPicker(base)
	}
	if m.newSessionWizardOpen {
		return m.renderNewSessionWizard(base)
	}
	if m.agentPickerOpen {
		return m.renderAgentPicker(base)
	}
	if m.profilePickerOpen {
		return m.renderProfilePicker(base)
	}
	if m.teamPickerOpen {
		return m.renderTeamPicker(base)
	}
	if m.modelPickerOpen {
		return m.renderModelPicker(base)
	}
	if m.reasoningEffortPickerOpen {
		return m.renderReasoningEffortPicker(base)
	}
	if m.reasoningSummaryPickerOpen {
		return m.renderReasoningSummaryPicker(base)
	}
	if m.filePickerOpen {
		return m.renderFilePicker(base)
	}

	return base
}

func (m *monitorModel) renderDashboard(grid layoutmgr.GridLayout, headerLine string) string {
	main := m.renderMainBodyDashboard(grid)
	gap := " "
	composerLeft := m.renderComposer(grid.Composer)
	composerRight := m.renderRightFooterPanel(grid.ActivityFeed.Width, grid.Composer.Height)
	composerRow := lipgloss.JoinHorizontal(lipgloss.Top, composerLeft, gap, composerRight)
	bottomBar := m.renderBottomBar(grid.ScreenWidth)
	sections := []string{headerLine, "", main, "", composerRow, bottomBar}
	if m.isDetached() {
		w := m.width
		if w <= 0 {
			w = 80
		}
		warningText := "No active context. Use /new, /sessions, or /agents."
		if m.rpcHealthKnown && !m.rpcReachable {
			warningText = "Daemon disconnected at " + strings.TrimSpace(m.rpcEndpoint) + ". Start `workbench daemon` and retry with /reconnect."
		}
		warning := m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render(warningText))
		sections = []string{headerLine, warning, "", main, "", composerRow, bottomBar}
	} else if m.runStatus != types.RunStatusRunning {
		w := m.width
		if w <= 0 {
			w = 80
		}
		warning := m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render("Agent is not active; start the daemon first or use --agent-id to attach to the running agent."))
		sections = []string{headerLine, warning, "", main, "", composerRow, bottomBar}
	}
	final := lipgloss.JoinVertical(lipgloss.Left, sections...)
	// Guarantee view never exceeds terminal width (handles m.width==0 or any section overflow).
	effectiveWidth := m.width
	if effectiveWidth <= 0 {
		effectiveWidth = 80
	}
	return lipgloss.NewStyle().MaxWidth(effectiveWidth).MaxHeight(m.height).Render(final)
}

func (m *monitorModel) renderRightFooterPanel(totalWidth, totalHeight int) string {
	if totalWidth <= 0 || totalHeight <= 0 {
		return ""
	}
	contentW := max(1, totalWidth)
	contentH := max(1, totalHeight)
	totalCost := "unknown"
	if m.stats.totalTokens == 0 {
		totalCost = "$0.0000"
	} else if m.stats.totalCostUSD > 0 {
		totalCost = fmt.Sprintf("$%.4f", m.stats.totalCostUSD)
	}
	lines := []string{
		m.styles.sectionTitle.Render("Session Stats"),
		fmt.Sprintf("Last tokens: %d (%d in + %d out)", m.stats.lastTurnTokens, m.stats.lastTurnTokensIn, m.stats.lastTurnTokensOut),
		fmt.Sprintf("Total tokens: %d (%d in + %d out)", m.stats.totalTokens, m.stats.totalTokensIn, m.stats.totalTokensOut),
		fmt.Sprintf("Last cost: %s", fallback(strings.TrimSpace(m.stats.lastTurnCostUSD), "unknown")),
		fmt.Sprintf("Total cost: %s", totalCost),
	}
	rendered := make([]string, 0, len(lines))
	for i, line := range lines {
		wrapped := wrapViewportText(line, max(10, contentW))
		for _, part := range strings.Split(wrapped, "\n") {
			part = kit.TruncateRight(strings.TrimRight(part, " \t"), max(1, contentW))
			if i > 0 {
				part = kit.StyleDim.Render(part)
			}
			rendered = append(rendered, part)
		}
	}
	if len(rendered) > contentH {
		rendered = rendered[:contentH]
	}
	return lipgloss.NewStyle().Width(contentW).MaxWidth(contentW).MaxHeight(contentH).Render(strings.Join(rendered, "\n"))
}

func (m *monitorModel) renderBottomBar(width int) string {
	w := width
	if w <= 0 {
		w = 80
	}
	uptime := "unknown"
	if !m.stats.started.IsZero() {
		uptime = time.Since(m.stats.started).Round(time.Second).String()
	}
	base := fmt.Sprintf("tasks: %d  |  uptime: %s", m.stats.tasksDone, uptime)

	controls := "Tab: focus"
	if m.stats.lastLLMErrorSet {
		retryState := "no-retry"
		if m.stats.lastLLMErrorRetryable {
			retryState = "retryable"
		}
		controls += "  |  LLM error: " + fallback(strings.TrimSpace(m.stats.lastLLMErrorClass), "unknown") + " (" + retryState + ")"
	}
	if m.isCompactMode() {
		controls += "  |  Ctrl+]/Ctrl+[ switch tab (Output | Activity | Plan | Outbox)"
	} else {
		controls += "  |  Ctrl+]/Ctrl+[ cycle side panel (Activity | Plan | Tasks | Thoughts)"
	}
	if strings.TrimSpace(m.teamID) != "" {
		controls += "  |  /team focus run  |  Ctrl+G clear focus"
	}
	line := base + "  |  " + controls
	line = kit.TruncateRight(line, max(1, w-2))
	return m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render(line))
}

func (m *monitorModel) renderHeader() string {
	content := ""
	if m.isDetached() {
		content = lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.headerTitle.Render("Workbench Control Shell "),
			kit.RenderTag(kit.TagOptions{Key: "Status", Value: "detached"}),
		)
	} else if strings.TrimSpace(m.teamID) != "" {
		content = lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.headerTitle.Render("Workbench TEAM "),
			kit.RenderTag(kit.TagOptions{Key: "Team", Value: m.teamID}),
			kit.RenderTag(kit.TagOptions{Key: "Tasks", Value: fmt.Sprintf("%d pending, %d active, %d done", m.teamPendingCount, m.teamActiveCount, m.teamDoneCount)}),
		)
		if strings.TrimSpace(m.focusedRunID) != "" {
			focusLabel := strings.TrimSpace(m.focusedRunRole)
			if focusLabel == "" {
				focusLabel = shortID(strings.TrimSpace(m.focusedRunID))
			}
			content = lipgloss.JoinHorizontal(lipgloss.Left, content, kit.RenderTag(kit.TagOptions{Key: "Focus", Value: focusLabel}))
		}
	} else {
		content = lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.headerTitle.Render("Workbench - Always On "),
			kit.RenderTag(kit.TagOptions{Key: "Agent", Value: m.runID}),
		)
	}
	w := m.width
	if w <= 0 {
		w = 80
	}
	return m.styles.header.Copy().MaxWidth(w).Render(content)
}

func (m *monitorModel) renderOutbox(spec layoutmgr.PanelSpec) string {
	body := m.outboxVP.View()
	if footer := m.renderPaginationFooter(m.outboxPage, m.outboxPageSize, m.outboxTotalCount); footer != "" {
		body = strings.TrimRight(body, "\n") + "\n" + footer
	}
	return m.panelStyle(panelOutbox).Width(spec.InnerWidth()).Height(spec.InnerHeight()).Render(
		m.styles.sectionTitle.Render("Outbox") + "\n" + body,
	)
}

func (m *monitorModel) renderMemory(spec layoutmgr.PanelSpec) string {
	return m.panelStyle(panelMemory).Width(spec.InnerWidth()).Height(spec.InnerHeight()).Render(
		m.styles.sectionTitle.Render("Memory (semantic search)") + "\n" + m.memoryVP.View(),
	)
}

func (m *monitorModel) composerStatusSegments() []string {
	modelID := strings.TrimSpace(m.model)
	if modelID == "" {
		modelID = "default"
	}
	profileRef := strings.TrimSpace(m.profile)
	if profileRef == "" {
		profileRef = "default"
	}

	tagKeyStyle := kit.CloneStyle(kit.StyleStatusKey)
	tagValueStyle := kit.CloneStyle(kit.StyleStatusValue)

	modelLabel := kit.RenderTag(kit.TagOptions{
		Key:   "model",
		Value: modelID,
		Styles: kit.TagStyles{
			KeyStyle:   tagKeyStyle,
			ValueStyle: tagValueStyle,
		},
	})
	profileLabel := kit.RenderTag(kit.TagOptions{
		Key:   "profile",
		Value: profileRef,
		Styles: kit.TagStyles{
			KeyStyle:   tagKeyStyle,
			ValueStyle: tagValueStyle,
		},
	})

	segments := []string{modelLabel}
	// Show live agent status when available, with an animated spinner for active states.
	if status := strings.TrimSpace(m.agentStatusLine); status != "" {
		display := status
		// Animate active statuses; keep terminal/warning statuses static.
		isStatic := strings.HasPrefix(status, "✓") ||
			strings.HasPrefix(status, "⚠") ||
			status == "Idle" ||
			status == "Stopped"
		if !isStatic {
			frames := []rune(statusSpinnerFrames)
			if len(frames) > 0 {
				frame := frames[m.statusAnimFrame%len(frames)]
				display = string(frame) + " " + status
			}
		}
		statusLabel := kit.RenderTag(kit.TagOptions{
			Key:   "status",
			Value: display,
			Styles: kit.TagStyles{
				KeyStyle:   tagKeyStyle,
				ValueStyle: tagValueStyle,
			},
		})
		segments = append(segments, statusLabel)
	}
	if cost.SupportsReasoningSummary(modelID) {
		effort := strings.TrimSpace(m.reasoningEffort)
		if effort == "" {
			effort = "medium"
		}
		summary := strings.TrimSpace(m.reasoningSummary)
		if summary == "" {
			summary = "auto"
		}
		reasoningEffortLabel := kit.RenderTag(kit.TagOptions{
			Key:   "reasoning-effort",
			Value: effort,
			Styles: kit.TagStyles{
				KeyStyle:   tagKeyStyle,
				ValueStyle: tagValueStyle,
			},
		})
		reasoningSummaryLabel := kit.RenderTag(kit.TagOptions{
			Key:   "reasoning-summary",
			Value: summary,
			Styles: kit.TagStyles{
				KeyStyle:   tagKeyStyle,
				ValueStyle: tagValueStyle,
			},
		})
		segments = append(segments, reasoningSummaryLabel, reasoningEffortLabel)
	}
	segments = append(segments, profileLabel)
	if mc := m.teamModelChange; mc != nil && strings.EqualFold(strings.TrimSpace(mc.Status), "pending") {
		targetModel := strings.TrimSpace(mc.RequestedModel)
		if targetModel != "" {
			modelChangeLabel := kit.RenderTag(kit.TagOptions{
				Key:   "model-change",
				Value: "pending -> " + kit.TruncateMiddle(targetModel, 16),
				Styles: kit.TagStyles{
					KeyStyle:   tagKeyStyle,
					ValueStyle: tagValueStyle,
				},
			})
			segments = append(segments, modelChangeLabel)
		}
	}
	return segments
}

func wrapComposerStatusSegments(segments []string, contentW int) []string {
	lines := make([]string, 0, 2)
	current := ""
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if current == "" {
			current = seg
			continue
		}
		candidate := current + "  " + seg
		if lipgloss.Width(candidate) <= contentW {
			current = candidate
			continue
		}
		lines = append(lines, current)
		current = seg
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

func (m *monitorModel) composerStatusText(contentW int) (string, int) {
	segments := m.composerStatusSegments()
	contentW = max(10, contentW)
	// On narrow widths keep the composer header to a single line so the input area remains obvious.
	if contentW <= 72 {
		line := ""
		for _, seg := range segments {
			if seg == "" {
				continue
			}
			candidate := seg
			if line != "" {
				candidate = line + "  " + seg
			}
			if lipgloss.Width(candidate) > contentW {
				break
			}
			line = candidate
		}
		if line == "" && len(segments) > 0 {
			line = segments[0]
		}
		return kit.TruncateRight(line, contentW), 1
	}
	lines := wrapComposerStatusSegments(segments, contentW)
	return strings.Join(lines, "\n"), len(lines)
}

func (m *monitorModel) composerContentLineCount(contentW int) int {
	_, statusLines := m.composerStatusText(contentW)
	contentLines := statusLines
	if palette := m.renderCommandPalette(contentW); palette != "" {
		contentLines += 1 + lipgloss.Height(palette)
	}
	inputH := m.input.Height()
	if inputH < 1 {
		inputH = 1
	}
	contentLines += inputH
	return max(1, contentLines)
}

func (m *monitorModel) renderComposer(spec layoutmgr.PanelSpec) string {
	contentW := max(20, spec.ContentWidth)
	statusText, _ := m.composerStatusText(contentW)
	status := lipgloss.NewStyle().Width(contentW).Render(statusText)

	// Build content parts: status, palette (if open), input
	contentParts := []string{status}

	// Render command palette if open
	if palette := m.renderCommandPalette(contentW); palette != "" {
		contentParts = append(contentParts, "", palette)
	}

	contentParts = append(contentParts, m.input.View())

	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	return m.commandBarStyle().
		Width(spec.InnerWidth()).
		Height(spec.InnerHeight()).
		Render(content)
}

func (m *monitorModel) updateFocus() {
	if m.focusedPanel == panelComposer {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

func (m *monitorModel) commandBarStyle() lipgloss.Style {
	if m.focusedPanel == panelComposer {
		return m.styles.panelFocused
	}
	return m.styles.commandBar
}

func (m *monitorModel) renderPaginationFooter(page, pageSize, totalCount int) string {
	if totalCount <= 0 || pageSize <= 0 {
		return ""
	}

	totalPages := (totalCount + pageSize - 1) / pageSize
	if totalPages <= 1 {
		return ""
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page*pageSize + 1
	end := start + pageSize - 1
	if end > totalCount {
		end = totalCount
	}

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Faint(true)
	return style.Render(fmt.Sprintf(
		"%d-%d of %d | Page %d/%d | n/→ next  p/← prev  g first  G last",
		start, end, totalCount, page+1, totalPages,
	))
}

func hasPaginationFooter(pageSize, totalCount int) bool {
	if totalCount <= 0 || pageSize <= 0 {
		return false
	}
	totalPages := (totalCount + pageSize - 1) / pageSize
	return totalPages > 1
}

func (m *monitorModel) inboxViewportContent(width int) string {
	return renderInbox(m.inboxList)
}

func (m *monitorModel) outboxViewportContent(width int) string {
	return renderOutboxLines(m.outboxResults, m.renderer, width)
}

func renderInbox(tasks []taskState) string {
	if len(tasks) == 0 {
		return kit.StyleDim.Render("No pending inbox tasks.")
	}
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		id := strings.TrimSpace(task.TaskID)
		if id == "" {
			continue
		}
		goal := truncateText(task.Goal, 48)
		// Use bullet + bold ID for better visual hierarchy
		line := "• " + kit.StyleBold.Render(shortID(id))
		if role := strings.TrimSpace(task.AssignedRole); role != "" {
			line += " " + kit.StyleDim.Render("["+role+"]")
		}
		if strings.TrimSpace(task.Status) != "" && strings.TrimSpace(task.Status) != string(types.TaskStatusPending) {
			line += " " + kit.StyleDim.Render("["+strings.TrimSpace(task.Status)+"]")
		}
		if strings.Contains(strings.ToLower(task.Goal), "batch review only") {
			line += " " + kit.StyleDim.Render("[batch]")
			if strings.Contains(strings.ToLower(task.Goal), "[partial]") {
				line += " " + kit.StyleDim.Render("[partial]")
			}
		}
		if goal != "" {
			line += " — " + goal
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderOutboxLines(results []outboxEntry, renderer *ContentRenderer, width int) string {
	_ = renderer
	_ = width
	if len(results) == 0 {
		return kit.StyleDim.Render("No completed tasks yet.")
	}
	lines := make([]string, 0, len(results))
	for _, r := range results {
		goal := truncateText(r.Goal, 50)
		status := r.Status
		if status == "" {
			status = "unknown"
		}

		// Color-code status for quick visual scanning
		statusStr := status
		switch status {
		case "succeeded":
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Render(status)
		case "failed", "quarantined", "canceled":
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Render(status)
		}

		// Header: bullet + bold ID + dim goal -> colored status (+ cost/tokens).
		metaParts := make([]string, 0, 2)
		if r.CostUSD > 0 {
			metaParts = append(metaParts, fmt.Sprintf("$%.4f", r.CostUSD))
		}
		if r.TotalTokens > 0 {
			metaParts = append(metaParts, fmt.Sprintf("%d tok", r.TotalTokens))
		}
		meta := ""
		if len(metaParts) != 0 {
			meta = " " + kit.StyleDim.Render("("+strings.Join(metaParts, " • ")+")")
		}
		if strings.Contains(strings.ToLower(r.Goal), "batch review only") {
			meta += " " + kit.StyleDim.Render("[batch]")
		}
		header := "• " + kit.StyleBold.Render(shortID(r.TaskID)) + " " +
			kit.StyleDim.Render("\""+goal+"\"") + " → " + statusStr + meta
		if role := strings.TrimSpace(r.AssignedRole); role != "" {
			header = "• " + kit.StyleBold.Render(shortID(r.TaskID)) + " " + kit.StyleDim.Render("["+role+"] ") +
				kit.StyleDim.Render("\""+goal+"\"") + " → " + statusStr + meta
		}
		lines = append(lines, header)

		if strings.TrimSpace(r.Error) != "" && (status == "failed" || status == "canceled" || status == "quarantined") {
			lines = append(lines, "  └ "+lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Render("error: "+strings.TrimSpace(r.Error)))
		}
		if r.ArtifactsCount > 0 || strings.TrimSpace(r.SummaryPath) != "" {
			info := fmt.Sprintf("  └ deliverables: %d", r.ArtifactsCount)
			if strings.TrimSpace(r.SummaryPath) != "" {
				info += " (summary: " + r.SummaryPath + ")"
			}
			lines = append(lines, info)
		}
		if r.TotalTokens > 0 || r.CostUSD > 0 {
			parts := make([]string, 0, 2)
			if r.TotalTokens > 0 {
				parts = append(parts, fmt.Sprintf("tokens: %d (%d in + %d out)", r.TotalTokens, r.InputTokens, r.OutputTokens))
			}
			if r.CostUSD > 0 {
				parts = append(parts, fmt.Sprintf("cost: $%.4f", r.CostUSD))
			}
			if len(parts) != 0 {
				lines = append(lines, "  └ "+strings.Join(parts, " • "))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func renderMemResults(results []string) string {
	if len(results) == 0 {
		return "No memory results."
	}
	return strings.Join(results, "\n")
}
