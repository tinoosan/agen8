package activitytui

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/tinoosan/agen8/pkg/types"
)

// ── Color palette (Tokyo Night-ish) ────────────────────────────────────

var (
	colorOK      = lipgloss.Color("#98c379")
	colorErr     = lipgloss.Color("#e06c75")
	colorPending = lipgloss.Color("#e5c07b")
	colorAccent  = lipgloss.Color("#7aa2f7")

	styleOK      = lipgloss.NewStyle().Foreground(colorOK)
	styleErr     = lipgloss.NewStyle().Foreground(colorErr)
	stylePending = lipgloss.NewStyle().Foreground(colorPending)
	styleAccent  = lipgloss.NewStyle().Foreground(colorAccent)

	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c8d3f5"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#5a6375"))
	styleMeta   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9da3b4"))

	styleSelRow = lipgloss.NewStyle().
			Background(lipgloss.Color("#1e2a3a")).
			Foreground(lipgloss.Color("#c8d3f5"))
	styleUnselRow = lipgloss.NewStyle().Foreground(lipgloss.Color("#9da3b4"))

	headerBg = lipgloss.Color("#1a1a2e")

	mdMu       sync.Mutex
	mdByWidth  = map[int]*glamour.TermRenderer{}
	mdFallback = lipgloss.NewStyle()
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
	footer := m.renderFooter()

	bodyHeight := m.height - 2 // header + footer
	if bodyHeight < 1 {
		out := header + "\n" + footer
		return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
	}

	body := m.renderBody(bodyHeight)
	body = lipgloss.NewStyle().MaxHeight(bodyHeight).Render(body)

	out := header + "\n" + body + "\n" + footer
	return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
}

// ── Header ─────────────────────────────────────────────────────────────

func (m *Model) renderHeader() string {
	status := styleOK.Render("● connected")
	if !m.connected {
		status = styleErr.Render("● disconnected")
	}

	if m.isNarrow() {
		left := styleHeader.Render("activity") + styleDim.Render(" · ") + status
		if m.lastErr != "" {
			left += styleDim.Render(" · ") + styleErr.Render("err: "+truncate(m.lastErr, 20))
		}
		return lipgloss.NewStyle().
			Width(m.width).MaxWidth(m.width).MaxHeight(1).
			Background(headerBg).
			Foreground(lipgloss.Color("#eaeaea")).
			Padding(0, 1).
			Render(left)
	}

	count := fmt.Sprintf("%d ops", len(m.activities))
	if m.totalCount > len(m.activities) {
		count = fmt.Sprintf("%d/%d ops", len(m.activities), m.totalCount)
	}

	left := styleHeader.Render("agen8 activity") +
		styleDim.Render("  ·  ") + status +
		styleDim.Render("  ·  ") + styleMeta.Render(count)

	if m.lastErr != "" {
		left += styleDim.Render("  ·  ") + styleErr.Render("err: "+truncate(m.lastErr, 40))
	}

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).
		Background(headerBg).
		Foreground(lipgloss.Color("#eaeaea")).
		Padding(0, 1).
		Render(left)
}

// ── Footer ─────────────────────────────────────────────────────────────

func (m *Model) renderFooter() string {
	if m.isNarrow() {
		hints := styleDim.Render("j/k") + " " +
			styleDim.Render("↵") + " " +
			styleDim.Render("esc") + " " +
			styleDim.Render("g/G") + " " +
			styleDim.Render("r") + " " +
			styleDim.Render("q")
		return lipgloss.NewStyle().
			Width(m.width).MaxWidth(m.width).MaxHeight(1).
			Background(headerBg).
			Padding(0, 1).
			Render(hints)
	}

	hints := styleDim.Render("j/k") + " scroll  " +
		styleDim.Render("enter") + " detail  " +
		styleDim.Render("esc") + " close  " +
		styleDim.Render("g/G") + " top/bottom  " +
		styleDim.Render("pgup/pgdn") + " page  " +
		styleDim.Render("r") + " refresh  " +
		styleDim.Render("q") + " quit"

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).
		Background(headerBg).
		Padding(0, 1).
		Render(hints)
}

// ── Body ───────────────────────────────────────────────────────────────

