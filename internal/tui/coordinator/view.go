package coordinator

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/tinoosan/agen8/internal/tui"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

var (
	styleHeader    = lipgloss.NewStyle().Bold(true)
	styleVerbBold  = lipgloss.NewStyle().Bold(true)
	styleArgItalic = lipgloss.NewStyle().Italic(true)

	mdMu       sync.Mutex
	mdByWidth  = map[int]*glamour.TermRenderer{}
	mdFallback = lipgloss.NewStyle()
)

const (
	compactWidth = 60
)

func (m *Model) isNarrow() bool { return m.width < compactWidth }

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
	base := lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
	if m.modelPicker.IsOpen() {
		return m.renderModelPicker(base)
	}
	return base
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
		tags = append(tags, kit.StyleOK.Render("● connected"))
	} else {
		tags = append(tags, kit.StyleErr.Render("● disconnected"))
	}

	// Agent status pill
	if m.agentStatus != "" && m.agentStatus != "Idle" {
		var statusPill string
		switch {
		case strings.Contains(m.agentStatus, "Error"):
			statusPill = kit.StylePillErr.Render(m.agentStatus)
		case strings.Contains(m.agentStatus, "Done"):
			statusPill = kit.StylePillOK.Render(m.agentStatus)
		default:
			statusPill = kit.StylePillWhite.Render(m.agentStatus + " " + m.spinner())
		}
		tags = append(tags, statusPill)
	}

	// Context fill pill
	if m.contextBudgetTokens > 0 {
		pct := m.contextTokens * 100 / m.contextBudgetTokens
		tags = append(tags, kit.RenderTag(kit.TagOptions{
			Key:   "ctx",
			Value: fmt.Sprintf("%dk/%dk (%d%%)", m.contextTokens/1000, m.contextBudgetTokens/1000, pct),
		}))
	}

	// Mode tag
	mode := kit.Fallback(m.sessionMode, "standalone")
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
		tags = append(tags, kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 32)))
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

func (m *Model) renderModelPicker(base string) string {
	dims := m.modelPicker.SetSize(m.width, m.height)
	content := m.modelPicker.View()
	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)
	_ = base
	return kit.RenderOverlay(opts)
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
			// If a non-text entry has no role (e.g. live-streamed bridge ops where
			// the role name hasn't been populated yet), inherit the role from the
			// currently-active agent turn so it doesn't cause a spurious flush and
			// duplicate "agent" block that flickers until the poll corrects it.
			role := strings.TrimSpace(e.role)
			if role == "" && curAgent != nil && !curAgent.isText {
				role = curAgent.role
			}
			role = kit.Fallback(role, "agent")

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
				if strings.ToLower(strings.TrimSpace(e.opKind)) == "code_exec" {
					// Hide the code_exec parent. Surface its children as flat entries:
					// write ops render individually; a synthetic summary carries the
					// non-write bridge count, status, and plan data.
					for _, wo := range e.bridgeWriteOps {
						curAgent.entries = append(curAgent.entries, wo)
					}
					s := strings.ToLower(strings.TrimSpace(e.status))
					isSuccess := s == "done" || s == "ok" || s == "succeeded" || s == "completed"
					needSummary := e.childCount > 0 || len(e.bridgeWriteOps) == 0 || !isSuccess ||
						len(e.planItems) > 0 || e.planDetailsTitle != ""
					if needSummary {
						curAgent.entries = append(curAgent.entries, feedEntry{
							kind:               e.kind,
							timestamp:          e.timestamp,
							finishedAt:         e.finishedAt,
							role:               e.role,
							status:             e.status,
							opKind:             "code_exec_summary",
							sourceID:           e.sourceID,
							data:               e.data,
							childCount:         e.childCount,
							bridgeSingleOpKind: e.bridgeSingleOpKind,
							bridgeSingleData:   e.bridgeSingleData,
							bridgeSingleText:   e.bridgeSingleText,
							bridgeSinglePath:   e.bridgeSinglePath,
							planItems:          e.planItems,
							planDetailsTitle:   e.planDetailsTitle,
						})
					}
				} else {
					curAgent.entries = append(curAgent.entries, e)
				}
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
		start = max(0, total-height)
	}

	// Compute scroll percent
	maxScroll := max(1, total-height)
	if total <= height {
		m.scrollPercent = 100.0
	} else {
		m.scrollPercent = float64(start) / float64(maxScroll) * 100.0
		if m.scrollPercent > 100 {
			m.scrollPercent = 100
		}
	}

	content := kit.ViewportSlice(strings.Join(lines, "\n"), height, start)
	return lipgloss.NewStyle().Width(m.width).Height(height).Render(content)
}

