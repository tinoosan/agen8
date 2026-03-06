package mail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/wordwrap"
	tuishared "github.com/tinoosan/agen8/internal/tui"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

var (
	styleAccent    = lipgloss.NewStyle().Foreground(kit.BorderColorAccent)
	styleHeader    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#eaeaea"))
	styleSection   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9ad0ff"))
	detailRenderer = tuishared.NewContentRenderer()
)

const (
	compactWidth        = 60
	compactWidthProject = 80
	smallHeight         = 14
	mediumHeight        = 20
)

func (m *Model) isNarrow() bool  { return m.width < compactWidth }
func (m *Model) isCompact() bool { return m.width < compactWidthProject }
func (m *Model) isShort() bool   { return m.height < smallHeight }
func (m *Model) isMedium() bool  { return m.height < mediumHeight }

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	switch m.mode {
	case viewProject:
		return m.renderProjectView()
	case viewTeam:
		// fall through to existing rendering
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	bodyHeight := m.height - 2 // header + footer
	if bodyHeight < 1 {
		// Tiny terminal heights can only show part of header/footer; clamp to viewport.
		out := header + "\n" + footer
		return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
	}

	body := m.renderBody(bodyHeight)
	// Keep body within its allocated space so footer remains visible.
	body = lipgloss.NewStyle().MaxHeight(bodyHeight).Render(body)

	out := header + "\n" + body + "\n" + footer
	return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
}

func (m *Model) renderHeader() string {
	if m.isNarrow() {
		status := kit.StyleOK.Render("● connected")
		if !m.connected {
			status = kit.StyleErr.Render("● disconnected")
		}

		left := styleHeader.Render("mail") + kit.StyleDim.Render(" · ") + status
		if m.lastErr != "" {
			left += kit.StyleDim.Render(" · ") + kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 20))
		}
		if m.notice != "" {
			left += kit.StyleDim.Render(" · ") + kit.StylePending.Render(kit.Truncate(m.notice, 18))
		}

		return lipgloss.NewStyle().
			Width(m.width).
			MaxWidth(m.width).
			MaxHeight(1).
			Foreground(lipgloss.Color("#eaeaea")).
			Padding(0, 1).
			Render(left)
	}

	tid := ""
	if m.selectedTeam != nil {
		tid = strings.TrimSpace(m.selectedTeam.TeamID)
	}
	if tid == "" {
		tid = strings.TrimSpace(m.sessionID)
	}
	if len(tid) > 12 {
		tid = tid[:12]
	}

	status := kit.StyleOK.Render("● connected")
	if !m.connected {
		status = kit.StyleErr.Render("● disconnected")
	}

	counts := fmt.Sprintf("inbox:%d  outbox:%d", len(m.inbox), len(m.outbox))

	prefix := ""
	if m.selectedTeam != nil {
		prefix = kit.StyleDim.Render("← ") + kit.StyleAccent.Render(teamShortLabel(*m.selectedTeam)) + kit.StyleDim.Render("  ·  ")
	}

	left := prefix + styleHeader.Render("agen8 mail") +
		kit.StyleDim.Render("  ·  team: ") + styleAccent.Render(kit.Fallback(tid, "-")) +
		kit.StyleDim.Render("  ·  ") + status +
		kit.StyleDim.Render("  ·  ") + kit.StyleDim.Render(counts)

	if m.lastErr != "" {
		left += kit.StyleDim.Render("  ·  ") + kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 40))
	}
	if m.notice != "" {
		left += kit.StyleDim.Render("  ·  ") + kit.StylePending.Render(kit.Truncate(m.notice, 28))
	}

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Foreground(lipgloss.Color("#eaeaea")).
		Padding(0, 1).
		Render(left)
}

