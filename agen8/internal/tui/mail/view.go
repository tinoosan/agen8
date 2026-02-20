package mail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

var (
	colorGreen = lipgloss.Color("#98c379")
	colorRed   = lipgloss.Color("#e06c75")
	colorAmber = lipgloss.Color("#e5c07b")

	styleGreen = lipgloss.NewStyle().Foreground(colorGreen)
	styleRed   = lipgloss.NewStyle().Foreground(colorRed)
	styleAmber = lipgloss.NewStyle().Foreground(colorAmber)

	styleAccent  = lipgloss.NewStyle().Foreground(kit.BorderColorAccent)
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#eaeaea"))
	styleSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9ad0ff"))
)

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	bodyHeight := m.height - 2 // header + footer
	if bodyHeight < 1 {
		return header + "\n" + footer
	}

	body := m.renderBody(bodyHeight)

	return header + "\n" + body + "\n" + footer
}

func (m *Model) renderHeader() string {
	sid := m.sessionID
	if len(sid) > 12 {
		sid = sid[:12]
	}

	status := styleGreen.Render("● connected")
	if !m.connected {
		status = styleRed.Render("● disconnected")
	}

	counts := fmt.Sprintf("inbox:%d  outbox:%d", len(m.inbox), len(m.outbox))

	left := styleHeader.Render("agen8 mail") +
		kit.StyleDim.Render("  ·  session: ") + styleAccent.Render(sid) +
		kit.StyleDim.Render("  ·  ") + status +
		kit.StyleDim.Render("  ·  ") + kit.StyleDim.Render(counts)

	if m.lastErr != "" {
		left += kit.StyleDim.Render("  ·  ") + styleRed.Render("err: "+truncate(m.lastErr, 40))
	}

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		Background(lipgloss.Color("#1a1a2e")).
		Foreground(lipgloss.Color("#eaeaea")).
		Padding(0, 1).
		Render(left)
}

func (m *Model) renderFooter() string {
	hints := kit.StyleDim.Render("tab") + " focus  " +
		kit.StyleDim.Render("j/k") + " scroll  " +
		kit.StyleDim.Render("enter") + " detail  " +
		kit.StyleDim.Render("esc") + " close  " +
		kit.StyleDim.Render("r") + " refresh  " +
		kit.StyleDim.Render("q") + " quit"

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		Background(lipgloss.Color("#1a1a2e")).
		Padding(0, 1).
		Render(hints)
}

func (m *Model) renderBody(height int) string {
	innerW := m.width - 2 // account for panel borders

	// Current task panel: 6 lines (1 title + 3 content + 2 border)
	currentH := 6
	if currentH > height-2 {
		currentH = height - 2
	}
	if currentH < 3 {
		currentH = 3 // border + 1 line minimum
	}
	remaining := height - currentH
	if remaining < 2 {
		remaining = 2
	}

	currentPanel := m.renderCurrentTask(innerW, currentH)

	if m.detailOpen {
		if m.selectedTask() != nil {
			detailPanel := m.renderDetailPanel(innerW, remaining)
			return lipgloss.JoinVertical(lipgloss.Left, currentPanel, detailPanel)
		}
		// No task selected — fall through to normal inbox+outbox layout
		m.detailOpen = false
	}

	// Split remaining 50/50 between inbox and outbox
	inboxH := remaining / 2
	outboxH := remaining - inboxH

	inboxPanel := m.renderInboxPanel(innerW, inboxH)
	outboxPanel := m.renderOutboxPanel(innerW, outboxH)

	return lipgloss.JoinVertical(lipgloss.Left, currentPanel, inboxPanel, outboxPanel)
}

