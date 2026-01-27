package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/types"
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
	wasAtBottom := m.transcript.AtBottom()
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
		m.transcript.SetYOffset(0)
	} else if autoScroll && wasAtBottom {
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
			rendered := strings.Trim(m.renderer.RenderAgentMarkdown(strings.TrimSpace(it.text), agentInnerW), "\n")
			lines = append(lines, m.styleAgentBox.Render(rendered))
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
		case transcriptActionGroup:
			header := strings.TrimSpace(it.groupHeader)
			if header == "" {
				header = "Action"
			}
			lines = append(lines, m.styleBold.Render("• "+header))
			lineNo++

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
				lines = append(lines, line)
				lineNo += 1 + strings.Count(line, "\n")
			}
		case transcriptFileChange:
			raw := strings.TrimSpace(it.text)
			// Grouped file-changes block: render each file entry using the same
			// header+diff formatting as single-file blocks.
			//
			// Hypothesis H2: In grouped blocks, our previous "first line is header"
			// logic treated "## File changes" as the header, leaving per-file headers
			// unstyled and formatted differently than the single-file case.
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

				// Render a plain title (no markdown "##") so it doesn't look like raw markdown.
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
						// Fallback: render as markdown.
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
				lines = append(lines, m.styleFileChangeBox.Render(strings.TrimRight(content, "\n")))
				lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
				break
			}

			// Header is the first line; body is the rest. (We intentionally tolerate
			// either "\n" or "\n\n" between them.)
			nl := strings.IndexByte(raw, '\n')
			if nl < 0 {
				// Fallback: render as markdown.
				rendered := strings.Trim(m.renderer.RenderMarkdown(raw, fileInnerW), "\n")
				lines = append(lines, m.styleFileChangeBox.Render(rendered))
				lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
				break
			}
			hdr := strings.TrimSpace(raw[:nl])
			body := strings.TrimLeft(raw[nl+1:], "\n")
			if strings.TrimSpace(body) == "" {
				// Fallback: render as markdown.
				rendered := strings.Trim(m.renderer.RenderMarkdown(raw, fileInnerW), "\n")
				lines = append(lines, m.styleFileChangeBox.Render(rendered))
				lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
				break
			}

			path, added, deleted, hasCounts := parseFileChangeHeaderLine(hdr)
			headerRendered := renderFileChangeHeaderLine(path, added, deleted, hasCounts, fileInnerW)
			bodyRendered := strings.Trim(m.renderer.RenderMarkdown(strings.TrimSpace(body), fileInnerW), "\n")
			// Keep the diff tight to the header (one newline).
			rendered := headerRendered + "\n" + bodyRendered
			lines = append(lines, m.styleFileChangeBox.Render(rendered))
			lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
		case transcriptApprovalRequest:
			diffContent := strings.TrimSpace(it.approvalDiff)
			if diffContent != "" {
				path := ""
				if it.approvalOp != nil {
					path = strings.TrimSpace(it.approvalOp.Path)
				}
				displayPath := strings.TrimPrefix(path, "/")
				if strings.TrimSpace(displayPath) == "" {
					displayPath = strings.TrimSpace(path)
				}
				added, deleted, hasCounts := approvalPreviewCounts(diffContent)
				headerRendered := renderFileChangeHeaderLine(displayPath, added, deleted, hasCounts, fileInnerW)
				bodyRendered := strings.Trim(m.renderer.RenderMarkdown(diffContent, fileInnerW), "\n")
				if bodyRendered == "" {
					bodyRendered = "_(no preview available)_"
				}

				statusText := "⏳ Waiting for approval..."
				statusStyle := m.styleDim
				switch strings.ToLower(strings.TrimSpace(it.approvalStatus)) {
				case "approved":
					statusText = "✅ Approved"
					statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
				case "denied":
					statusText = "❌ Denied"
					statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true)
				}

				content := headerRendered
				if bodyRendered != "" {
					content += "\n" + bodyRendered
				}
				content = strings.TrimRight(content, "\n")
				content += "\n" + statusStyle.Render(statusText)
				lines = append(lines, m.styleFileChangeBox.Render(content))
				lineNo += 1 + strings.Count(lines[len(lines)-1], "\n")
				break
			}

			var req types.HostOpRequest
			if it.approvalOp != nil {
				req = *it.approvalOp
			}
			title, desc := approvalPromptText(req)
			headerText := "Approval Required: " + strings.TrimSpace(title)
			if strings.TrimSpace(title) == "" {
				if strings.TrimSpace(req.Op) != "" {
					headerText = "Approval Required: " + req.Op
				} else if strings.TrimSpace(req.Path) != "" {
					headerText = "Approval Required: " + req.Path
				} else {
					headerText = "Approval Required"
				}
			}
			header := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#ffb347")).
				Bold(true).
				Render(kit.TruncateRight(headerText, fileInnerW))

			bodyParts := make([]string, 0, 2)
			if strings.TrimSpace(desc) != "" {
				bodyParts = append(bodyParts, desc)
			}
			if diffContent != "" {
				bodyParts = append(bodyParts, diffContent)
			}
			if len(bodyParts) == 0 {
				bodyParts = append(bodyParts, "_(no preview available)_")
			}
			body := strings.Join(bodyParts, "\n\n")
			renderedBody := strings.Trim(m.renderer.RenderMarkdown(body, fileInnerW), "\n")

			statusText := "Waiting for approval..."
			statusStyle := m.styleDim
			switch strings.ToLower(strings.TrimSpace(it.approvalStatus)) {
			case "approved":
				statusText = "✅ Approved"
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950")).Bold(true)
			case "denied":
				statusText = "❌ Denied"
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).Bold(true)
			}
			status := statusStyle.Render(statusText)

			content := header
			if renderedBody != "" {
				content += "\n" + renderedBody
			}
			content += "\n" + status
			lines = append(lines, m.styleFileChangeBox.Render(strings.TrimRight(content, "\n")))
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

func approvalPreviewCounts(diff string) (int, int, bool) {
	preview := stripDiffFences(diff)
	if preview == "" {
		return 0, 0, false
	}
	added, deleted := diffStat(preview)
	return added, deleted, added != 0 || deleted != 0
}

func stripDiffFences(diff string) string {
	s := strings.TrimSpace(diff)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		} else {
			return ""
		}
	}
	if idx := strings.LastIndex(s, "\n```"); idx >= 0 {
		s = s[:idx]
	} else if strings.HasSuffix(s, "```") {
		s = strings.TrimSuffix(s, "```")
	}
	return strings.Trim(s, "\n")
}
