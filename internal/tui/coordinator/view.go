package coordinator

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

// ── Color palette ──────────────────────────────────────────────────────

var (
	colorOK       = lipgloss.Color("#98c379")
	colorErr      = lipgloss.Color("#e06c75")
	colorPending  = lipgloss.Color("#e5c07b")
	colorAccent   = lipgloss.Color("#7aa2f7")
	colorThinking = lipgloss.Color("#9d7fdb") // muted purple for thinking
	colorPlan     = lipgloss.Color("#56b6c2") // teal for plan updates

	styleOK        = lipgloss.NewStyle().Foreground(colorOK)
	styleErr       = lipgloss.NewStyle().Foreground(colorErr)
	stylePending   = lipgloss.NewStyle().Foreground(colorPending)
	styleAccent    = lipgloss.NewStyle().Foreground(colorAccent)
	styleThinking  = lipgloss.NewStyle().Foreground(colorThinking)
	stylePlan      = lipgloss.NewStyle().Foreground(colorPlan)
	styleHeader    = lipgloss.NewStyle().Bold(true)
	stylePillOK    = lipgloss.NewStyle().Bold(true).Foreground(colorOK).Reverse(true).Padding(0, 1)
	stylePillErr   = lipgloss.NewStyle().Bold(true).Foreground(colorErr).Reverse(true).Padding(0, 1)
	stylePillDim   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#707070")).Reverse(true).Padding(0, 1)
	stylePillWhite = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
	styleVerbBold  = lipgloss.NewStyle().Bold(true)

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

	// Agent status pill
	if m.agentStatus != "" && m.agentStatus != "Idle" {
		var statusPill string
		switch {
		case strings.Contains(m.agentStatus, "Error"):
			statusPill = stylePillErr.Render(m.agentStatus)
		case strings.Contains(m.agentStatus, "Done"):
			statusPill = stylePillOK.Render(m.agentStatus)
		default:
			statusPill = stylePillWhite.Render(m.agentStatus + " " + m.spinner())
		}
		tags = append(tags, statusPill)
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
		case feedThinking:
			// Thinking entries are injected inline into the current agent turn,
			// or rendered as a standalone dim line if no agent turn is active.
			if curAgent != nil {
				curAgent.entries = append(curAgent.entries, e)
				if e.timestamp.After(curAgent.timestamp) {
					curAgent.timestamp = e.timestamp
				}
			} else {
				flushAgent()
				turns = append(turns, conversationTurn{
					kind:      turnThinking,
					timestamp: e.timestamp,
					text:      e.text,
					entries:   []feedEntry{e},
				})
			}
		case feedAgent:
			role := fallback(e.role, "agent")

			// Flush if role changes, or if we are switching between text and ops records.
			// Also flush if both are text (to keep distinct text blocks separate).
			if curAgent != nil {
				if curAgent.role != role || curAgent.isText != e.isText || curAgent.isText {
					flushAgent()
				}
			}

			if curAgent == nil {
				curAgent = &conversationTurn{
					kind:      turnAgent,
					role:      role,
					timestamp: e.timestamp,
					isText:    e.isText,
				}
				if e.isText {
					curAgent.text = e.text
				}
			}
			if e.timestamp.After(curAgent.timestamp) {
				curAgent.timestamp = e.timestamp
			}
			if !e.isText {
				curAgent.entries = append(curAgent.entries, e)
			}
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

// groupBridgeToolCalls collapses code_exec bridge tool calls within each turn.
// A bridge call is identified by either:
//   - data["tag"] == "code_exec_bridge" (new activities), or
//   - temporal overlap: the entry started while a code_exec was still running
//     (entry.timestamp >= code_exec.timestamp && entry.timestamp <= code_exec.finishedAt)
func groupBridgeToolCalls(turns []conversationTurn) []conversationTurn {
	for ti := range turns {
		t := &turns[ti]
		if t.kind != turnAgent || t.isText || len(t.entries) == 0 {
			continue
		}

		var filtered []feedEntry
		var lastCodeExecIdx int = -1 // index in filtered slice

		for _, e := range t.entries {
			isBridge := false

			// Check tag first (works for new activities).
			if e.data != nil && strings.TrimSpace(e.data["tag"]) == "code_exec_bridge" {
				isBridge = true
			}

			// Temporal fallback: entry started during code_exec execution window.
			if !isBridge && lastCodeExecIdx >= 0 {
				ce := filtered[lastCodeExecIdx]
				if !ce.finishedAt.IsZero() &&
					!e.timestamp.Before(ce.timestamp) &&
					!e.timestamp.After(ce.finishedAt) {
					isBridge = true
				}
			}

			if isBridge && lastCodeExecIdx >= 0 {
				filtered[lastCodeExecIdx].childCount++
				// Promote plan items from collapsed bridge entries to the parent code_exec.
				if len(e.planItems) > 0 {
					filtered[lastCodeExecIdx].planItems = e.planItems
				}
				continue
			}

			if strings.ToLower(strings.TrimSpace(e.opKind)) == "code_exec" {
				lastCodeExecIdx = len(filtered)
			} else if lastCodeExecIdx >= 0 {
				// If this non-bridge entry started after code_exec finished,
				// clear the code_exec anchor so later entries aren't grouped.
				ce := filtered[lastCodeExecIdx]
				if !ce.finishedAt.IsZero() && e.timestamp.After(ce.finishedAt) {
					lastCodeExecIdx = -1
				}
			}
			filtered = append(filtered, e)
		}
		t.entries = filtered
	}
	return turns
}

func (m *Model) feedLines(width int) []string {
	turns := groupBridgeToolCalls(m.buildTurns())
	if len(turns) == 0 {
		return nil
	}

	inner := maxInt(12, width-4)
	lines := make([]string, 0, len(turns)*4)

	for i, t := range turns {
		switch t.kind {
		case turnUser:
			lines = append(lines, m.renderUserBlock(t, inner)...)
		case turnAgent:
			lines = append(lines, m.renderAgentBlock(t, inner)...)
		case turnSystem:
			lines = append(lines, m.renderSystemBlock(t, inner)...)
		case turnThinking:
			if len(t.entries) > 0 {
				lines = append(lines, m.renderThinkingBlock(t.entries[0], inner)...)
			}
		}
		// Separator between blocks is just an empty line
		if i < len(turns)-1 {
			lines = append(lines, "")
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
	label := styleAccent.Bold(true).Render("You")
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := maxInt(1, inner-labelW-ageW)
	headerLine := "  " + label + strings.Repeat(" ", gap) + ageStr

	msg := strings.TrimSpace(renderMarkdown(t.text, inner-2))
	outLines := []string{headerLine}
	for _, l := range strings.Split(msg, "\n") {
		outLines = append(outLines, "  "+l)
	}
	return outLines
}

// ── Agent block ────────────────────────────────────────────────────────
//
//  architect                                        30s ago
//   ● Read  src/auth/handler.go
//     └ Done
//     Bash  go test ./...
//     └ running ⠹

func (m *Model) renderAgentBlock(t conversationTurn, inner int) []string {
	if t.isText {
		msg := strings.TrimSpace(renderMarkdown(t.text, inner-4))
		outLines := []string{"  " + styleVerbBold.Render("●") + " " + styleVerbBold.Render(fallback(t.role, "agent"))}
		for _, l := range strings.Split(msg, "\n") {
			outLines = append(outLines, "    "+l)
		}
		return outLines
	}

	// Tool operations block
	age := relativeAge(t.timestamp.Format(time.RFC3339))
	role := truncate(t.role, maxInt(4, 14))
	label := kit.StyleDim.Render("● ") + styleAccent.Bold(true).Render(role)
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := maxInt(1, inner-labelW-ageW)
	headerLine := label + strings.Repeat(" ", gap) + ageStr

	lines := []string{headerLine}
	for _, e := range t.entries {
		// Thinking entries render as a collapsed/expanded block.
		if e.kind == feedThinking {
			lines = append(lines, m.renderThinkingBlock(e, inner)...)
			continue
		}

		// ── Standalone plan write ─────────────────────────────────
		// When the entry itself is a plan write with checklist items,
		// render as a dedicated "Updated plan" block and skip the
		// normal verb rendering entirely.
		if len(e.planItems) > 0 && isActivityPlanWrite(e.opKind, e.path) {
			lines = append(lines, renderPlanChecklist(e.planItems)...)
			continue
		}

		verb := kindToVerb(e.opKind, e.data)
		s := strings.ToLower(strings.TrimSpace(e.status))

		// Pick dot color based on status.
		var dot string
		switch {
		case s == "done" || s == "completed" || s == "ok" || s == "succeeded":
			dot = styleOK.Render("●")
		case s == "error" || s == "failed" || s == "canceled" || s == "cancelled":
			dot = styleErr.Render("●")
		case s == "running":
			dot = stylePending.Render("●")
		default:
			dot = kit.StyleDim.Render("●")
		}

		var argPreview string
		opLower := strings.ToLower(strings.TrimSpace(e.opKind))
		if opLower == "code_exec" {
			// Show just the verb — no arg preview for code_exec.
		} else if e.path != "" && isPathBasedOp(e.opKind) {
			argPreview = truncate(e.path, maxInt(8, inner-len(verb)-8))
		} else {
			argPreview = truncate(stripLeadingVerb(e.text, verb), maxInt(8, inner-len(verb)-8))
		}

		// Primary operation line: colored dot + bold verb + arg preview
		opLine := "  " + dot + " " + styleVerbBold.Render(verb)
		if argPreview != "" {
			//normal text
			opLine += " " + argPreview
		}
		lines = append(lines, opLine)

		// Collect sub-items (bridge summary + status) to render with tree branches.
		var subItems []string

		if e.childCount > 0 {
			subItems = append(subItems, "Ran "+styleVerbBold.Render(fmt.Sprintf("%d", e.childCount))+" tools")
		}

		var statusText string
		switch {
		case s == "running":
			statusText = kit.StyleDim.Render("running " + m.spinner())
		case s == "pending":
			statusText = kit.StyleDim.Render("pending ...")
		case s == "done" || s == "completed" || s == "ok" || s == "succeeded":
			statusText = kit.StyleDim.Render("ok")
		case s == "error" || s == "failed" || s == "canceled" || s == "cancelled":
			statusText = kit.StyleDim.Render("failed")
		default:
			statusText = kit.StyleDim.Render(e.status)
		}
		subItems = append(subItems, statusText)

		// Add vertical connector when there are multiple sub-items (extends branch ~25%).

		// Render sub-items with proper tree branches and extra horizontal space.
		for i, item := range subItems {
			var branch string
			if i < len(subItems)-1 {
				branch = styleVerbBold.Render("├")
			} else {
				branch = styleVerbBold.Render("└")
			}
			lines = append(lines, "  "+branch+"  "+item)
		}

		// ── Promoted plan checklist (from bridge tool calls) ──────
		// When a code_exec parent has plan items promoted from a
		// collapsed bridge child, render the checklist below the
		// normal operation sub-items.
		if len(e.planItems) > 0 {
			lines = append(lines, renderPlanChecklist(e.planItems)...)
		}
	}
	return lines
}

// ── Plan checklist ────────────────────────────────────────────────────

// renderPlanChecklist renders an "Updated plan" block with tree branches
// connecting each checklist item. Completed items show a green ✓, pending
// items show a dimmed ○.
//
//	● Updated plan
//	├─ ✓ Set up project structure
//	├─ ○ Add unit tests
//	└─ ○ Deploy to staging
func renderPlanChecklist(items []string) []string {
	lines := []string{"  " + stylePlan.Render("●") + " " + stylePlan.Bold(true).Render("Updated plan")}
	for i, item := range items {
		isLast := i == len(items)-1
		var branch string
		if isLast {
			branch = stylePlan.Render("  └─")
		} else {
			branch = stylePlan.Render("  ├─")
		}
		if strings.HasPrefix(item, "[x]") {
			text := strings.TrimPrefix(item, "[x] ")
			lines = append(lines, branch+" "+styleOK.Render("✓")+" "+kit.StyleDim.Strikethrough(true).Render(text))
		} else {
			text := strings.TrimPrefix(item, "[ ] ")
			lines = append(lines, branch+" "+kit.StyleDim.Render("○")+" "+text)
		}
	}
	return lines
}

// ── Thinking line ──────────────────────────────────────────────────────

// renderThinkingBlock renders a thinking feedEntry as collapsed or expanded.
// Collapsed (default):  ▸ Thought for 2s ◐            ctrl+o
// Expanded (ctrl+o):    ▾ Thought for 2s
//
//	│  line one...
//	└─ last line
func (m *Model) renderThinkingBlock(e feedEntry, inner int) []string {
	// Format duration label.
	var durStr string
	if e.live {
		durStr = "…"
	} else if e.thinkingDuration > 0 {
		if e.thinkingDuration < time.Second {
			durStr = fmt.Sprintf("%dms", e.thinkingDuration.Milliseconds())
		} else {
			durStr = fmt.Sprintf("%.0fs", e.thinkingDuration.Seconds())
		}
	}

	label := "Thought"
	if durStr != "" {
		label = "Thought for " + durStr
	}

	if !m.thinkingExpanded {
		// Collapsed: ▸ Thought for Ns ◐         ctrl+o
		hint := kit.StyleDim.Render("ctrl+o")
		var spinner string
		if e.live {
			spinner = " " + styleThinking.Render(m.spinner())
		}
		triangle := styleThinking.Render("▸")
		labelStr := styleThinking.Italic(true).Render(label) + spinner
		labelW := runewidth.StringWidth(stripANSI("▸ " + label + spinner))
		hintW := runewidth.StringWidth(stripANSI("ctrl+o"))
		gap := maxInt(1, inner-2-labelW-hintW)
		line := "  " + triangle + " " + labelStr + strings.Repeat(" ", gap) + hint
		return []string{line}
	}

	// Expanded: ▾ Thought for Ns + tree branches; one chunk per summary, markdown-rendered, separated by blank lines.
	triangle := styleThinking.Render("▾")
	headerLine := "  " + triangle + " " + styleThinking.Italic(true).Render(label)
	result := []string{headerLine}

	rawLines := e.thinkingLines
	if len(rawLines) == 0 {
		if e.live {
			rawLines = []string{m.spinner() + " thinking…"}
		} else {
			rawLines = []string{"(no summary available)"}
		}
	}

	// Each entry is a complete summary chunk from the daemon. Trim and filter empty.
	var chunks []string
	for _, l := range rawLines {
		l = strings.TrimSpace(l)
		if l != "" {
			chunks = append(chunks, l)
		}
	}
	if len(chunks) == 0 {
		chunks = rawLines
	}

	// Render each chunk as markdown and collect all content lines, with a blank line between chunks.
	// Prefix per line: "  " (2) + branch (4) + " " (1) = 7 visible chars.
	mdWidth := maxInt(20, inner-7)
	var contentLines []string
	for i, chunk := range chunks {
		rendered := strings.TrimSpace(renderMarkdown(chunk, mdWidth))
		if rendered != "" {
			for _, line := range strings.Split(rendered, "\n") {
				contentLines = append(contentLines, line)
			}
		}
		if i < len(chunks)-1 {
			contentLines = append(contentLines, "") // blank line between chunks
		}
	}
	if len(contentLines) == 0 {
		contentLines = append(contentLines, "(no summary available)")
	}

	// Last non-blank line gets └─; all others get │.
	lastNonBlank := -1
	for i := len(contentLines) - 1; i >= 0; i-- {
		if strings.TrimSpace(contentLines[i]) != "" {
			lastNonBlank = i
			break
		}
	}
	if lastNonBlank < 0 {
		lastNonBlank = len(contentLines) - 1
	}

	for i, l := range contentLines {
		var branch string
		if i == lastNonBlank {
			branch = styleThinking.Render("  └─")
		} else {
			branch = styleThinking.Render("  │ ")
		}
		if strings.TrimSpace(l) == "" {
			result = append(result, "  "+branch)
		} else {
			result = append(result, "  "+branch+" "+l)
		}
	}
	return result
}

// ── System block ───────────────────────────────────────────────────────
//
//   ◆ system                                           1m ago
//   Session paused

func (m *Model) renderSystemBlock(t conversationTurn, inner int) []string {
	age := relativeAge(t.timestamp.Format(time.RFC3339))
	label := stylePending.Bold(true).Render("system")
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := maxInt(1, inner-labelW-ageW)
	headerLine := "  " + label + strings.Repeat(" ", gap) + ageStr

	msg := strings.TrimSpace(renderMarkdown(t.text, inner-2))
	outLines := []string{headerLine}
	for _, l := range strings.Split(msg, "\n") {
		outLines = append(outLines, "  "+kit.StyleDim.Render(l))
	}
	return outLines
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

	// Render clean without bounds
	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).
		MaxHeight(2).
		Padding(0, 1). // Minimal indent
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
		hints = kit.StyleDim.Render("/help") + "  " +
			kit.StyleDim.Render("ctrl+c") + " quit"
	}

	leftW := runewidth.StringWidth(stripANSI(scrollLabel))
	rightW := runewidth.StringWidth(stripANSI(hints))
	gap := maxInt(1, m.width-2-leftW-rightW)

	line := scrollLabel + strings.Repeat(" ", gap) + hints
	return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).Render(line)
}

func (m *Model) spinner() string {
	return spinnerFrames[m.spinFrame%len(spinnerFrames)]
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

// stripLeadingVerb removes a leading verb word from text to avoid duplication
// when the verb is rendered separately (e.g. "Write /path" with verb "Write" → "/path").
func stripLeadingVerb(text, verb string) string {
	if verb == "" || text == "" {
		return text
	}
	if len(text) >= len(verb) &&
		strings.EqualFold(text[:len(verb)], verb) &&
		(len(text) == len(verb) || text[len(verb)] == ' ') {
		return strings.TrimSpace(text[len(verb):])
	}
	return text
}

// isPathBasedOp returns true for filesystem and related developer operations
// where the Path field should be prioritized for the argument preview.
func isPathBasedOp(kind string) bool {
	k := strings.ToLower(strings.TrimSpace(kind))
	if strings.HasPrefix(k, "fs_") {
		return true
	}
	switch k {
	case "browser", "shell_exec", "code_exec":
		return false
	}
	return false
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

// wrapText breaks a single line into multiple lines at word boundaries,
// each no wider than width characters.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	if runewidth.StringWidth(s) <= width {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{s}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		test := cur + " " + w
		if runewidth.StringWidth(test) > width {
			lines = append(lines, cur)
			cur = w
		} else {
			cur = test
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
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

// ── Markdown ───────────────────────────────────────────────────────────

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
		glamour.WithStyles(coordinatorMarkdownStyle()),
		glamour.WithWordWrap(width),
		glamour.WithPreservedNewLines(),
	)
	if err != nil {
		return nil, err
	}
	mdByWidth[width] = r
	return r, nil
}

func boolPtr(b bool) *bool       { return &b }
func uintPtr(u uint) *uint       { return &u }
func stringPtr(s string) *string { return &s }

func coordinatorMarkdownStyle() ansi.StyleConfig {
	style := styles.DarkStyleConfig

	// Reset document margins so the given wrap width is fully utilized
	style.Document.Margin = uintPtr(0)
	// Render markdown as markdown (hide raw markers like "###" and "**").
	style.H1.StylePrimitive.Prefix = ""
	style.H2.StylePrimitive.Prefix = ""
	style.H3.StylePrimitive.Prefix = ""
	style.H4.StylePrimitive.Prefix = ""
	style.H5.StylePrimitive.Prefix = ""
	style.H6.StylePrimitive.Prefix = ""

	style.Strong.BlockPrefix = ""
	style.Strong.BlockSuffix = ""
	style.Strong.Bold = boolPtr(true)

	style.Emph.BlockPrefix = ""
	style.Emph.BlockSuffix = ""
	style.Emph.Italic = boolPtr(true)

	style.Strikethrough.BlockPrefix = ""
	style.Strikethrough.BlockSuffix = ""

	style.Code.StylePrimitive.BlockPrefix = ""
	style.Code.StylePrimitive.BlockSuffix = ""

	style.CodeBlock.Margin = uintPtr(0)
	style.CodeBlock.Indent = uintPtr(0)
	style.CodeBlock.StylePrimitive.BlockPrefix = ""
	style.CodeBlock.StylePrimitive.BlockSuffix = "\n"

	return style
}