func (m *Model) renderCurrentTask(width, height int) string {
	title := styleSection.Render("Current Task")
	body := kit.StyleDim.Render("No active task")

	if m.currentTask != nil {
		t := m.currentTask
		elapsed := time.Since(t.CreatedAt).Round(time.Second)
		contentW := maxInt(10, width-14)
		body = strings.Join([]string{
			kit.StyleStatusKey.Render("Goal:    ") + kit.StyleStatusValue.Render(truncate(t.Goal, contentW)),
			kit.StyleStatusKey.Render("Status:  ") + renderStatus(t.Status) + kit.StyleDim.Render("   Role: ") + kit.StyleStatusValue.Render(fallback(t.Role, "-")),
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

func (m *Model) renderInboxPanel(width, height int) string {
	title := styleSection.Render(fmt.Sprintf("Inbox (%d)", len(m.inbox)))
	isFocused := m.focus == panelInbox

	contentH := height - 2 // border
	if contentH < 1 {
		contentH = 1
	}

	var lines []string
	if len(m.inbox) == 0 {
		lines = []string{kit.StyleDim.Render("No pending inbox tasks.")}
	} else {
		for i, t := range m.inbox {
			marker := "  "
			if isFocused && i == m.inboxSel {
				marker = styleAccent.Render("› ")
			}
			line := marker + kit.StyleBold.Render(shortID(t.ID))
			if t.Role != "" {
				line += " " + kit.StyleDim.Render("["+t.Role+"]")
			}
			if t.Status != "" && t.Status != "pending" {
				line += " " + kit.StyleDim.Render("["+t.Status+"]")
			}
			goal := truncate(t.Goal, maxInt(10, width-25))
			if goal != "" {
				line += " — " + goal
			}
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	// Manual viewport: show slice around selection (title line eats 1 from contentH)
	content = viewportSlice(content, contentH-1, m.inboxSel)

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

func (m *Model) renderOutboxPanel(width, height int) string {
	title := styleSection.Render(fmt.Sprintf("Outbox (%d)", len(m.outbox)))
	isFocused := m.focus == panelOutbox

	contentH := height - 2
	if contentH < 1 {
		contentH = 1
	}

	var lines []string
	if len(m.outbox) == 0 {
		lines = []string{kit.StyleDim.Render("No completed tasks yet.")}
	} else {
		for i, r := range m.outbox {
			marker := "  "
			if isFocused && i == m.outboxSel {
				marker = styleAccent.Render("› ")
			}

			goal := truncate(r.Goal, maxInt(10, width-30))
			statusStr := renderStatus(r.Status)

			metaParts := make([]string, 0, 2)
			if r.CostUSD > 0 {
				metaParts = append(metaParts, fmt.Sprintf("$%.4f", r.CostUSD))
			}
			if r.TotalTokens > 0 {
				metaParts = append(metaParts, fmt.Sprintf("%d tok", r.TotalTokens))
			}
			meta := ""
			if len(metaParts) != 0 {
				meta = " " + kit.StyleDim.Render("("+strings.Join(metaParts, " · ")+")")
			}

			header := marker + kit.StyleBold.Render(shortID(r.ID))
			if r.Role != "" {
				header += " " + kit.StyleDim.Render("["+r.Role+"]")
			}
			header += " " + kit.StyleDim.Render("\""+goal+"\"") + " → " + statusStr + meta
			lines = append(lines, header)

			if r.Error != "" && (r.Status == "failed" || r.Status == "canceled") {
				lines = append(lines, "    └ "+styleRed.Render("error: "+truncate(r.Error, maxInt(10, width-20))))
			}
			if r.TotalTokens > 0 {
				tokLine := fmt.Sprintf("    └ tokens: %d (%d in + %d out)", r.TotalTokens, r.InputTokens, r.OutputTokens)
				if r.CostUSD > 0 {
					tokLine += fmt.Sprintf(" · cost: $%.4f", r.CostUSD)
				}
				lines = append(lines, tokLine)
			}
		}
	}

	content := strings.Join(lines, "\n")
	// Title line eats 1 from contentH
	content = viewportSlice(content, contentH-1, m.outboxScrollOffset())

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

func (m *Model) renderDetailPanel(width, height int) string {
	task := m.selectedTask()
	if task == nil {
		return ""
	}

	contentH := height - 2
	if contentH < 1 {
		contentH = 1
	}
	contentW := maxInt(10, width-4)

	title := styleSection.Render("Task Detail")
	lines := []string{
		kit.StyleStatusKey.Render("ID:       ") + kit.StyleStatusValue.Render(task.ID),
		kit.StyleStatusKey.Render("Run:      ") + kit.StyleStatusValue.Render(fallback(task.RunID, "-")),
		kit.StyleStatusKey.Render("Role:     ") + kit.StyleStatusValue.Render(fallback(task.Role, "-")),
		kit.StyleStatusKey.Render("Status:   ") + renderStatus(task.Status),
	}

	if task.Goal != "" {
		lines = append(lines, kit.StyleStatusKey.Render("Goal:     ")+wrapText(task.Goal, contentW-10))
	}
	if task.Summary != "" {
		lines = append(lines, kit.StyleStatusKey.Render("Summary:  ")+wrapText(task.Summary, contentW-10))
	}
	if task.Error != "" {
		lines = append(lines, kit.StyleStatusKey.Render("Error:    ")+styleRed.Render(wrapText(task.Error, contentW-10)))
	}
	if task.TotalTokens > 0 {
		lines = append(lines, kit.StyleStatusKey.Render("Tokens:   ")+
			fmt.Sprintf("%d (%d in + %d out)", task.TotalTokens, task.InputTokens, task.OutputTokens))
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
	content = viewportSlice(content, contentH-1, 0)

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
	case "succeeded":
		return styleGreen.Render(status)
	case "failed", "canceled":
		return styleRed.Render(status)
	case "active":
		return styleAmber.Render(status)
	default:
		return status
	}
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" || runewidth.StringWidth(s) <= max {
		return s
	}
	if max <= 3 {
		return runewidth.Truncate(s, max, "")
	}
	return runewidth.Truncate(s, max-3, "") + "..."
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func wrapText(s string, width int) string {
	if width <= 0 {
		width = 40
	}
	return wordwrap.String(strings.TrimSpace(s), width)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// viewportSlice returns a window of visibleLines starting from targetIdx (top-anchored).
func viewportSlice(content string, visibleLines, targetIdx int) string {
	lines := strings.Split(content, "\n")
	if visibleLines <= 0 {
		visibleLines = 1
	}
	if len(lines) <= visibleLines {
		return content
	}
	// Clamp targetIdx to valid range
	if targetIdx < 0 {
		targetIdx = 0
	}
	if targetIdx >= len(lines) {
		targetIdx = len(lines) - 1
	}
	start := targetIdx
	end := start + visibleLines
	if end > len(lines) {
		end = len(lines)
		start = maxInt(0, end-visibleLines)
	}
	return strings.Join(lines[start:end], "\n")
}
