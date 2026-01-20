package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

func splitThinkingText(s string) (header string, summary string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	parts := strings.SplitN(s, "\n\n", 2)
	header = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		summary = strings.TrimSpace(parts[1])
	}
	return header, summary
}

func (m *Model) addTranscriptItem(it transcriptItem) {
	wasAtBottom := m.transcript.AtBottom()
	wasEmpty := len(m.transcriptItems) == 0

	m.transcriptItems = append(m.transcriptItems, it)
	m.rebuildTranscript()
	// If the user was at the bottom, keep them there (chat behavior). Otherwise,
	// preserve their scroll position.
	if wasEmpty {
		// For the first item, keep the top visible (avoid "first message is cut off").
		m.transcript.SetYOffset(0)
	} else if wasAtBottom {
		m.transcript.GotoBottom()
	}
}

func (m *Model) rebuildTranscript() {
	// Important: wrap content to the *actual* transcript viewport width.
	//
	// If we wrap to a larger width than the viewport, the terminal will soft-wrap
	// long lines. When the wrapped line count exceeds the terminal height, the
	// screen scrolls and the header appears to "disappear".
	w := max(20, m.transcript.Width)

	// Allocate inner widths so the *rendered* transcript never exceeds `w`,
	// accounting for borders/padding and speaker chrome.
	//
	// User box chrome:
	//   - border: 1 left + 1 right
	//   - padding: 1 left + 1 right
	//   - accent bar: 1 (│) + 1 spacer
	//   => 6 columns of overhead
	//
	// Agent box chrome:
	//   - padding: 1 left + 1 right
	//   => 2 columns of overhead
	//
	// File-change box chrome:
	//   - border: 1 left + 1 right
	//   - padding: 1 left + 1 right
	//   => 4 columns of overhead
	userInnerW := max(20, w-6)
	agentInnerW := max(20, w-2)
	fileInnerW := max(20, w-4)

	lines := make([]string, 0, len(m.transcriptItems))
	startLines := make([]int, 0, len(m.transcriptItems))
	lineNo := 0
	for _, it := range m.transcriptItems {
		startLines = append(startLines, lineNo)
		switch it.kind {
		case transcriptSpacer:
			lines = append(lines, m.styleDim.Render(""))
			lineNo++
		case transcriptUser:
			// Render user text as markdown so pasted tasks and lists are readable.
			// Glamour rendering can include leading/trailing newlines; trim them so the
			// user box hugs content (no phantom blank rows).
			rendered := strings.Trim(m.renderer.RenderMarkdown(it.text, userInnerW), "\n")
			h := 1
			if rendered != "" {
				h = 1 + strings.Count(rendered, "\n")
			}
			accentLines := make([]string, 0, h)
			for i := 0; i < h; i++ {
				accentLines = append(accentLines, "│")
			}
			accent := m.styleUserLabel.Render(strings.Join(accentLines, "\n"))
			body := lipgloss.JoinHorizontal(lipgloss.Top, accent, " ", rendered)
			lines = append(lines, m.styleUserBox.Render(body))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		case transcriptAgent:
			// Render agent answers as markdown (code blocks, bullets, tables).
			//
			// Important: do not prefix "agent>" inside the markdown source, otherwise
			// fenced blocks (```json) stop being recognized by the markdown parser.
			rendered := strings.Trim(m.renderer.RenderMarkdown(strings.TrimSpace(it.text), agentInnerW), "\n")
			body := m.styleAgent.Render(rendered)
			lines = append(lines, m.styleAgentBox.Render(body))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		case transcriptThinking:
			// Thinking indicator + optional summary (dimmed + subtle gutter).
			innerW := max(20, w-4) // 2 for gutter + 1 space + 1 margin
			header, summary := splitThinkingText(it.text)
			header = wrapText(strings.TrimSpace(header), innerW)
			if !m.thinkingExpanded && strings.TrimSpace(summary) != "" {
				header = strings.TrimSpace(header) + "  " + "summary available (Ctrl+Y)"
			}

			rendered := strings.TrimSpace(header)
			if m.thinkingExpanded && strings.TrimSpace(summary) != "" && m.renderer != nil {
				mdW := max(20, w-6) // account for gutter + space + a touch of margin
				md := strings.Trim(m.renderer.RenderThinkingMarkdown(summary, mdW), "\n")
				if md != "" {
					rendered = strings.TrimSpace(rendered) + "\n\n" + md
				}
			}
			rendered = strings.TrimRight(rendered, "\n")
			h := 1
			if rendered != "" {
				h = 1 + strings.Count(rendered, "\n")
			}
			gutterLines := make([]string, 0, h)
			for i := 0; i < h; i++ {
				gutterLines = append(gutterLines, "│")
			}
			gutter := m.styleDim.Render(strings.Join(gutterLines, "\n"))
			body := m.styleDim.Render(rendered)
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, gutter, " ", body))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		case transcriptError:
			lines = append(lines, m.styleError.Render(wrapText(it.text, max(20, w-4))))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		case transcriptAction:
			prefix := "• "
			if it.actionIsToolRun && !it.actionIsCompleted {
				prefix = "• Run "
			}
			if it.actionIsToolRun && it.actionIsCompleted {
				prefix = "• Ran "
			}
			line := m.styleAction.Render(prefix + wrapText(it.actionText, max(20, w-12)))
			if it.actionIsCompleted && strings.TrimSpace(it.actionCompletion) != "" {
				line += "  " + m.styleDim.Render(strings.TrimSpace(it.actionCompletion))
			}
			lines = append(lines, line)
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		case transcriptFileChange:
			raw := strings.TrimSpace(it.text)
			hdr, body, ok := strings.Cut(raw, "\n\n")
			if !ok || strings.TrimSpace(body) == "" {
				// Fallback: render as markdown.
				rendered := strings.Trim(m.renderer.RenderMarkdown(raw, fileInnerW), "\n")
				lines = append(lines, m.styleFileChangeBox.Render(rendered))
				lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
				break
			}

			path, added, deleted, hasCounts := parseFileChangeHeaderLine(hdr)
			headerRendered := renderFileChangeHeaderLine(path, added, deleted, hasCounts, fileInnerW)
			bodyRendered := strings.Trim(m.renderer.RenderMarkdown(strings.TrimSpace(body), fileInnerW), "\n")
			rendered := headerRendered + "\n\n" + bodyRendered
			lines = append(lines, m.styleFileChangeBox.Render(rendered))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		}
	}

	m.transcriptItemStartLine = startLines
	m.transcript.SetContent(strings.Join(lines, "\n"))
	// Clamp scroll when content shrinks or re-wrap changes line count.
	m.transcript.SetYOffset(m.transcript.YOffset)
}

