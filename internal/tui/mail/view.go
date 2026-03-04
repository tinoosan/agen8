package mail

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
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

	detailMDMu       sync.Mutex
	detailMDByWidth  = map[int]*glamour.TermRenderer{}
	detailMDFallback = lipgloss.NewStyle()
)

const (
	compactWidth = 60
	smallHeight  = 14
	mediumHeight = 20
)

func (m *Model) isNarrow() bool { return m.width < compactWidth }
func (m *Model) isShort() bool  { return m.height < smallHeight }
func (m *Model) isMedium() bool { return m.height < mediumHeight }

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
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
		status := styleGreen.Render("● connected")
		if !m.connected {
			status = styleRed.Render("● disconnected")
		}

		left := styleHeader.Render("mail") + kit.StyleDim.Render(" · ") + status
		if m.lastErr != "" {
			left += kit.StyleDim.Render(" · ") + styleRed.Render("err: "+truncate(m.lastErr, 20))
		}
		if m.notice != "" {
			left += kit.StyleDim.Render(" · ") + styleAmber.Render(truncate(m.notice, 18))
		}

		return lipgloss.NewStyle().
			Width(m.width).
			MaxWidth(m.width).
			MaxHeight(1).
			Foreground(lipgloss.Color("#eaeaea")).
			Padding(0, 1).
			Render(left)
	}

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
	if m.notice != "" {
		left += kit.StyleDim.Render("  ·  ") + styleAmber.Render(truncate(m.notice, 28))
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
	if m.isNarrow() {
		hints := kit.StyleDim.Render("tab") + " " +
			kit.StyleDim.Render("j/k") + " " +
			kit.StyleDim.Render("↵") + " " +
			kit.StyleDim.Render("esc") + " " +
			kit.StyleDim.Render("r") + " " +
			kit.StyleDim.Render("q")

		return lipgloss.NewStyle().
			Width(m.width).
			MaxWidth(m.width).
			MaxHeight(1).
			Padding(0, 1).
			Render(hints)
	}

	hints := kit.StyleDim.Render("tab") + " focus  " +
		kit.StyleDim.Render("j/k") + " scroll  " +
		kit.StyleDim.Render("enter") + " detail  " +
		kit.StyleDim.Render("esc") + " close  " +
		kit.StyleDim.Render("r") + " refresh  " +
		kit.StyleDim.Render("q") + " quit"

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

func (m *Model) renderFocusedOnly(width, height int) string {
	if m.focus == panelInbox {
		title := styleSection.Render(fmt.Sprintf("Inbox (%d)", len(m.inbox)))
		contentH := height - 1
		if contentH < 1 {
			contentH = 1
		}
		lines := m.buildInboxLines(width, true)
		content := strings.Join(lines, "\n")
		content = viewportSlice(content, contentH, m.inboxSel)
		return lipgloss.NewStyle().Width(width).Height(height).Render(title + "\n" + content)
	}

	title := styleSection.Render(fmt.Sprintf("Outbox (%d)", len(m.outbox)))
	contentH := height - 1
	if contentH < 1 {
		contentH = 1
	}
	lines := m.buildOutboxLines(width, true)
	content := strings.Join(lines, "\n")
	content = viewportSlice(content, contentH, m.outboxScrollOffset())
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
		line := marker + kit.StyleBold.Render(shortID(t.ID, idLen))
		if !m.isNarrow() {
			if t.Role != "" {
				line += " " + kit.StyleDim.Render("["+t.Role+"]")
			}
			if t.Status != "" && t.Status != "pending" {
				line += " " + kit.StyleDim.Render("["+t.Status+"]")
			}
		}

		space := 25
		if m.isNarrow() {
			space = 15
		}
		goal := truncate(t.Goal, maxInt(10, width-space))
		if goal != "" {
			line += " — " + goal
		}
		lines = append(lines, line)
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

		space := 30
		if m.isNarrow() {
			space = 20
		}
		goal := truncate(r.Goal, maxInt(10, width-space))
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

		header := marker + kit.StyleBold.Render(shortID(r.ID, idLen))
		if !m.isNarrow() && r.Role != "" {
			header += " " + kit.StyleDim.Render("["+r.Role+"]")
		}
		header += " " + kit.StyleDim.Render("\""+goal+"\"") + " → " + statusStr + meta
		lines = append(lines, header)

		if r.Error != "" && (r.Status == "failed" || r.Status == "canceled") {
			lines = append(lines, "    └ "+styleRed.Render("error: "+truncate(r.Error, maxInt(10, width-20))))
		}
		if !m.isNarrow() && r.TotalTokens > 0 {
			tokLine := fmt.Sprintf("    └ tokens: %d (%d in + %d out)", r.TotalTokens, r.InputTokens, r.OutputTokens)
			if r.CostUSD > 0 {
				tokLine += fmt.Sprintf(" · cost: $%.4f", r.CostUSD)
			}
			lines = append(lines, tokLine)
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
		lines = append(lines, kit.StyleStatusKey.Render("Summary:  ")+renderDetailMarkdown(task.Summary, contentW-10))
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

func shortID(id string, n int) string {
	if len(id) <= n {
		return id
	}
	return id[:n]
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

func renderDetailMarkdown(md string, width int) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	if width <= 0 {
		width = 40
	}

	r, err := detailMarkdownRenderer(width)
	if err != nil {
		return detailMDFallback.Render(wrapText(md, width))
	}
	out, err := r.Render(md)
	if err != nil {
		return detailMDFallback.Render(wrapText(md, width))
	}
	return strings.TrimRight(out, "\n")
}

func detailMarkdownRenderer(width int) (*glamour.TermRenderer, error) {
	detailMDMu.Lock()
	defer detailMDMu.Unlock()

	if r, ok := detailMDByWidth[width]; ok {
		return r, nil
	}

	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return nil, err
	}
	detailMDByWidth[width] = r
	return r, nil
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