func (m *Model) renderFooter() string {
	backKey := "q"
	backLabel := "quit"
	if m.selectedTeam != nil {
		backKey = "esc"
		backLabel = "back"
	}

	teamNav := ""
	if m.selectedTeam != nil && !m.isNarrow() {
		teamNav = kit.StyleDim.Render("[/]") + " prev/next team  "
	}

	if m.isNarrow() {
		hints := kit.StyleDim.Render("tab") + " " +
			kit.StyleDim.Render("j/k") + " " +
			kit.StyleDim.Render("space") + " " +
			kit.StyleDim.Render("↵") + " " +
			kit.StyleDim.Render("esc") + " " +
			kit.StyleDim.Render("r") + " " +
			kit.StyleDim.Render(backKey)

		return lipgloss.NewStyle().
			Width(m.width).
			MaxWidth(m.width).
			MaxHeight(1).
			Padding(0, 1).
			Render(hints)
	}

	hints := kit.StyleDim.Render("tab") + " focus  " +
		kit.StyleDim.Render("j/k") + " scroll  " +
		kit.StyleDim.Render("space") + " expand  " +
		kit.StyleDim.Render("enter") + " detail  " +
		teamNav +
		kit.StyleDim.Render("r") + " refresh  " +
		kit.StyleDim.Render(backKey) + " " + backLabel

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Padding(0, 1).
		Render(hints)
}

func (m *Model) renderBody(height int) string {
	innerW := m.width - 2 // account for panel borders

	if m.isShort() {
		if m.detailOpen && m.selectedTask() != nil {
			return m.renderDetailPanel(innerW, height)
		}
		return m.renderFocusedOnly(m.width, height)
	}

	var currentPanel string
	remaining := height

	if !m.isMedium() {
		currentH := 6
		if currentH > height-2 {
			currentH = height - 2
		}
		if currentH < 3 {
			currentH = 3 // border + 1 line minimum
		}
		remaining = height - currentH
		if remaining < 2 {
			remaining = 2
		}
		currentPanel = m.renderCurrentTask(innerW, currentH)
	}

	if m.detailOpen {
		if m.selectedTask() != nil {
			detailPanel := m.renderDetailPanel(innerW, remaining)
			if currentPanel != "" {
				return lipgloss.JoinVertical(lipgloss.Left, currentPanel, detailPanel)
			}
			return detailPanel
		}
		// No task selected — fall through to normal inbox+outbox layout
		m.detailOpen = false
	}

	// Split remaining 50/50 between inbox and outbox
	inboxH := remaining / 2
	outboxH := remaining - inboxH

	inboxPanel := m.renderInboxPanel(innerW, inboxH)
	outboxPanel := m.renderOutboxPanel(innerW, outboxH)

	if currentPanel != "" {
		return lipgloss.JoinVertical(lipgloss.Left, currentPanel, inboxPanel, outboxPanel)
	}
	return lipgloss.JoinVertical(lipgloss.Left, inboxPanel, outboxPanel)
}

func (m *Model) renderCurrentTask(width, height int) string {
	title := styleSection.Render("Current Task")
	body := kit.StyleDim.Render("No active task")

	if m.currentTask != nil {
		t := m.currentTask
		elapsed := time.Since(t.CreatedAt).Round(time.Second)
		contentW := max(10, width-14)
		body = strings.Join([]string{
			kit.StyleStatusKey.Render("Goal:    ") + kit.StyleStatusValue.Render(kit.Truncate(t.Goal, contentW)),
			kit.StyleStatusKey.Render("Status:  ") + renderStatus(t.Status) + kit.StyleDim.Render("   Role: ") + kit.StyleStatusValue.Render(kit.Fallback(t.Role, "-")),
			kit.StyleStatusKey.Render("Started: ") + t.CreatedAt.Format("15:04:05") + kit.StyleDim.Render("   Elapsed: ") + elapsed.String(),
		}, "\n")
	}

	border := lipgloss.RoundedBorder()
	return lipgloss.NewStyle().
		Border(border).
		BorderForeground(kit.BorderColorDefault).
		Width(width).
		Height(height - 2). // subtract border
		Render(title + "\n" + body)
}

func (m *Model) renderFocusedOnly(width, height int) string {
	if m.focus == panelInbox {
		title := styleSection.Render(fmt.Sprintf("Inbox (%d)", len(m.inbox)))
		contentH := height - 1
		if contentH < 1 {
			contentH = 1
		}
		lines := m.buildInboxLines(width, true)
		content := strings.Join(lines, "\n")
		content = kit.ViewportSlice(content, contentH, m.inboxSel)
		return lipgloss.NewStyle().Width(width).Height(height).Render(title + "\n" + content)
	}

	title := styleSection.Render(fmt.Sprintf("Outbox (%d)", len(m.outbox)))
	contentH := height - 1
	if contentH < 1 {
		contentH = 1
	}
	lines := m.buildOutboxLines(width, true)
	content := strings.Join(lines, "\n")
	content = kit.ViewportSlice(content, contentH, m.outboxScrollOffset())
	return lipgloss.NewStyle().Width(width).Height(height).Render(title + "\n" + content)
}