func prefixFirstLine(s string, prefix string) string {
	if s == "" {
		return prefix
	}
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return prefix + s[:idx] + "\n" + s[idx+1:]
	}
	return prefix + s
}

func (m *Model) scrollToCurrentTurnStart() {
	// Keep the current turn "anchored" so the user message + action lines remain visible
	// even when the agent answer is long.
	idx := m.lastTurnUserItemIdx
	if idx < 0 || idx >= len(m.transcriptItemStartLine) {
		return
	}
	m.transcript.SetYOffset(m.transcriptItemStartLine[idx])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncateMiddle(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen < 8 {
		return s[:maxLen]
	}
	keep := (maxLen - 1) / 2
	return s[:keep] + "…" + s[len(s)-keep:]
}

func firstLine(s string) string {
	s = strings.ReplaceAll(s, "\r", "")
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return truncateMiddle(s, 48)
}

func wrapText(s string, width int) string {
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimRight(s, "\n")
	if width <= 0 {
		return s
	}
	// Use a dedicated wrapping lib so reflow behaves consistently across width changes.
	return wordwrap.String(s, width)
}

func parseFileChangeHeaderLine(hdr string) (path string, added int, deleted int, hasCounts bool) {
	hdr = strings.ReplaceAll(hdr, "\r", "")
	hdr = strings.TrimSpace(hdr)
	if hdr == "" {
		return "", 0, 0, false
	}
	fields := strings.Fields(hdr)
	if len(fields) < 3 {
		return hdr, 0, 0, false
	}
	// Expect: "<path> ... +N -M" (we only care about the trailing +N/-M).
	last := fields[len(fields)-1]
	prev := fields[len(fields)-2]
	if !strings.HasPrefix(prev, "+") || !strings.HasPrefix(last, "-") {
		return hdr, 0, 0, false
	}
	// Parse ints; tolerate "+0" "-0".
	a := strings.TrimPrefix(prev, "+")
	d := strings.TrimPrefix(last, "-")
	// manual Atoi without importing strconv here (keep file imports stable)
	parseInt := func(s string) (int, bool) {
		if s == "" {
			return 0, false
		}
		n := 0
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c < '0' || c > '9' {
				return 0, false
			}
			n = n*10 + int(c-'0')
		}
		return n, true
	}
	ai, okA := parseInt(a)
	di, okD := parseInt(d)
	if !okA || !okD {
		return hdr, 0, 0, false
	}
	path = strings.Join(fields[:len(fields)-2], " ")
	return path, ai, di, true
}

func renderFileChangeHeaderLine(path string, added int, deleted int, hasCounts bool, width int) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "unknown"
	}
	// Styles: path (blue), + (green), - (red).
	stylePath := lipgloss.NewStyle().Foreground(lipgloss.Color("#6bbcff")).Bold(true)
	stylePlus := lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
	styleMinus := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true)

	// Hide +0/-0 in the header (user preference).
	if !hasCounts || (added == 0 && deleted == 0) {
		// Truncate path to fit.
		maxPath := max(8, width)
		path = truncateMiddle(path, maxPath)
		return stylePath.Render(path)
	}

	plus := stylePlus.Render("+" + itoa(added))
	minus := styleMinus.Render("-" + itoa(deleted))

	// Fit within width by truncating the path.
	countsRaw := "  +" + itoa(added) + " -" + itoa(deleted)
	availPath := max(8, width-lipgloss.Width(countsRaw))
	path = truncateMiddle(path, availPath)
	return stylePath.Render(path) + "  " + plus + " " + minus
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		// Should not happen for our use, but handle defensively.
		return "-" + itoa(-n)
	}
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + (n % 10))
		n /= 10
	}
	return string(b[i:])
}

func truncateRight(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen < 2 {
		return s[:maxLen]
	}
	return s[:maxLen-1] + "…"
}
