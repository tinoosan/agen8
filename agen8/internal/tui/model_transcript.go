package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/agen8/internal/tui/kit"
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
	m.addTranscriptItemWithScroll(it, true)
}

func (m *Model) addTranscriptItemWithScroll(it transcriptItem, autoScroll bool) {
	wasAtBottom := m.transcriptAtBottom()
	wasEmpty := len(m.transcriptItems) == 0

	if it.kind != transcriptActionGroup {
		m.resetActionGroupState()
	}

	m.transcriptItems = append(m.transcriptItems, it)
	m.rebuildTranscript()
	// If the user was at the bottom, keep them there (chat behavior). Otherwise,
	// preserve their scroll position.
	if wasEmpty {
		// For the first item, keep the top visible (avoid "first message is cut off").
		m.transcriptSetYOffsetGlobal(0)
	} else if autoScroll && wasAtBottom {
		m.transcriptGotoBottom()
	}
}

func (m *Model) rebuildTranscript() {
	if m == nil {
		return
	}
	// For small transcripts, keep the simple full rebuild.
	if len(m.transcriptItems) <= 50 {
		m.rebuildTranscriptFull()
		return
	}
	m.rebuildTranscriptWindowed()
}

func (m *Model) rebuildTranscriptFull() {
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
		rendered := m.renderTranscriptItem(it, w, userInnerW, agentInnerW, fileInnerW)
		lines = append(lines, rendered)
		lineNo += 1 + strings.Count(rendered, "\n")
	}

	m.transcriptItemStartLine = startLines
	m.transcriptTotalLines = lineNo
	m.transcriptLogicalYOffset = m.transcript.YOffset
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
	m.transcriptSetYOffsetGlobal(m.transcriptItemStartLine[idx])
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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
	return kit.TruncateMiddle(s, 48)
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

func (m *Model) transcriptIsWindowed() bool {
	return len(m.transcriptItems) > 50
}

func (m *Model) transcriptAtBottom() bool {
	if m == nil {
		return true
	}
	if !m.transcriptIsWindowed() {
		return m.transcript.AtBottom()
	}
	bottom := max(0, m.transcriptTotalLines-m.transcript.Height)
	return m.transcriptLogicalYOffset >= bottom
}

func (m *Model) transcriptGotoBottom() {
	if m == nil {
		return
	}
	if !m.transcriptIsWindowed() {
		m.transcript.GotoBottom()
		m.transcriptLogicalYOffset = m.transcript.YOffset
		return
	}
	m.transcriptLogicalYOffset = max(0, m.transcriptTotalLines-m.transcript.Height)
	m.rebuildTranscript()
}

func (m *Model) transcriptSetYOffsetGlobal(y int) {
	if m == nil {
		return
	}
	if !m.transcriptIsWindowed() {
		m.transcript.SetYOffset(y)
		m.transcriptLogicalYOffset = m.transcript.YOffset
		return
	}
	maxY := max(0, m.transcriptTotalLines-m.transcript.Height)
	if y < 0 {
		y = 0
	}
	if y > maxY {
		y = maxY
	}
	m.transcriptLogicalYOffset = y
	m.rebuildTranscript()
}