func (m *Model) renderInboxPanel(width, height int) string {
	title := styleSection.Render(fmt.Sprintf("Inbox (%d)", len(m.inbox)))
	isFocused := m.focus == panelInbox

	contentH := height - 2 // border
	if contentH < 1 {
		contentH = 1
	}

	lines := m.buildInboxLines(width, isFocused)
	content := strings.Join(lines, "\n")
	// Manual viewport: show slice around selection (title line eats 1 from contentH)
	content = kit.ViewportSlice(content, contentH-1, m.inboxSel)

	borderColor := kit.BorderColorDefault
	if isFocused {
		borderColor = kit.BorderColorAccent
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width).
		Height(contentH).
		Render(title + "\n" + content)
}

func (m *Model) buildInboxLines(width int, isFocused bool) []string {
	var lines []string
	if len(m.inbox) == 0 {
		return []string{kit.StyleDim.Render("No pending inbox tasks.")}
	}

	idLen := 8
	if m.isNarrow() {
		idLen = 6
	}

	for i, t := range m.inbox {
		marker := "  "
		if isFocused && i == m.inboxSel {
			marker = styleAccent.Render("› ")
		}
		indicator := "  "
		if len(t.Children) > 0 {
			if t.Expanded {
				indicator = "▾ "
			} else {
				indicator = "▸ "
			}
		}
		line := marker + indicator + kit.StyleBold.Render(shortID(t.ID, idLen))
		if !m.isNarrow() {
			if t.Role != "" {
				line += " " + kit.StyleDim.Render("["+t.Role+"]")
			}
			if t.Status != "" && t.Status != "pending" {
				line += " " + kit.StyleDim.Render("["+t.Status+"]")
			}
			if len(t.Children) > 0 {
				line += " " + kit.StyleDim.Render(fmt.Sprintf("[batch:%d]", len(t.Children)))
			}
		}

		space := 25
		if m.isNarrow() {
			space = 15
		}
		goal := kit.Truncate(t.Goal, max(10, width-space))
		if goal != "" {
			line += " — " + goal
		}
		lines = append(lines, line)
		if t.Expanded {
			for _, child := range t.Children {
				childLine := "    " + styleAccent.Render("↳ ") + kit.StyleBold.Render(shortID(child.ID, idLen))
				if child.Role != "" {
					childLine += " " + kit.StyleDim.Render("["+child.Role+"]")
				}
				status := strings.TrimSpace(child.DisplayStatus)
				if status == "" {
					status = child.Status
				}
				if status != "" {
					childLine += " " + kit.StyleDim.Render("[") + renderStatus(status) + kit.StyleDim.Render("]")
				}
				if src := strings.TrimSpace(child.Source); src != "" && !m.isNarrow() {
					childLine += " " + kit.StyleDim.Render("["+src+"]")
				}
				if goal := kit.Truncate(child.Goal, max(10, width-space)); goal != "" {
					childLine += " — " + goal
				}
				lines = append(lines, childLine)
			}
		}
	}
	return lines
}

func (m *Model) renderOutboxPanel(width, height int) string {
	title := styleSection.Render(fmt.Sprintf("Outbox (%d)", len(m.outbox)))
	isFocused := m.focus == panelOutbox

	contentH := height - 2
	if contentH < 1 {
		contentH = 1
	}

	lines := m.buildOutboxLines(width, isFocused)
	content := strings.Join(lines, "\n")
	// Title line eats 1 from contentH
	content = kit.ViewportSlice(content, contentH-1, m.outboxScrollOffset())

	borderColor := kit.BorderColorDefault
	if isFocused {
		borderColor = kit.BorderColorAccent
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width).
		Height(contentH).
		Render(title + "\n" + content)
}

