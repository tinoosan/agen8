package coordinator

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

// ── Color palette ──────────────────────────────────────────────────────

var (
	colorOK      = lipgloss.Color("#98c379")
	colorErr     = lipgloss.Color("#e06c75")
	colorPending = lipgloss.Color("#e5c07b")
	colorAccent  = lipgloss.Color("#7aa2f7")

	styleOK      = lipgloss.NewStyle().Foreground(colorOK)
	styleErr     = lipgloss.NewStyle().Foreground(colorErr)
	stylePending = lipgloss.NewStyle().Foreground(colorPending)
	styleAccent  = lipgloss.NewStyle().Foreground(colorAccent)
	styleHeader  = lipgloss.NewStyle().Bold(true)
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const (
	compactWidth = 60
	smallHeight  = 14
)

func (m *Model) isNarrow() bool { return m.width < compactWidth }
func (m *Model) isShort() bool  { return m.height < smallHeight }

// ── View ───────────────────────────────────────────────────────────────

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	header := m.renderHeader()
	sep := kit.StyleDim.Render(strings.Repeat("─", m.width))
	footer := m.renderFooter()
	input := m.renderInputBar()

	// height: header(1) + sep(1) + feed(?) + input(2 with border) + footer(1) = 5 chrome lines
	bodyHeight := m.height - 5
	if bodyHeight < 1 {
		out := header + "\n" + input + "\n" + footer
		return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
	}
	feed := m.renderFeed(bodyHeight)

	out := header + "\n" + sep + "\n" + feed + "\n" + input + "\n" + footer
	return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
}

// ── Header ─────────────────────────────────────────────────────────────

func (m *Model) renderHeader() string {
	left := styleHeader.Render("agen8") + kit.StyleDim.Render(" coordinator")

	// Build right-side tags
	var tags []string

	sid := strings.TrimSpace(m.sessionID)
	if len(sid) > 12 {
		sid = sid[:12]
	}
	if sid != "" {
		tags = append(tags, kit.RenderTag(kit.TagOptions{
			Key:   "session",
			Value: sid,
		}))
	}

	// Connection pill
	if m.connected {
		tags = append(tags, styleOK.Render("● connected"))
	} else {
		tags = append(tags, styleErr.Render("● disconnected"))
	}

	// Mode tag
	mode := fallback(m.sessionMode, "standalone")
	tags = append(tags, kit.RenderTag(kit.TagOptions{
		Key:   "mode",
		Value: mode,
	}))

	// Role tag (only in wide terminals)
	if !m.isNarrow() && strings.TrimSpace(m.coordinatorRole) != "" {
		tags = append(tags, kit.RenderTag(kit.TagOptions{
			Key:   "role",
			Value: m.coordinatorRole,
		}))
	}

	if m.lastErr != "" {
		tags = append(tags, styleErr.Render("err: "+truncate(m.lastErr, 32)))
	}

	right := strings.Join(tags, kit.StyleDim.Render(" · "))

	// Layout: left  ···padding···  right
	leftW := runewidth.StringWidth(stripANSI(left))
	rightW := runewidth.StringWidth(stripANSI(right))
	avail := m.width - 2 // padding
	gap := avail - leftW - rightW
	if gap < 1 {
		gap = 1
	}

	line := left + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).Render(line)
}

// ── Turn grouping ──────────────────────────────────────────────────────

func (m *Model) buildTurns() []conversationTurn {
	if len(m.feed) == 0 {
		return nil
	}
	entries := make([]feedEntry, len(m.feed))
	copy(entries, m.feed)
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].timestamp.Before(entries[j].timestamp)
	})

	var turns []conversationTurn
	var curAgent *conversationTurn

	flushAgent := func() {
		if curAgent != nil {
			turns = append(turns, *curAgent)
			curAgent = nil
		}
	}

	for _, e := range entries {
		switch e.kind {
		case feedUser:
			flushAgent()
			turns = append(turns, conversationTurn{
				kind:      turnUser,
				role:      "You",
				timestamp: e.timestamp,
				text:      e.text,
			})
		case feedSystem:
			flushAgent()
			turns = append(turns, conversationTurn{
				kind:      turnSystem,
				role:      "system",
				timestamp: e.timestamp,
				text:      e.text,
			})
		case feedAgent:
			role := fallback(e.role, "agent")
			if curAgent != nil && curAgent.role != role {
				flushAgent()
			}
			if curAgent == nil {
				curAgent = &conversationTurn{
					kind:      turnAgent,
					role:      role,
					timestamp: e.timestamp,
				}
			}
			if e.timestamp.After(curAgent.timestamp) {
				curAgent.timestamp = e.timestamp
			}
			curAgent.entries = append(curAgent.entries, e)
		}
	}
	flushAgent()
	return turns
}