func (m *Model) feedLines(width int) []string {
	spinParity := m.spinFrame % 2
	if m.lineCache != nil && m.lineCacheGen == m.feedGen && m.lineCacheWidth == width && m.lineCacheSpinParity == spinParity {
		return m.lineCache
	}

	turns := m.buildTurns()
	if len(turns) == 0 {
		m.lineCache = nil
		m.lineCacheGen = m.feedGen
		m.lineCacheWidth = width
		m.lineCacheSpinParity = spinParity
		return nil
	}

	inner := max(12, width-4)
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

	m.lineCache = lines
	m.lineCacheGen = m.feedGen
	m.lineCacheWidth = width
	m.lineCacheSpinParity = spinParity
	return lines
}

// ── User block ─────────────────────────────────────────────────────────
//
//   ❯ You                                              2m ago
//   Please implement the authentication module
//   ✓ queued

func (m *Model) renderUserBlock(t conversationTurn, inner int) []string {
	age := relativeAge(t.timestamp.Format(time.RFC3339))
	label := kit.StyleAccent.Bold(true).Render("You")
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := max(1, inner-labelW-ageW)
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
		outLines := []string{"  " + styleVerbBold.Render("●") + " " + styleVerbBold.Render(kit.Fallback(t.role, "agent"))}
		for _, l := range strings.Split(msg, "\n") {
			outLines = append(outLines, "    "+l)
		}
		return outLines
	}

	// Tool operations block
	age := relativeAge(t.timestamp.Format(time.RFC3339))
	role := kit.Truncate(t.role, max(4, 14))
	label := kit.StyleDim.Render("● ") + kit.StyleAccent.Bold(true).Render(role)
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := max(1, inner-labelW-ageW)
	headerLine := label + strings.Repeat(" ", gap) + ageStr

	lines := []string{headerLine}
	for _, e := range t.entries {
		// Thinking entries render as a collapsed/expanded block.
		if e.kind == feedThinking {
			lines = append(lines, m.renderThinkingBlock(e, inner)...)
			continue
		}

		// ── Standalone plan write ─────────────────────────────────
		// Plan file writes are always rendered as dedicated plan blocks,
		// never as diffs. Skip normal verb/diff rendering entirely.
		if isActivityPlanWrite(e.opKind, e.path) {
			if len(e.planItems) > 0 {
				lines = append(lines, renderPlanChecklist(e.planItems)...)
			}
			if e.planDetailsTitle != "" {
				lines = append(lines, renderPlanDetails(e.planDetailsTitle)...)
			}
			continue
		}

		verb := kindToVerb(e.opKind, e.path, e.data)
		s := strings.ToLower(strings.TrimSpace(e.status))

		// Pick dot color based on status; running dots pulse between white and pending.
		var dot string
		switch {
		case s == "done" || s == "completed" || s == "ok" || s == "succeeded":
			dot = kit.StyleOK.Render("●")
		case s == "error" || s == "failed" || s == "canceled" || s == "cancelled":
			dot = kit.StyleErr.Render("●")
		case s == "running" || s == "pending":
			if m.spinFrame%2 == 0 {
				dot = kit.StylePending.Render("●")
			} else {
				dot = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Render("●")
			}
		default:
			dot = kit.StyleDim.Render("●")
		}

		var argPreview string
		var argItalic bool // http_fetch: show URL in italic, no brackets
		opLower := strings.ToLower(strings.TrimSpace(e.opKind))
		if opLower == "code_exec" || opLower == "code_exec_summary" {
			// For summary entries, show tool count in parens for failed/running state.
			if opLower == "code_exec_summary" && e.childCount > 0 {
				isFail := s == "error" || s == "failed" || s == "canceled" || s == "cancelled"
				isRunning := s == "running" || s == "pending"
				if isFail || isRunning {
					noun := "tools"
					if e.childCount == 1 {
						noun = "tool"
					}
					argPreview = fmt.Sprintf("(%d %s)", e.childCount, noun)
				}
			}
		} else if verb == "Updated memory" || verb == "Remembering" {
			// /memory: no path in display.
		} else if verb == "Learning" {
			argPreview = skillNameFromPath(e.path)
			if argPreview != "" {
				argPreview = kit.Truncate(argPreview, max(8, inner-len(verb)-8))
			}
		} else if opLower == "http_fetch" && e.data != nil {
			if u := strings.TrimSpace(e.data["url"]); u != "" {
				argPreview = kit.Truncate(u, max(8, inner-len(verb)-8))
				argItalic = true
			}
		} else if opLower == "obsidian" && e.data != nil {
			if cmd := strings.TrimSpace(e.data["command"]); cmd != "" {
				argPreview = kit.Truncate(cmd, max(8, inner-len(verb)-8))
			}
		} else if opLower == "soul_update" && e.data != nil {
			if reason := strings.TrimSpace(e.data["reason"]); reason != "" {
				argPreview = kit.Truncate(reason, max(8, inner-len(verb)-8))
			}
		} else if opLower == "task_create" && e.data != nil {
			if goal := strings.TrimSpace(e.data["goal"]); goal != "" {
				argPreview = goal
			}
			if assigned := strings.TrimSpace(e.data["assignedRole"]); assigned != "" {
				if argPreview == "" {
					argPreview = "-> " + assigned
				} else {
					argPreview += " -> " + assigned
				}
			}
			argPreview = kit.Truncate(argPreview, max(8, inner-len(verb)-8))
		} else if opLower == "task_review" && e.data != nil {
			if id := strings.TrimSpace(e.data["taskId"]); id != "" {
				argPreview = kit.Truncate(id, max(8, inner-len(verb)-8))
			}
		} else if e.path != "" && isPathBasedOp(e.opKind) {
			argPreview = kit.Truncate(e.path, max(8, inner-len(verb)-8))
		} else {
			argPreview = kit.Truncate(stripLeadingVerb(e.text, verb), max(8, inner-len(verb)-8))
		}

		// Primary operation line: colored dot + bold verb + arg preview
		opLine := "  " + dot + " " + styleVerbBold.Render(verb)
		if argPreview != "" {
			if argItalic {
				opLine += " " + styleArgItalic.Render(argPreview)
			} else {
				opLine += " " + argPreview
			}
		}
		// Append +N −M stat for write ops.
		if isWriteOp(e.opKind) && e.data != nil {
			if added, deleted := tui.DiffStat(e.data["patchPreview"]); added > 0 || deleted > 0 {
				opLine += "  " + kit.StyleOK.Render("+"+strconv.Itoa(added)) + " " + kit.StyleErr.Render("−"+strconv.Itoa(deleted))
			}
		}
		lines = append(lines, opLine)

		// Collect sub-items (bridge summary + status) to render with tree branches.
		var subItems []string

		if e.childCount > 0 {
			if e.childCount == 1 && e.bridgeSingleOpKind != "" {
				// Single bridge call: show Verb + Args under the parent instead of "Ran 1 tools".
				bridgeVerb := kindToVerb(e.bridgeSingleOpKind, e.bridgeSinglePath, e.bridgeSingleData)
				var bridgeArg string
				var bridgeArgItalic bool
				bridgeOpLower := strings.ToLower(strings.TrimSpace(e.bridgeSingleOpKind))
				if bridgeVerb == "Updated memory" || bridgeVerb == "Remembering" {
					// no arg
				} else if bridgeVerb == "Learning" {
					bridgeArg = skillNameFromPath(e.bridgeSinglePath)
					if bridgeArg != "" {
						bridgeArg = kit.Truncate(bridgeArg, max(8, inner-len(bridgeVerb)-8))
					}
				} else if bridgeOpLower == "http_fetch" && e.bridgeSingleData != nil {
					if u := strings.TrimSpace(e.bridgeSingleData["url"]); u != "" {
						bridgeArg = kit.Truncate(u, max(8, inner-len(bridgeVerb)-8))
						bridgeArgItalic = true
					}
				} else if bridgeOpLower == "obsidian" && e.bridgeSingleData != nil {
					if cmd := strings.TrimSpace(e.bridgeSingleData["command"]); cmd != "" {
						bridgeArg = kit.Truncate(cmd, max(8, inner-len(bridgeVerb)-8))
					}
				} else if bridgeOpLower == "soul_update" && e.bridgeSingleData != nil {
					if reason := strings.TrimSpace(e.bridgeSingleData["reason"]); reason != "" {
						bridgeArg = kit.Truncate(reason, max(8, inner-len(bridgeVerb)-8))
					}
				} else if bridgeOpLower == "task_create" && e.bridgeSingleData != nil {
					if goal := strings.TrimSpace(e.bridgeSingleData["goal"]); goal != "" {
						bridgeArg = goal
					}
					if assigned := strings.TrimSpace(e.bridgeSingleData["assignedRole"]); assigned != "" {
						if bridgeArg == "" {
							bridgeArg = "-> " + assigned
						} else {
							bridgeArg += " -> " + assigned
						}
					}
					bridgeArg = kit.Truncate(bridgeArg, max(8, inner-len(bridgeVerb)-8))
				} else if bridgeOpLower == "task_review" && e.bridgeSingleData != nil {
					if id := strings.TrimSpace(e.bridgeSingleData["taskId"]); id != "" {
						bridgeArg = kit.Truncate(id, max(8, inner-len(bridgeVerb)-8))
					}
				} else if e.bridgeSinglePath != "" && isPathBasedOp(e.bridgeSingleOpKind) {
					bridgeArg = kit.Truncate(e.bridgeSinglePath, max(8, inner-len(bridgeVerb)-8))
				} else {
					bridgeArg = kit.Truncate(stripLeadingVerb(e.bridgeSingleText, bridgeVerb), max(8, inner-len(bridgeVerb)-8))
				}
			line := styleVerbBold.Render(bridgeVerb)
			if bridgeArg != "" {
				if bridgeArgItalic {
					line += " " + styleArgItalic.Render(bridgeArg)
				} else {
					line += " " + bridgeArg
				}
			}
			subItems = append(subItems, line)
			} else {
				noun := "tools"
				if e.childCount == 1 {
					noun = "tool"
				}
				subItems = append(subItems, "Ran "+styleVerbBold.Render(fmt.Sprintf("%d", e.childCount))+" "+noun)
			}
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

		// Determine whether we have a renderable diff for this op.
		hasDiff := isWriteOp(e.opKind) && isSuccessStatus(s) && e.data != nil && strings.TrimSpace(e.data["patchPreview"]) != ""

		// Suppress the "ok" sub-item when a diff or plan block will be rendered instead.
		hasPlanBlock := len(e.planItems) > 0 || e.planDetailsTitle != ""
		if !hasDiff && !hasPlanBlock {
			subItems = append(subItems, statusText)
		}

		// Add vertical connector when there are multiple sub-items (extends branch ~25%).

		// Render sub-items with proper tree branches and extra horizontal space.
		for i, item := range subItems {
			var branch string
			if i < len(subItems)-1 {
				branch = styleVerbBold.Render("├─")
			} else {
				branch = styleVerbBold.Render("└─")
			}
			lines = append(lines, "  "+branch+" "+item)
		}

		// Render inline diff (replaces the "└─ ok" line for write ops).
		if hasDiff {
			if diffLines := renderFileDiff(e.data, m.hideDiffs, inner); len(diffLines) > 0 {
				lines = append(lines, diffLines...)
			} else {
				// Fallback: no parseable diff → show status normally.
				lines = append(lines, "  "+styleVerbBold.Render("└─")+" "+statusText)
			}
		}

		// ── Promoted plan blocks (from collapsed bridge plan writes) ──────
		// Render checklist and/or plan details title promoted from
		// plan/CHECKLIST.md and plan/HEAD.md bridge writes respectively.
		if len(e.planItems) > 0 {
			lines = append(lines, renderPlanChecklist(e.planItems)...)
		}
		if e.planDetailsTitle != "" {
			lines = append(lines, renderPlanDetails(e.planDetailsTitle)...)
		}
	}
	return lines
}