func (m *Model) renderBody(height int) string {
	if len(m.activities) == 0 {
		empty := styleDim.Render("  No activities yet. Waiting for agent ops…")
		if m.lastErr != "" {
			empty = styleErr.Render("  " + m.lastErr)
		}
		return lipgloss.NewStyle().Width(m.width).Height(height).Render(empty)
	}

	if m.detailOpen && m.sel >= 0 && m.sel < len(m.activities) {
		if m.isShort() {
			// Tiny: only detail
			return m.renderDetailPanel(m.width, height)
		}
		// Split: list on top, detail on bottom
		listH := height / 3
		if listH < 4 {
			listH = 4
		}
		detailH := height - listH
		if detailH < 3 {
			detailH = 3
			listH = height - detailH
		}
		list := m.renderListPanel(m.width, listH)
		detail := m.renderDetailPanel(m.width, detailH)
		return lipgloss.JoinVertical(lipgloss.Left, list, detail)
	}

	return m.renderListPanel(m.width, height)
}

// ── List panel ─────────────────────────────────────────────────────────

func (m *Model) renderListPanel(width, height int) string {
	lines := m.buildListLines(width)
	content := strings.Join(lines, "\n")

	// Viewport: if live-following, anchor to bottom; else center on selection
	if m.liveFollow {
		totalLines := len(lines)
		anchor := totalLines - height
		if anchor < 0 {
			anchor = 0
		}
		content = viewportSlice(content, height, anchor)
	} else {
		// If not live-following, center on selection.
		// Each item takes 2 lines, so m.sel * 2 gives the start line of the selected item.
		// We want to scroll such that the selected item is visible, ideally centered.
		// A simple approach is to scroll to the start of the selected item.
		// If the selected item is near the end, ensure the viewport shows the end.
		startLine := m.sel * 2
		if startLine+height > len(lines) {
			startLine = maxInt(0, len(lines)-height)
		}
		content = viewportSlice(content, height, startLine)
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(content)
}

func (m *Model) buildListLines(width int) []string {
	if len(m.activities) == 0 {
		return []string{styleDim.Render("  No activities.")}
	}

	lines := make([]string, 0, len(m.activities)*2)
	for i, act := range m.activities {
		isSel := i == m.sel

		// ── Line 1: marker + status + kind + title ──
		marker := "  "
		if isSel {
			marker = styleAccent.Render("› ")
		}

		statusIcon := m.statusIcon(act)
		icon := kindIcon(act.Kind)
		kind := strings.TrimSpace(act.Kind)
		if kind == "" {
			kind = "op"
		}
		title := actTitle(act)
		availW := maxInt(10, width-len(kind)-12)
		title = truncate(title, availW)

		// Kind first, then title
		var line1 string
		kindStr := icon + " " + kind
		if isSel {
			line1 = marker + statusIcon + " " + styleAccent.Render(kindStr) + styleDim.Render(" · ") + styleSelRow.Render(title)
		} else {
			line1 = marker + statusIcon + " " + styleMeta.Render(kindStr) + styleDim.Render(" · ") + styleUnselRow.Render(title)
		}

		// ── Line 2: timestamp · duration ──
		meta := m.buildMetaLine(act, width)
		var line2 string
		if isSel {
			line2 = styleSelRow.Render("    " + meta)
		} else {
			line2 = styleDim.Render("    " + meta)
		}

		lines = append(lines, line1, line2)
	}
	return lines
}

func (m *Model) statusIcon(act types.Activity) string {
	switch act.Status {
	case types.ActivityOK:
		return styleOK.Render("✓")
	case types.ActivityError:
		return styleErr.Render("✗")
	default:
		frame := spinnerFrames[m.spinFrame%len(spinnerFrames)]
		return stylePending.Render(frame)
	}
}

func (m *Model) buildMetaLine(act types.Activity, width int) string {
	parts := make([]string, 0, 3)

	// Timestamp
	if !act.StartedAt.IsZero() {
		parts = append(parts, act.StartedAt.Format("15:04:05"))
	}
	if act.Duration > 0 {
		parts = append(parts, act.Duration.Truncate(time.Millisecond).String())
	}

	line := strings.Join(parts, " · ")
	return truncate(line, maxInt(10, width-6))
}

// ── Detail panel ───────────────────────────────────────────────────────

func (m *Model) renderDetailPanel(width, height int) string {
	act := m.activities[m.sel]
	innerW := maxInt(10, width-4)

	md := renderActivityDetailMD(act)
	rendered := renderMarkdown(md, innerW)

	// Apply viewport scroll
	rendered = viewportSlice(rendered, height-2, m.detailScroll)

	borderColor := colorAccent
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width - 2). // account for border
		Height(height - 2).
		Render(rendered)
}