// ── Feed ───────────────────────────────────────────────────────────────

func (m *Model) renderFeed(height int) string {
	lines := m.feedLines(m.width)
	if len(lines) == 0 {
		empty := kit.StyleDim.Render("  Waiting for activity...")
		return lipgloss.NewStyle().Width(m.width).Height(height).Render(empty)
	}

	total := len(lines)
	start := m.feedScroll
	if m.liveFollow {
		start = maxInt(0, total-height)
	}

	// Compute scroll percent
	maxScroll := maxInt(1, total-height)
	if total <= height {
		m.scrollPercent = 100.0
	} else {
		m.scrollPercent = float64(start) / float64(maxScroll) * 100.0
		if m.scrollPercent > 100 {
			m.scrollPercent = 100
		}
	}

	content := viewportSlice(strings.Join(lines, "\n"), height, start)
	return lipgloss.NewStyle().Width(m.width).Height(height).Render(content)
}

func (m *Model) feedLines(width int) []string {
	turns := m.buildTurns()
	if len(turns) == 0 {
		return nil
	}

	inner := maxInt(12, width-2)
	dimRule := kit.StyleDim.Render(strings.Repeat("─", inner))
	lines := make([]string, 0, len(turns)*4)

	for i, t := range turns {
		switch t.kind {
		case turnUser:
			lines = append(lines, m.renderUserBlock(t, inner)...)
		case turnAgent:
			lines = append(lines, m.renderAgentBlock(t, inner)...)
		case turnSystem:
			lines = append(lines, m.renderSystemBlock(t, inner)...)
		}
		// Separator between blocks
		if i < len(turns)-1 {
			lines = append(lines, "  "+dimRule)
		}
	}
	return lines
}

// ── User block ─────────────────────────────────────────────────────────
//
//   ❯ You                                              2m ago
//   Please implement the authentication module
//   ✓ queued

func (m *Model) renderUserBlock(t conversationTurn, inner int) []string {
	age := relativeAge(t.timestamp.Format(time.RFC3339))
	label := styleAccent.Render("❯") + " " + styleAccent.Bold(true).Render("You")
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := maxInt(1, inner-labelW-ageW)
	headerLine := "  " + label + strings.Repeat(" ", gap) + ageStr

	msgLine := "  " + truncate(t.text, maxInt(8, inner-2))
	statusLine := "  " + styleOK.Render("✓ queued")

	return []string{headerLine, msgLine, statusLine, ""}
}

// ── Agent block ────────────────────────────────────────────────────────
//
//   ■ architect                                        30s ago
//   📄 ✓ fs_read  src/auth/handler.go
//   ⚡ ⠹ shell_exec  go test ./...

func (m *Model) renderAgentBlock(t conversationTurn, inner int) []string {
	age := relativeAge(t.timestamp.Format(time.RFC3339))
	role := truncate(t.role, maxInt(4, 14))
	if m.isNarrow() {
		role = truncate(role, 6)
	}

	label := styleAccent.Render("■") + " " + styleAccent.Bold(true).Render(role)
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := maxInt(1, inner-labelW-ageW)
	headerLine := "  " + label + strings.Repeat(" ", gap) + ageStr

	lines := []string{headerLine}
	for _, e := range t.entries {
		icon := kit.KindIcon(e.opKind)
		status := m.statusIcon(e.status)
		kind := truncate(fallback(e.opKind, "op"), 20)
		text := truncate(e.text, maxInt(8, inner-len(kind)-8))

		line := "  " + icon + " " + status + " " + kit.StyleDim.Render(kind)
		if text != "" && text != kind {
			line += "  " + text
		}
		lines = append(lines, line)
	}
	lines = append(lines, "")
	return lines
}

// ── System block ───────────────────────────────────────────────────────
//
//   ◆ system                                           1m ago
//   Session paused