// ── Plan blocks ───────────────────────────────────────────────────────

// renderPlanChecklist renders an "Updated plan" block with tree branches
// connecting each checklist item. Completed items show a green ✓, pending
// items show a dimmed ○.
//
//	● Updated plan
//	├─ ✓ Set up project structure
//	├─ ○ Add unit tests
//	└─ ○ Deploy to staging
func renderPlanChecklist(items []string) []string {
	lines := []string{"  " + kit.StylePlan.Render("●") + " " + kit.StylePlan.Bold(true).Render("Updated plan")}
	for i, item := range items {
		isLast := i == len(items)-1
		var branch string
		if isLast {
			branch = kit.StylePlan.Render("  └─")
		} else {
			branch = kit.StylePlan.Render("  ├─")
		}
		if strings.HasPrefix(item, "[x]") {
			text := strings.TrimPrefix(item, "[x] ")
			lines = append(lines, branch+" "+kit.StyleOK.Render("✓")+" "+kit.StyleDim.Strikethrough(true).Render(text))
		} else {
			text := strings.TrimPrefix(item, "[ ] ")
			lines = append(lines, branch+" "+kit.StyleDim.Render("○")+" "+text)
		}
	}
	return lines
}

// renderPlanDetails renders an "Updated plan details" block showing the title
// extracted from plan/HEAD.md.
//
//	● Updated plan details
//	└─ Create blog post about Redis
func renderPlanDetails(title string) []string {
	header := "  " + kit.StylePlan.Render("●") + " " + kit.StylePlan.Bold(true).Render("Updated plan details")
	if strings.TrimSpace(title) == "" {
		return []string{header}
	}
	branch := kit.StylePlan.Render("  └─")
	return []string{header, branch + " " + kit.StyleDim.Render(strings.TrimSpace(title))}
}