func (m *Model) buildOutboxLines(width int, isFocused bool) []string {
	var lines []string
	if len(m.outbox) == 0 {
		return []string{kit.StyleDim.Render("No outbox tasks yet.")}
	}

	idLen := 8
	if m.isNarrow() {
		idLen = 6
	}

	for i, r := range m.outbox {
		marker := "  "
		if isFocused && i == m.outboxSel {
			marker = styleAccent.Render("› ")
		}
		indicator := "  "
		if len(r.Children) > 0 {
			if r.Expanded {
				indicator = "▾ "
			} else {
				indicator = "▸ "
			}
		}

		space := 30
		if m.isNarrow() {
			space = 20
		}
		goal := kit.Truncate(r.Goal, max(10, width-space))
		statusStr := renderStatus(r.Status)

		meta := ""
		if !m.isNarrow() {
			metaParts := make([]string, 0, 2)
			if r.CostUSD > 0 {
				metaParts = append(metaParts, fmt.Sprintf("$%.4f", r.CostUSD))
			}
			if r.TotalTokens > 0 {
				metaParts = append(metaParts, fmt.Sprintf("%d tok", r.TotalTokens))
			}
			if len(metaParts) != 0 {
				meta = " " + kit.StyleDim.Render("("+strings.Join(metaParts, " · ")+")")
			}
		}

		header := marker + indicator + kit.StyleBold.Render(shortID(r.ID, idLen))
		if !m.isNarrow() && r.Role != "" {
			header += " " + kit.StyleDim.Render("["+r.Role+"]")
		}
		if !m.isNarrow() && len(r.Children) > 0 {
			header += " " + kit.StyleDim.Render(fmt.Sprintf("[batch:%d]", len(r.Children)))
		}
		header += " " + kit.StyleDim.Render("\""+goal+"\"") + " → " + statusStr + meta
		lines = append(lines, header)

		if r.Error != "" && (r.Status == "failed" || r.Status == "canceled") {
			lines = append(lines, "    └ "+kit.StyleErr.Render("error: "+kit.Truncate(r.Error, max(10, width-20))))
		}
		if !m.isNarrow() && r.TotalTokens > 0 {
			tokLine := fmt.Sprintf("    └ tokens: %d (%d in + %d out)", r.TotalTokens, r.InputTokens, r.OutputTokens)
			if r.CostUSD > 0 {
				tokLine += fmt.Sprintf(" · cost: $%.4f", r.CostUSD)
			}
			lines = append(lines, tokLine)
		}
		if r.Expanded {
			for _, child := range r.Children {
				status := strings.TrimSpace(child.DisplayStatus)
				if status == "" {
					status = child.Status
				}
				childLine := "    " + styleAccent.Render("↳ ") + kit.StyleBold.Render(shortID(child.ID, idLen))
				if child.Role != "" {
					childLine += " " + kit.StyleDim.Render("["+child.Role+"]")
				}
				if src := strings.TrimSpace(child.Source); src != "" && !m.isNarrow() {
					childLine += " " + kit.StyleDim.Render("["+src+"]")
				}
				childGoal := kit.Truncate(child.Goal, max(10, width-space))
				childLine += " " + kit.StyleDim.Render("\""+childGoal+"\"") + " → " + renderStatus(status)
				lines = append(lines, childLine)
			}
		}
	}
	return lines
}