func (m *Model) renderTranscriptItem(it transcriptItem, w, userInnerW, agentInnerW, fileInnerW int) string {
	switch it.kind {
	case transcriptSpacer:
		return m.styleDim.Render("")
	case transcriptUser:
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
		return m.styleUserBox.Render(body)
	case transcriptAgent:
		rendered := strings.Trim(m.renderer.RenderAgentMarkdown(strings.TrimSpace(it.text), agentInnerW), "\n")
		return m.styleAgentBox.Render(rendered)
	case transcriptThinking:
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
		return lipgloss.JoinHorizontal(lipgloss.Top, gutter, " ", body)
	case transcriptError:
		return m.styleError.Render(wrapText(it.text, max(20, w-4)))
	case transcriptActionGroup:
		header := strings.TrimSpace(it.groupHeader)
		if header == "" {
			header = "Action"
		}
		var b strings.Builder
		b.WriteString(m.styleBold.Render("• " + header))
		b.WriteString("\n")
		for i, item := range it.groupItems {
			connector := "├"
			if i == len(it.groupItems)-1 {
				connector = "└"
			}
			actionW := max(20, w-12)
			content := wrapText(strings.TrimSpace(item.text), actionW)
			line := m.styleDim.Render(connector+" ") + m.styleAction.Render(content)
			if item.status != "" {
				style := m.styleDim
				if item.isError {
					style = m.styleError
				}
				line += " " + style.Render(item.status)
			}
			b.WriteString(line)
			if i != len(it.groupItems)-1 {
				b.WriteString("\n")
			}
		}
		return b.String()
	case transcriptFileChange:
		raw := strings.TrimSpace(it.text)
		if strings.HasPrefix(strings.TrimSpace(raw), "## File changes") {
			rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "## File changes"))
			rest = strings.TrimLeft(rest, "\n")
			parts := []string{}
			if strings.TrimSpace(rest) != "" {
				parts = strings.Split(rest, "\n---\n\n")
				if len(parts) == 1 {
					parts = strings.Split(rest, "\n---\n")
				}
			}

			title := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6bbcff")).
				Bold(true).
				Render("File changes")

			renderedParts := make([]string, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				nl := strings.IndexByte(part, '\n')
				if nl < 0 {
					renderedParts = append(renderedParts, strings.Trim(m.renderer.RenderMarkdown(part, fileInnerW), "\n"))
					continue
				}
				hdr := strings.TrimSpace(part[:nl])
				body := strings.TrimLeft(part[nl+1:], "\n")
				if strings.TrimSpace(body) == "" {
					renderedParts = append(renderedParts, strings.Trim(m.renderer.RenderMarkdown(part, fileInnerW), "\n"))
					continue
				}
				pth, a, d, okCounts := parseFileChangeHeaderLine(hdr)
				hdrR := renderFileChangeHeaderLine(pth, a, d, okCounts, fileInnerW)
				bodyR := strings.Trim(m.renderer.RenderMarkdown(strings.TrimSpace(body), fileInnerW), "\n")
				renderedParts = append(renderedParts, hdrR+"\n"+bodyR)
			}

			dividerW := fileInnerW
			if dividerW > 32 {
				dividerW = 32
			}
			if dividerW < 10 {
				dividerW = 10
			}
			divider := m.styleDim.Render(strings.Repeat("─", dividerW))
			content := title
			for i, rp := range renderedParts {
				if strings.TrimSpace(rp) == "" {
					continue
				}
				content += "\n" + rp
				if i != len(renderedParts)-1 {
					content += "\n\n" + divider + "\n"
				}
			}
			return m.styleFileChangeBox.Render(strings.TrimRight(content, "\n"))
		}

		nl := strings.IndexByte(raw, '\n')
		if nl < 0 {
			rendered := strings.Trim(m.renderer.RenderMarkdown(raw, fileInnerW), "\n")
			return m.styleFileChangeBox.Render(rendered)
		}
		hdr := strings.TrimSpace(raw[:nl])
		body := strings.TrimLeft(raw[nl+1:], "\n")
		if strings.TrimSpace(body) == "" {
			rendered := strings.Trim(m.renderer.RenderMarkdown(raw, fileInnerW), "\n")
			return m.styleFileChangeBox.Render(rendered)
		}

		path, added, deleted, hasCounts := parseFileChangeHeaderLine(hdr)
		headerRendered := renderFileChangeHeaderLine(path, added, deleted, hasCounts, fileInnerW)
		bodyRendered := strings.Trim(m.renderer.RenderMarkdown(strings.TrimSpace(body), fileInnerW), "\n")
		rendered := headerRendered + "\n" + bodyRendered
		return m.styleFileChangeBox.Render(rendered)
	default:
		return it.text
	}
}

func (m *Model) estimateItemHeight(idx int) int {
	if idx < 0 || idx >= len(m.transcriptItems) {
		return 1
	}
	it := m.transcriptItems[idx]
	w := max(40, m.transcript.Width)
	switch it.kind {
	case transcriptSpacer:
		return 1
	case transcriptUser:
		lines := 3
		if w > 6 {
			lines += len(it.text) / max(1, (w-6))
		}
		return max(3, lines)
	case transcriptAgent:
		lines := 2
		if w > 2 {
			lines += len(it.text) / max(1, (w-2))
		}
		if lines > 100 {
			lines = 100
		}
		return max(2, lines)
	case transcriptThinking:
		return 3 + strings.Count(it.text, "\n")
	case transcriptActionGroup:
		return 2 + len(it.groupItems)*2
	case transcriptFileChange:
		return 6 + strings.Count(it.text, "\n")
	case transcriptError:
		lines := 2
		if w > 4 {
			lines += len(it.text) / max(1, (w-4))
		}
		return max(2, lines)
	default:
		return 3
	}
}