// ── Thinking line ──────────────────────────────────────────────────────

// renderThinkingBlock renders a thinking feedEntry as collapsed or expanded.
// While live:  ▸ Thinking ◐ (spinner)        ctrl+o
// After end:  ▸ Thought for 2s              ctrl+o
// Expanded:   ▾ Thinking / ▾ Thought for 2s + tree branches.
func (m *Model) renderThinkingBlock(e feedEntry, inner int) []string {
	// Format duration label (only after thinking has ended).
	var durStr string
	if !e.live && e.thinkingDuration > 0 {
		if e.thinkingDuration < time.Second {
			durStr = fmt.Sprintf("%dms", e.thinkingDuration.Milliseconds())
		} else {
			durStr = fmt.Sprintf("%.0fs", e.thinkingDuration.Seconds())
		}
	}

	var label string
	if e.live {
		label = "Thinking"
	} else if durStr != "" {
		label = "Thought for " + durStr
	} else {
		label = "Thought"
	}

	if !m.thinkingExpanded {
		// Collapsed: ▸ Thinking ◐ (spinner) or ▸ Thought for Ns    ctrl+o
		hint := kit.StyleDim.Render("ctrl+o")
		var spinner string
		if e.live {
			spinner = " " + kit.StyleThinking.Render(m.spinner())
		}
		triangle := kit.StyleThinking.Render("▸")
		labelStr := kit.StyleThinking.Italic(true).Render(label) + spinner
		labelW := runewidth.StringWidth(stripANSI("▸ " + label + spinner))
		hintW := runewidth.StringWidth(stripANSI("ctrl+o"))
		gap := max(1, inner-2-labelW-hintW)
		line := "  " + triangle + " " + labelStr + strings.Repeat(" ", gap) + hint
		return []string{line}
	}

	// Expanded: ▾ Thought for Ns + tree branches; one chunk per summary, markdown-rendered, separated by blank lines.
	triangle := kit.StyleThinking.Render("▾")
	headerLine := "  " + triangle + " " + kit.StyleThinking.Italic(true).Render(label)
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
	mdWidth := max(20, inner-7)
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
			branch = kit.StyleThinking.Render("  └─")
		} else {
			branch = kit.StyleThinking.Render("  │ ")
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
	label := kit.StylePending.Bold(true).Render("system")
	ageStr := kit.StyleDim.Render(age)

	labelW := runewidth.StringWidth(stripANSI(label))
	ageW := runewidth.StringWidth(stripANSI(ageStr))
	gap := max(1, inner-labelW-ageW)
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
			feedbackStyled = kit.StyleOK.Render(feedback)
		case feedbackErr:
			feedbackStyled = kit.StyleErr.Render(feedback)
		default:
			feedbackStyled = kit.StylePending.Render(feedback)
		}
	}

	avail := max(12, m.width-2-runewidth.StringWidth(stripANSI(feedback))-2)
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
		diffsHint := "hide diffs"
		if m.hideDiffs {
			diffsHint = "show diffs"
		}
		hints = kit.StyleDim.Render("/help") + "  " +
			kit.StyleDim.Render("ctrl+e") + " " + kit.StyleDim.Render(diffsHint) + "  " +
			kit.StyleDim.Render("ctrl+o") + " thinking  " +
			kit.StyleDim.Render("ctrl+c") + " quit"
	}

	leftW := runewidth.StringWidth(stripANSI(scrollLabel))
	rightW := runewidth.StringWidth(stripANSI(hints))
	gap := max(1, m.width-2-leftW-rightW)

	line := scrollLabel + strings.Repeat(" ", gap) + hints
	return lipgloss.NewStyle().Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).Render(line)
}