func (m *Model) renderSystemBlock(t conversationTurn, inner int) []string {
	age := relativeAge(t.timestamp.Format(time.RFC3339))
	label := stylePending.Render("◆") + " " + stylePending.Bold(true).Render("system")
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := maxInt(1, inner-labelW-ageW)
	headerLine := "  " + label + strings.Repeat(" ", gap) + ageStr

	msgLine := "  " + truncate(t.text, maxInt(8, inner-2))
	return []string{headerLine, msgLine, ""}
}

// ── Input bar ──────────────────────────────────────────────────────────

func (m *Model) renderInputBar() string {
	feedback := strings.TrimSpace(m.feedback)
	feedbackStyled := ""
	if feedback != "" {
		switch m.feedbackKind {
		case feedbackOK:
			feedbackStyled = styleOK.Render(feedback)
		case feedbackErr:
			feedbackStyled = styleErr.Render(feedback)
		default:
			feedbackStyled = stylePending.Render(feedback)
		}
	}

	avail := maxInt(12, m.width-2-runewidth.StringWidth(stripANSI(feedback))-2)
	m.input.Width = avail
	left := m.input.View()
	line := left
	if feedbackStyled != "" {
		gap := m.width - 4 - runewidth.StringWidth(stripANSI(left)) - runewidth.StringWidth(stripANSI(feedback))
		if gap < 1 {
			gap = 1
		}
		line = left + strings.Repeat(" ", gap) + feedbackStyled
	}

	// Rounded top border to separate from feed
	border := lipgloss.RoundedBorder()
	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).
		BorderTop(true).
		BorderStyle(border).
		BorderForeground(kit.BorderColorDefault).
		MaxHeight(2).
		Padding(0, 1).
		Render(line)
}

// ── Footer ─────────────────────────────────────────────────────────────

func (m *Model) renderFooter() string {
	// Left: scroll position
	var scrollLabel string
	if m.scrollPercent >= 99.9 || m.liveFollow {
		scrollLabel = kit.StyleDim.Render("end")
	} else {
		scrollLabel = kit.StyleDim.Render(fmt.Sprintf("%d%%", int(m.scrollPercent)))
	}

	// Right: key hints
	var hints string
	if m.isNarrow() {
		hints = kit.StyleDim.Render("↵ /p /r /s pgup/dn g/G ^c")
	} else {
		hints = kit.StyleDim.Render("enter") + " send  " +
			kit.StyleDim.Render("/pause /resume /stop /help") + "  " +
			kit.StyleDim.Render("pgup/pgdn") + " scroll  " +
			kit.StyleDim.Render("home/end") + " jump  " +
			kit.StyleDim.Render("ctrl+c") + " quit"
	}

	leftW := runewidth.StringWidth(stripANSI(scrollLabel))
	rightW := runewidth.StringWidth(stripANSI(hints))
	gap := maxInt(1, m.width-2-leftW-rightW)

	line := scrollLabel + strings.Repeat(" ", gap) + hints
	return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).Render(line)
}

// ── Status icon ────────────────────────────────────────────────────────

func (m *Model) statusIcon(status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "ok", "succeeded", "done", "completed":
		return styleOK.Render("✓")
	case "error", "failed", "canceled", "cancelled":
		return styleErr.Render("✗")
	case "pending":
		return stylePending.Render("…")
	default:
		return stylePending.Render(spinnerFrames[m.spinFrame%len(spinnerFrames)])
	}
}

func (m *Model) totalFeedLines() int {
	return len(m.feedLines(m.width))
}

// ── Shared helpers ─────────────────────────────────────────────────────

func stripANSI(s string) string {
	b := make([]rune, 0, len(s))
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b = append(b, r)
	}
	return string(b)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" || runewidth.StringWidth(s) <= max {
		return s
	}
	if max <= 1 {
		return runewidth.Truncate(s, max, "")
	}
	return runewidth.Truncate(s, max-1, "") + "…"
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

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

func viewportSlice(content string, visibleLines, targetIdx int) string {
	lines := strings.Split(content, "\n")
	if visibleLines <= 0 {
		visibleLines = 1
	}
	if len(lines) <= visibleLines {
		return content
	}
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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

func relativeAge(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "—"
	}
	ts, ok := parseTime(raw)
	if !ok {
		return truncate(raw, 8)
	}
	d := time.Since(ts)
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}