func (m *Model) getItemHeight(idx int) int {
	if m.transcriptRenderCache != nil {
		if cached, ok := m.transcriptRenderCache[idx]; ok {
			if cached.width == m.transcript.Width {
				return cached.height
			}
		}
	}
	return m.estimateItemHeight(idx)
}

func (m *Model) findItemAtLine(lineNo int) int {
	n := len(m.transcriptItemStartLine)
	if n == 0 {
		return 0
	}
	if lineNo <= 0 {
		return 0
	}
	lo, hi := 0, n-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if m.transcriptItemStartLine[mid] <= lineNo {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

func (m *Model) rebuildTranscriptWindowed() {
	w := max(20, m.transcript.Width)
	userInnerW := max(20, w-6)
	agentInnerW := max(20, w-2)
	fileInnerW := max(20, w-4)

	if m.transcriptRenderCache == nil {
		m.transcriptRenderCache = make(map[int]*renderedItem)
	}

	// Update start line table using cached heights/estimates.
	startLines := make([]int, len(m.transcriptItems))
	lineNo := 0
	for i := range m.transcriptItems {
		startLines[i] = lineNo
		lineNo += m.getItemHeight(i)
	}
	m.transcriptItemStartLine = startLines
	m.transcriptTotalLines = lineNo

	// Clamp logical scroll.
	maxY := max(0, m.transcriptTotalLines-m.transcript.Height)
	if m.transcriptLogicalYOffset < 0 {
		m.transcriptLogicalYOffset = 0
	}
	if m.transcriptLogicalYOffset > maxY {
		m.transcriptLogicalYOffset = maxY
	}

	visibleStart := m.transcriptLogicalYOffset
	visibleEnd := visibleStart + m.transcript.Height

	firstVisible := m.findItemAtLine(visibleStart)
	lastVisible := m.findItemAtLine(visibleEnd)
	firstVisible = max(0, firstVisible)
	lastVisible = min(len(m.transcriptItems)-1, lastVisible)

	bufferAbove := 10
	bufferBelow := 10
	firstRender := max(0, firstVisible-bufferAbove)
	lastRender := min(len(m.transcriptItems)-1, lastVisible+bufferBelow)

	windowStartLine := 0
	if len(m.transcriptItemStartLine) != 0 {
		windowStartLine = m.transcriptItemStartLine[firstRender]
	}

	lines := make([]string, 0, lastRender-firstRender+1)
	for i := firstRender; i <= lastRender; i++ {
		if cached, ok := m.transcriptRenderCache[i]; ok && cached.width == w {
			lines = append(lines, cached.rendered)
			continue
		}
		rendered := m.renderTranscriptItem(m.transcriptItems[i], w, userInnerW, agentInnerW, fileInnerW)
		height := 1 + strings.Count(rendered, "\n")
		m.transcriptRenderCache[i] = &renderedItem{rendered: rendered, height: height, width: w}
		lines = append(lines, rendered)
	}

	m.transcriptWindow = transcriptWindow{
		firstItem:        firstRender,
		lastItem:         lastRender,
		bufferAbove:      bufferAbove,
		bufferBelow:      bufferBelow,
		contentStartLine: windowStartLine,
	}

	m.transcript.SetContent(strings.Join(lines, "\n"))
	rel := m.transcriptLogicalYOffset - windowStartLine
	if rel < 0 {
		rel = 0
	}
	m.transcript.SetYOffset(rel)
}

func (m *Model) cleanupRenderCache() {
	if m == nil || m.transcriptRenderCache == nil {
		return
	}
	if len(m.transcriptItems) == 0 {
		m.transcriptRenderCache = nil
		return
	}
	firstVisible := m.findItemAtLine(m.transcriptLogicalYOffset)
	lastVisible := m.findItemAtLine(m.transcriptLogicalYOffset + m.transcript.Height)
	keepRadius := 100
	keepFrom := max(0, firstVisible-keepRadius)
	keepTo := min(len(m.transcriptItems)-1, lastVisible+keepRadius)

	for idx := range m.transcriptRenderCache {
		if idx < keepFrom || idx > keepTo {
			delete(m.transcriptRenderCache, idx)
		}
	}
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
		path = kit.TruncateMiddle(path, maxPath)
		return stylePath.Render(path)
	}

	plus := stylePlus.Render("+" + itoa(added))
	minus := styleMinus.Render("-" + itoa(deleted))

	// Fit within width by truncating the path.
	countsRaw := "  +" + itoa(added) + " -" + itoa(deleted)
	availPath := max(8, width-lipgloss.Width(countsRaw))
	path = kit.TruncateMiddle(path, availPath)
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