func (m *Model) spinner() string {
	return kit.SpinnerFrames[m.spinFrame%len(kit.SpinnerFrames)]
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

// isWriteOp returns true for file-write operations that produce a diff preview.
func isWriteOp(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "fs_write", "fs_append", "fs_edit", "fs_patch":
		return true
	}
	return false
}

// isSuccessStatus returns true for terminal-success statuses.
func isSuccessStatus(s string) bool {
	switch s {
	case "done", "ok", "succeeded", "completed":
		return true
	}
	return false
}

var styleDiffSep = kit.StyleAccent.Copy().Faint(true)

// renderDiffLines renders parsed diff lines with lipgloss colors.
func renderDiffLines(patchPreview string, patchTruncated, patchRedacted bool, width int) []string {
	if patchRedacted {
		return []string{"    " + kit.StyleDim.Render("[diff redacted]")}
	}
	lines, _, _ := tui.ParseDiffLines(patchPreview)
	if len(lines) == 0 {
		return nil
	}
	maxNo := 0
	for _, l := range lines {
		if l.LineNo > maxNo {
			maxNo = l.LineNo
		}
	}
	noWidth := len(strconv.Itoa(maxNo))

	var out []string
	for _, l := range lines {
		var rendered string
		switch l.Kind {
		case tui.DiffLineInsert:
			no := fmt.Sprintf("%*d", noWidth, l.LineNo)
			rendered = kit.StyleOK.Render("+ " + no + " │ " + kit.Truncate(l.Content, width-noWidth-6))
		case tui.DiffLineDelete:
			no := fmt.Sprintf("%*d", noWidth, l.LineNo)
			rendered = kit.StyleErr.Render("- " + no + " │ " + kit.Truncate(l.Content, width-noWidth-6))
		case tui.DiffLineSep:
			rendered = styleDiffSep.Render("  ···")
		default: // context
			no := fmt.Sprintf("%*d", noWidth, l.LineNo)
			rendered = kit.StyleDim.Render("  " + no + " │ " + kit.Truncate(l.Content, width-noWidth-6))
		}
		out = append(out, "    "+rendered)
	}
	if patchTruncated {
		out = append(out, "    "+kit.StyleDim.Render("  … (truncated)"))
	}
	return out
}

// renderFileDiff renders the diff block for a write op given its data map.
// Returns nil when diffs are hidden, width is too narrow, or no patch is available.
func renderFileDiff(data map[string]string, hideDiffs bool, width int) []string {
	if hideDiffs || data == nil || width < 40 {
		return nil
	}
	patchPreview := strings.TrimSpace(data["patchPreview"])
	patchTruncated := strings.TrimSpace(data["patchTruncated"]) == "true"
	patchRedacted := strings.TrimSpace(data["patchRedacted"]) == "true"
	if patchPreview == "" && !patchRedacted {
		return nil
	}
	return renderDiffLines(patchPreview, patchTruncated, patchRedacted, width)
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


func parseIntStr(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
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
		return kit.Truncate(raw, 8)
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

func boolPtr(b bool) *bool { return &b }
func uintPtr(u uint) *uint { return &u }

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