func renderActivityDetailMD(a types.Activity) string {
	var b strings.Builder

	b.WriteString("### Activity Detail\n\n")

	// Fields
	if strings.TrimSpace(a.Kind) != "" {
		b.WriteString("- **Operation:** `")
		b.WriteString(a.Kind)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.Title) != "" {
		b.WriteString("- **Title:** ")
		b.WriteString(a.Title)
		b.WriteString("\n")
	}
	if strings.TrimSpace(a.Path) != "" {
		b.WriteString("- **Path:** `")
		b.WriteString(a.Path)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.From) != "" {
		b.WriteString("- **From:** `")
		b.WriteString(a.From)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.To) != "" {
		b.WriteString("- **To:** `")
		b.WriteString(a.To)
		b.WriteString("`\n")
	}

	// Status + duration
	b.WriteString("- **Status:** ")
	b.WriteString(string(a.Status))
	b.WriteString(" ")
	b.WriteString(a.ShortStatus())
	if a.Duration > 0 {
		b.WriteString(" · ")
		b.WriteString(a.Duration.Truncate(time.Millisecond).String())
	}
	b.WriteString("\n")

	if !a.StartedAt.IsZero() {
		b.WriteString("- **Started:** ")
		b.WriteString(a.StartedAt.Format("15:04:05"))
		b.WriteString("\n")
	}

	// Error
	if strings.TrimSpace(a.Error) != "" {
		b.WriteString("\n**Error**\n\n")
		b.WriteString("```\n")
		b.WriteString(a.Error)
		b.WriteString("\n```\n")
	}

	// Data map (arguments)
	if len(a.Data) > 0 {
		b.WriteString("\n**Arguments**\n\n")
		for k, v := range a.Data {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			// Skip meta keys already shown above
			switch k {
			case "op", "ok", "err", "opId":
				continue
			}
			if len(v) > 200 {
				v = v[:197] + "…"
			}
			b.WriteString("- `")
			b.WriteString(k)
			b.WriteString("`: ")
			b.WriteString(v)
			b.WriteString("\n")
		}
	}

	// Output preview
	if strings.TrimSpace(a.OutputPreview) != "" {
		b.WriteString("\n**Output Preview**\n\n")
		preview := a.OutputPreview
		if len(preview) > 800 {
			preview = preview[:797] + "…"
		}
		b.WriteString("```\n")
		b.WriteString(preview)
		b.WriteString("\n```\n")
	}

	// Written content preview (fs_write / fs_append)
	if strings.TrimSpace(a.TextPreview) != "" && !a.TextRedacted {
		b.WriteString("\n**Written Content**")
		if a.TextTruncated {
			b.WriteString(" _(truncated)_")
		}
		b.WriteString("\n\n```\n")
		b.WriteString(a.TextPreview)
		b.WriteString("\n```\n")
	}

	return b.String()
}

// ── Helpers ────────────────────────────────────────────────────────────

// actTitle builds the primary display title for an activity row.
func actTitle(a types.Activity) string {
	title := strings.TrimSpace(a.Title)
	if title != "" {
		return title
	}
	kind := strings.TrimSpace(a.Kind)
	if kind == "" {
		kind = "op"
	}
	path := strings.TrimSpace(a.Path)
	if path != "" {
		return kind + " " + path
	}
	return kind
}

func kindIcon(kind string) string {
	switch {
	case strings.HasPrefix(kind, "fs_"):
		return "📄"
	case kind == "shell_exec":
		return "⚡"
	case kind == "http_fetch":
		return "🌐"
	case kind == "browser" || strings.HasPrefix(kind, "browser."):
		return "🖥"
	case kind == "agent_spawn":
		return "🤖"
	case kind == "code_exec":
		return "🐍"
	case kind == "email":
		return "📧"
	case kind == "task_create":
		return "📋"
	case kind == "trace_run":
		return "🔍"
	case strings.HasPrefix(kind, "ui."):
		return "🖼"
	case strings.HasPrefix(kind, "workdir"):
		return "📂"
	case strings.HasPrefix(kind, "llm."):
		return "🔗"
	default:
		return "⚙"
	}
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
	return runewidth.Truncate(s, max-1, "") + "…"
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

func renderMarkdown(md string, width int) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	if width <= 0 {
		width = 40
	}

	r, err := markdownRenderer(width)
	if err != nil {
		return mdFallback.Render(md)
	}
	out, err := r.Render(md)
	if err != nil {
		return mdFallback.Render(md)
	}
	return strings.TrimRight(out, "\n")
}

func markdownRenderer(width int) (*glamour.TermRenderer, error) {
	mdMu.Lock()
	defer mdMu.Unlock()

	if r, ok := mdByWidth[width]; ok {
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
	mdByWidth[width] = r
	return r, nil
}