func (m *Model) renderDetailPanel(width, height int) string {
	task := m.selectedTask()
	if task == nil {
		return ""
	}

	contentH := height - 2
	if contentH < 1 {
		contentH = 1
	}
	contentW := max(10, width-4)

	title := styleSection.Render("Task Detail")
	lines := []string{
		kit.StyleStatusKey.Render("ID:       ") + kit.StyleStatusValue.Render(task.ID),
		kit.StyleStatusKey.Render("Run:      ") + kit.StyleStatusValue.Render(kit.Fallback(task.RunID, "-")),
		kit.StyleStatusKey.Render("Role:     ") + kit.StyleStatusValue.Render(kit.Fallback(task.Role, "-")),
		kit.StyleStatusKey.Render("Status:   ") + renderStatus(task.Status),
	}

	if task.Goal != "" {
		lines = append(lines, kit.StyleStatusKey.Render("Goal:     ")+wrapText(task.Goal, contentW-10))
	}
	if task.Summary != "" {
		lines = append(lines, kit.StyleStatusKey.Render("Summary:  ")+renderDetailMarkdown(task.Summary, contentW-10))
	}
	if task.Error != "" {
		lines = append(lines, kit.StyleStatusKey.Render("Error:    ")+kit.StyleErr.Render(wrapText(task.Error, contentW-10)))
	}
	if task.TotalTokens > 0 {
		lines = append(lines, kit.StyleStatusKey.Render("Tokens:   ")+
			fmt.Sprintf("%d (%d in + %d out)", task.TotalTokens, task.InputTokens, task.OutputTokens))
	}
	if len(task.Children) > 0 {
		lines = append(lines, kit.StyleStatusKey.Render("Children: ")+fmt.Sprintf("%d callbacks", len(task.Children)))
	}
	if task.CostUSD > 0 {
		lines = append(lines, kit.StyleStatusKey.Render("Cost:     ")+fmt.Sprintf("$%.4f", task.CostUSD))
	}
	if !task.CreatedAt.IsZero() {
		lines = append(lines, kit.StyleStatusKey.Render("Created:  ")+task.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !task.CompletedAt.IsZero() {
		lines = append(lines, kit.StyleStatusKey.Render("Finished: ")+task.CompletedAt.Format("2006-01-02 15:04:05"))
	}
	if task.Artifacts > 0 {
		lines = append(lines, kit.StyleStatusKey.Render("Artifacts:")+fmt.Sprintf(" %d", task.Artifacts))
	}

	content := strings.Join(lines, "\n")
	// Title line eats 1 from contentH
	content = kit.ViewportSlice(content, contentH-1, 0)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(kit.BorderColorAccent).
		Width(width).
		Height(contentH).
		Render(title + "\n" + content)
}

// --- helpers ---

func renderStatus(status string) string {
	switch status {
	case "succeeded", "batched":
		return kit.StyleOK.Render(status)
	case "failed", "canceled":
		return kit.StyleErr.Render(status)
	case "active":
		return kit.StylePending.Render(status)
	default:
		return status
	}
}

func shortID(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
}

func wrapText(s string, width int) string {
	if width <= 0 {
		width = 40
	}
	return wordwrap.String(strings.TrimSpace(s), width)
}

func renderDetailMarkdown(md string, width int) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	rendered := strings.TrimRight(detailRenderer.RenderMarkdown(md, width), "\n")
	if strings.TrimSpace(rendered) == "" {
		return wrapText(md, width)
	}
	return rendered
}

// ---------------------------------------------------------------------------
// Project overview rendering
// ---------------------------------------------------------------------------

func (m *Model) renderProjectView() string {
	header := m.renderProjectHeader()
	footer := m.renderProjectFooter()
	summary := ""
	reserved := 2 // header + footer
	if !m.isShort() {
		summary = m.renderProjectSummaryBar()
		reserved++
	}

	bodyHeight := m.height - reserved
	if bodyHeight < 1 {
		out := header + "\n" + footer
		return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
	}

	body := m.renderTeamTable(m.width, bodyHeight)
	body = lipgloss.NewStyle().MaxHeight(bodyHeight).Render(body)

	out := header + "\n" + body + "\n" + footer
	if summary != "" {
		out = header + "\n" + summary + "\n" + body + "\n" + footer
	}
	return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
}

func (m *Model) renderProjectHeader() string {
	status := kit.StyleOK.Render("● connected")
	if !m.connected {
		status = kit.StyleErr.Render("● disconnected")
	}

	pid := kit.Fallback(kit.TruncateRight(m.projectID, 20), "-")
	teamCount := fmt.Sprintf("%d teams", len(m.teams))

	line := styleHeader.Render("agen8 mail") +
		kit.StyleDim.Render("  ·  ") + styleAccent.Render(pid) +
		kit.StyleDim.Render("  ·  ") + kit.StyleStatusValue.Render(teamCount) +
		kit.StyleDim.Render("  ·  ") + status

	if m.lastErr != "" {
		line += kit.StyleDim.Render("  ·  ") + kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 40))
	}
	if m.notice != "" {
		line += kit.StyleDim.Render("  ·  ") + kit.StylePending.Render(kit.Truncate(m.notice, 28))
	}

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(line)
}

func (m *Model) renderProjectSummaryBar() string {
	var pending, active, done, runningAgents int
	for _, t := range m.teams {
		pending += t.Pending
		active += t.Active
		done += t.Done
		runningAgents += t.RunningAgents
	}

	pendingLabel := kit.StylePending.Render(fmt.Sprintf("⏳ %d pending", pending))
	activeLabel := kit.StyleOK.Render(fmt.Sprintf("● %d active", active))
	doneLabel := kit.StyleDim.Render(fmt.Sprintf("✓ %d done", done))
	runningLabel := kit.StyleOK.Render(fmt.Sprintf("agents:%d", runningAgents))

	var line string
	if m.isCompact() {
		line = pendingLabel +
			kit.StyleDim.Render("  ") + activeLabel +
			kit.StyleDim.Render("  ") + doneLabel
	} else {
		line = pendingLabel +
			kit.StyleDim.Render("  ") + activeLabel +
			kit.StyleDim.Render("  ") + doneLabel +
			kit.StyleDim.Render("  ·  ") + runningLabel
	}

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(line)
}

func (m *Model) renderProjectFooter() string {
	var hints string
	if m.isNarrow() {
		hints = kit.StyleDim.Render("j/k") + " " +
			kit.StyleDim.Render("↵") + " " +
			kit.StyleDim.Render("r") + " " +
			kit.StyleDim.Render("q")
	} else {
		hints = kit.StyleDim.Render("j/k") + " scroll  " +
			kit.StyleDim.Render("enter") + " open team  " +
			kit.StyleDim.Render("g/G") + " first/last  " +
			kit.StyleDim.Render("r") + " refresh  " +
			kit.StyleDim.Render("q") + " quit"
	}
	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(hints)
}

func (m *Model) renderTeamTable(width, height int) string {
	header := m.renderTeamTableHeader(width)
	if len(m.teams) == 0 {
		empty := kit.StyleDim.Render("No teams found for this project.")
		body := lipgloss.NewStyle().Width(width).Height(max(1, height-1)).Padding(0, 1).Render(empty)
		return header + "\n" + body
	}

	rows := m.buildTeamRows(width)
	visibleRows := max(1, height-1)
	start := m.teamSel - visibleRows/2
	if start < 0 {
		start = 0
	}
	maxStart := max(0, len(rows)-visibleRows)
	if start > maxStart {
		start = maxStart
	}

	content := kit.ViewportSlice(strings.Join(rows, "\n"), visibleRows, start)
	body := lipgloss.NewStyle().
		Width(width).Height(visibleRows).Padding(0, 1).
		Render(content)

	return header + "\n" + body
}

func (m *Model) renderTeamTableHeader(width int) string {
	const markerW = 2
	inner := max(12, width-2-markerW)

	if m.isNarrow() {
		line := strings.Repeat(" ", markerW) +
			padRight("TEAM", max(6, inner-16)) + " " +
			padRight("STATUS", 14)
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
	}
	if m.isCompact() {
		statusW := 10
		profileW := 14
		tasksW := 12
		teamW := max(8, inner-(statusW+profileW+tasksW+3))
		line := strings.Repeat(" ", markerW) +
			padRight("TEAM", teamW) + " " +
			padRight("STATUS", statusW) + " " +
			padRight("PROFILE", profileW) + " " +
			padRight("TASKS", tasksW)
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
	}

	// Full width columns (no cost — mail focuses on task flow).
	teamW := 14
	statusW := 10
	profileW := 14
	coordW := 12
	agentsW := 8
	pendW := 5
	actW := 5
	doneW := 5
	ageW := max(6, inner-(teamW+statusW+profileW+coordW+agentsW+pendW+actW+doneW+8))

	line := strings.Repeat(" ", markerW) +
		padRight("TEAM", teamW) + " " +
		padRight("STATUS", statusW) + " " +
		padRight("PROFILE", profileW) + " " +
		padRight("COORD", coordW) + " " +
		padRight("AGENTS", agentsW) + " " +
		padRight("PEND", pendW) + " " +
		padRight("ACT", actW) + " " +
		padRight("DONE", doneW) + " " +
		padRight("AGE", ageW)
	return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
}

func (m *Model) buildTeamRows(width int) []string {
	rows := make([]string, 0, len(m.teams))
	const markerW = 2
	inner := max(12, width-2-markerW)

	for i, row := range m.teams {
		isSel := i == m.teamSel
		marker := "  "
		if isSel {
			marker = styleAccent.Render("› ")
		}

		teamLabel := teamShortLabel(row)

		if m.isNarrow() {
			teamW := max(6, inner-16)
			line := marker +
				kit.StyleStatusValue.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
				renderTeamStatusCell(row, 14, m.spinFrame)
			rows = append(rows, line)
			continue
		}

		if m.isCompact() {
			statusW := 10
			profileW := 14
			tasksW := 12
			teamW := max(8, inner-(statusW+profileW+tasksW+3))
			tasksSummary := fmt.Sprintf("%d/%d/%d", row.Pending, row.Active, row.Done)
			line := marker +
				kit.StyleStatusValue.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
				renderTeamStatusCell(row, statusW, m.spinFrame) + " " +
				kit.StyleDim.Render(padRight(kit.TruncateRight(kit.Fallback(row.ProfileID, "-"), profileW), profileW)) + " " +
				kit.StyleStatusValue.Render(padRight(tasksSummary, tasksW))
			rows = append(rows, line)
			continue
		}

		// Full width (no cost — mail focuses on task flow).
		teamW := 14
		statusW := 10
		profileW := 14
		coordW := 12
		agentsW := 8
		pendW := 5
		actW := 5
		doneW := 5
		ageW := max(6, inner-(teamW+statusW+profileW+coordW+agentsW+pendW+actW+doneW+8))

		agentStr := fmt.Sprintf("%d/%d", row.RunningAgents, row.TotalAgents)
		coordLabel := kit.Fallback(kit.TruncateRight(row.CoordinatorRole, coordW), "-")

		line := marker +
			kit.StyleStatusValue.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
			renderTeamStatusCell(row, statusW, m.spinFrame) + " " +
			kit.StyleDim.Render(padRight(kit.TruncateRight(kit.Fallback(row.ProfileID, "-"), profileW), profileW)) + " " +
			kit.StyleStatusValue.Render(padRight(coordLabel, coordW)) + " " +
			kit.StyleOK.Render(padRight(agentStr, agentsW)) + " " +
			kit.StylePending.Render(padRight(fmt.Sprintf("%d", row.Pending), pendW)) + " " +
			kit.StyleOK.Render(padRight(fmt.Sprintf("%d", row.Active), actW)) + " " +
			kit.StyleDim.Render(padRight(fmt.Sprintf("%d", row.Done), doneW)) + " " +
			kit.StyleDim.Render(padRight(relativeAge(row.UpdatedAt), ageW))
		rows = append(rows, line)
	}
	return rows
}

// teamShortLabel returns a compact display name for a team.
func teamShortLabel(row teamRow) string {
	id := strings.TrimSpace(row.TeamID)
	profile := strings.TrimSpace(row.ProfileID)

	hash := strings.TrimPrefix(id, "team-")
	if len(hash) > 8 {
		hash = hash[:8]
	}
	if hash == "" {
		hash = kit.Fallback(id, "-")
	}

	if profile != "" {
		return profile + "·" + hash[:min(4, len(hash))]
	}
	return hash
}

func renderTeamStatusCell(row teamRow, width, spinFrame int) string {
	if row.HasBlockedTasks {
		return kit.StyleErr.Render(padRight("blocked", width))
	}
	hasActiveTasks := row.Pending > 0 || row.Active > 0
	if row.RunningAgents > 0 {
		if hasActiveTasks && row.CoordinatorStatus != "" && isRunningStatus(row.CoordinatorStatus) {
			return kit.StyleOK.Render(padRight(kit.SpinnerFrames[spinFrame%len(kit.SpinnerFrames)]+" working", width))
		}
		if hasActiveTasks {
			return kit.StyleOK.Render(padRight("active", width))
		}
		return kit.StylePending.Render(padRight("idle", width))
	}
	s := strings.ToLower(strings.TrimSpace(row.Status))
	switch s {
	case "active":
		return kit.StylePending.Render(padRight("idle", width))
	case "registered":
		return kit.StyleDim.Render(padRight("registered", width))
	default:
		return kit.StyleDim.Render(padRight(kit.Fallback(s, "—"), width))
	}
}

// ---------------------------------------------------------------------------
// Shared helpers (replicated from dashboardtui — unexported there)
// ---------------------------------------------------------------------------

func padRight(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := runewidth.StringWidth(s)
	if w >= width {
		return runewidth.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", width-w)
}

func relativeAge(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "—"
	}
	ts, ok := parseTime(raw)
	if !ok {
		return kit.TruncateRight(raw, 8)
	}
	d := time.Since(ts)
	if d < 0 {
		d = 0
	}
	if d < 2*time.Second {
		return "just now"
	}
	if d < time.Minute {
		secs := int(d.Seconds() + 0.5)
		if secs < 2 {
			return "just now"
		}
		return fmt.Sprintf("%ds", secs)
	}
	if d >= 2*time.Minute {
		return fmt.Sprintf("%dm stale", int(d.Minutes()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func parseTime(raw string) (time.Time, bool) {
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
