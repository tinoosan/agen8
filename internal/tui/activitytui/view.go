package activitytui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	tuishared "github.com/tinoosan/agen8/internal/tui"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/types"
)

var (
	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c8d3f5"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#5a6375"))
	styleMeta   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9da3b4"))

	styleSelRow = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c8d3f5"))
	styleUnselRow = lipgloss.NewStyle().Foreground(lipgloss.Color("#9da3b4"))
	mdRenderer    = tuishared.NewContentRenderer()
)

const (
	compactWidth        = 60
	compactWidthProject = 80
	smallHeight         = 14
)

func (m *Model) isNarrow() bool  { return m.width < compactWidth }
func (m *Model) isCompact() bool { return m.width < compactWidthProject }
func (m *Model) isShort() bool   { return m.height < smallHeight }

// ── View ───────────────────────────────────────────────────────────────

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

	var header, footer, body string

	if m.detailOpen && m.sel >= 0 && m.sel < len(m.activities) {
		header = m.renderDetailHeader()
		footer = m.renderDetailFooter()
		bodyHeight := m.height - 2
		if bodyHeight < 1 {
			bodyHeight = 1
		}
		body = m.renderDetailView(m.width, bodyHeight)
	} else {
		header = m.renderHeader()
		footer = m.renderFooter()
		bodyHeight := m.height - 2
		if bodyHeight < 1 {
			bodyHeight = 1
		}
		body = m.renderBody(bodyHeight)
	}

	out := header + "\n" + body + "\n" + footer
	return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
}

// ── Header ─────────────────────────────────────────────────────────────

func (m *Model) renderHeader() string {
	status := kit.StyleOK.Render("● connected")
	if !m.connected {
		status = kit.StyleErr.Render("● disconnected")
	}

	if m.isNarrow() {
		left := styleHeader.Render("activity") + styleDim.Render(" · ") + status
		if m.lastErr != "" {
			left += styleDim.Render(" · ") + kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 20))
		}
		if m.notice != "" {
			left += styleDim.Render(" · ") + kit.StylePending.Render(kit.Truncate(m.notice, 18))
		}
		return lipgloss.NewStyle().
			Width(m.width).MaxWidth(m.width).MaxHeight(1).
			Foreground(lipgloss.Color("#eaeaea")).
			Padding(0, 1).
			Render(left)
	}

	count := fmt.Sprintf("%d ops", len(m.activities))
	if m.totalCount > len(m.activities) {
		count = fmt.Sprintf("%d/%d ops", len(m.activities), m.totalCount)
	}

	sid := strings.TrimSpace(m.sessionID)
	if len(sid) > 12 {
		sid = sid[:12]
	}
	if sid == "" {
		sid = "-"
	}
	prefix := ""
	if m.selectedTeam != nil {
		prefix = styleDim.Render("← ") + kit.StyleAccent.Render(teamShortLabel(*m.selectedTeam)) + styleDim.Render("  ·  ")
	}

	left := prefix + styleHeader.Render("agen8 activity") +
		styleDim.Render("  ·  session: ") + kit.StyleAccent.Render(sid) +
		styleDim.Render("  ·  ") + status +
		styleDim.Render("  ·  ") + styleMeta.Render(count)

	if m.lastErr != "" {
		left += styleDim.Render("  ·  ") + kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 40))
	}
	if m.notice != "" {
		left += styleDim.Render("  ·  ") + kit.StylePending.Render(kit.Truncate(m.notice, 28))
	}

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).
		Foreground(lipgloss.Color("#eaeaea")).
		Padding(0, 1).
		Render(left)
}

// ── Footer ─────────────────────────────────────────────────────────────

func (m *Model) renderFooter() string {
	backKey := "q"
	backLabel := "quit"
	if m.selectedTeam != nil {
		backKey = "esc"
		backLabel = "back"
	}

	teamNav := ""
	if m.selectedTeam != nil && !m.isNarrow() {
		teamNav = styleDim.Render("[/]") + " prev/next team  "
	}

	if m.isNarrow() {
		hints := styleDim.Render("j/k") + " " +
			styleDim.Render("↵") + " " +
			styleDim.Render("esc") + " " +
			styleDim.Render("g/G") + " " +
			styleDim.Render("t") + " " +
			styleDim.Render("r") + " " +
			styleDim.Render(backKey)
		return lipgloss.NewStyle().
			Width(m.width).MaxWidth(m.width).MaxHeight(1).
			Padding(0, 1).
			Render(hints)
	}

	hints := styleDim.Render("j/k") + " scroll  " +
		styleDim.Render("enter") + " detail  " +
		styleDim.Render("esc") + " close  " +
		styleDim.Render("g/G") + " top/bottom  " +
		styleDim.Render("t") + " timestamps  " +
		teamNav +
		styleDim.Render("r") + " refresh  " +
		styleDim.Render(backKey) + " " + backLabel

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).
		Padding(0, 1).
		Render(hints)
}

// ── Body ───────────────────────────────────────────────────────────────

func (m *Model) renderBody(height int) string {
	if len(m.activities) == 0 {
		empty := styleDim.Render("  No activities yet. Waiting for agent ops…")
		if m.lastErr != "" {
			empty = kit.StyleErr.Render("  " + m.lastErr)
		}
		return lipgloss.NewStyle().Width(m.width).Height(height).Render(empty)
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
		content = kit.ViewportSlice(content, height, anchor)
	} else {
		startLine := m.sel
		if startLine+height > len(lines) {
			startLine = max(0, len(lines)-height)
		}
		content = kit.ViewportSlice(content, height, startLine)
	}
	return lipgloss.NewStyle().Width(width).Height(height).Render(content)
}

func (m *Model) buildListLines(width int) []string {
	if len(m.activities) == 0 {
		return []string{styleDim.Render("  No activities.")}
	}

	lines := make([]string, 0, len(m.activities))
	for i, act := range m.activities {
		isSel := i == m.sel

		// Build: marker status timestamp [role] emoji tool_name activity
		marker := "  "
		if isSel {
			marker = kit.StyleAccent.Render("› ")
		}

		statusIcon := m.statusIcon(act)

		// Timestamp (toggled with 't')
		ts := ""
		if m.showTimestamps && !act.StartedAt.IsZero() {
			ts = styleDim.Render(act.StartedAt.Format("15:04:05")) + " "
		}

		// Role (from Data map if present)
		role := ""
		if r := strings.TrimSpace(act.Data["role"]); r != "" {
			role = styleDim.Render("[") + styleMeta.Render(r) + styleDim.Render("] ")
		}

		// Emoji + tool name
		icon := kit.KindIcon(act.Kind)
		kind := strings.TrimSpace(act.Kind)
		if kind == "" {
			kind = "op"
		}

		// Activity title
		title := actTitle(act)
		// Calculate remaining width for title truncation
		fixedW := 2 + 2 + 9 + len(kind) + 4 // marker + status + timestamp + kind + spacing
		availW := max(10, width-fixedW)
		title = kit.Truncate(title, availW)

		var line string
		if isSel {
			line = marker + statusIcon + " " + ts + role + icon + " " + kit.StyleAccent.Render(kind) + " " + styleSelRow.Render(title)
		} else {
			line = marker + statusIcon + " " + ts + role + icon + " " + styleMeta.Render(kind) + " " + styleUnselRow.Render(title)
		}

		lines = append(lines, line)
	}
	return lines
}

func (m *Model) statusIcon(act types.Activity) string {
	switch act.Status {
	case types.ActivityOK:
		return kit.StyleOK.Render("✓")
	case types.ActivityError:
		return kit.StyleErr.Render("✗")
	default:
		frame := kit.SpinnerFrames[m.spinFrame%len(kit.SpinnerFrames)]
		return kit.StylePending.Render(frame)
	}
}

// ── Detail view (full screen) ──────────────────────────────────────────

func (m *Model) renderDetailHeader() string {
	act := m.activities[m.sel]
	icon := kit.KindIcon(act.Kind)
	kind := strings.TrimSpace(act.Kind)
	if kind == "" {
		kind = "op"
	}
	title := actTitle(act)
	title = kit.Truncate(title, max(10, m.width-len(kind)-10))

	left := kit.StyleAccent.Render(icon+" "+kind) + styleDim.Render(" · ") + styleHeader.Render(title)

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).
		Foreground(lipgloss.Color("#eaeaea")).
		Padding(0, 1).
		Render(left)
}

func (m *Model) renderDetailFooter() string {
	hints := styleDim.Render("esc") + " back  " +
		styleDim.Render("j/k") + " scroll  " +
		styleDim.Render("pgup/pgdn") + " page  " +
		styleDim.Render("q") + " quit"

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).
		Padding(0, 1).
		Render(hints)
}

func (m *Model) renderDetailView(width, height int) string {
	act := m.activities[m.sel]
	innerW := max(10, width-2)

	md := renderActivityDetailMD(act)
	rendered := renderMarkdown(md, innerW)

	// Apply viewport scroll
	rendered = kit.ViewportSlice(rendered, height, m.detailScroll)

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(rendered)
}

func renderActivityDetailMD(a types.Activity) string {
	var b strings.Builder

	// ── Metadata ──
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

	// ── Error ──
	if strings.TrimSpace(a.Error) != "" {
		b.WriteString("\n---\n\n**Error**\n\n```\n")
		b.WriteString(a.Error)
		b.WriteString("\n```\n")
	}

	// ── Arguments (sorted, full values) ──
	if len(a.Data) > 0 {
		// Collect and sort keys for stable ordering
		keys := make([]string, 0, len(a.Data))
		for k := range a.Data {
			switch k {
			case "op", "ok", "err", "opId":
				continue
			}
			if strings.TrimSpace(a.Data[k]) == "" {
				continue
			}
			keys = append(keys, k)
		}
		sortStrings(keys)

		if len(keys) > 0 {
			b.WriteString("\n---\n\n**Arguments**\n\n")
			for _, k := range keys {
				v := strings.TrimSpace(a.Data[k])
				// For long values, show them in a code block
				if len(v) > 120 || strings.Contains(v, "\n") {
					b.WriteString("**")
					b.WriteString(k)
					b.WriteString("**\n\n```\n")
					if len(v) > 2000 {
						v = v[:1997] + "…"
					}
					b.WriteString(v)
					b.WriteString("\n```\n\n")
				} else {
					b.WriteString("- `")
					b.WriteString(k)
					b.WriteString("`: ")
					b.WriteString(v)
					b.WriteString("\n")
				}
			}
		}
	}

	// ── Output ──
	if strings.TrimSpace(a.OutputPreview) != "" {
		preview := a.OutputPreview
		if len(preview) > 4000 {
			preview = preview[:3997] + "…"
		}
		b.WriteString("\n---\n\n**Output**\n\n```\n")
		b.WriteString(preview)
		b.WriteString("\n```\n")
	}

	// ── Written content (fs_write / fs_append) ──
	if strings.TrimSpace(a.TextPreview) != "" && !a.TextRedacted {
		b.WriteString("\n---\n\n**Written Content**")
		if a.TextTruncated {
			b.WriteString(" _(truncated)_")
		}
		b.WriteString("\n\n```\n")
		preview := a.TextPreview
		if len(preview) > 4000 {
			preview = preview[:3997] + "…"
		}
		b.WriteString(preview)
		b.WriteString("\n```\n")
	}

	return b.String()
}

func sortStrings(s []string) {
	sort.Strings(s)
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

func renderMarkdown(md string, width int) string {
	md = strings.TrimSpace(md)
	if md == "" {
		return ""
	}
	return strings.TrimRight(mdRenderer.RenderMarkdown(md, width), "\n")
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

	line := styleHeader.Render("agen8 activity") +
		styleDim.Render("  ·  ") + kit.StyleAccent.Render(pid) +
		styleDim.Render("  ·  ") + styleMeta.Render(teamCount) +
		styleDim.Render("  ·  ") + status

	if m.lastErr != "" {
		line += styleDim.Render("  ·  ") + kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 40))
	}
	if m.notice != "" {
		line += styleDim.Render("  ·  ") + kit.StylePending.Render(kit.Truncate(m.notice, 28))
	}

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(line)
}

func (m *Model) renderProjectSummaryBar() string {
	var runningAgents, totalAgents int
	for _, t := range m.teams {
		runningAgents += t.RunningAgents
		totalAgents += t.TotalAgents
	}

	runningLabel := kit.StyleOK.Render(fmt.Sprintf("● %d running", runningAgents))
	totalLabel := styleDim.Render(fmt.Sprintf("/ %d agents", totalAgents))

	line := runningLabel + " " + totalLabel

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(line)
}

func (m *Model) renderProjectFooter() string {
	var hints string
	if m.isNarrow() {
		hints = styleDim.Render("j/k") + " " +
			styleDim.Render("↵") + " " +
			styleDim.Render("r") + " " +
			styleDim.Render("q")
	} else {
		hints = styleDim.Render("j/k") + " scroll  " +
			styleDim.Render("enter") + " open team  " +
			styleDim.Render("g/G") + " first/last  " +
			styleDim.Render("r") + " refresh  " +
			styleDim.Render("q") + " quit"
	}
	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(hints)
}

func (m *Model) renderTeamTable(width, height int) string {
	header := m.renderTeamTableHeader(width)
	if len(m.teams) == 0 {
		empty := styleDim.Render("No teams found for this project.")
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
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(styleDim.Render(line))
	}
	if m.isCompact() {
		statusW := 10
		agentsW := 8
		teamW := max(8, inner-(statusW+agentsW+2))
		line := strings.Repeat(" ", markerW) +
			padRight("TEAM", teamW) + " " +
			padRight("STATUS", statusW) + " " +
			padRight("AGENTS", agentsW)
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(styleDim.Render(line))
	}

	// Full width: TEAM, STATUS, AGENTS, PROFILE, AGE.
	teamW := 16
	statusW := 10
	agentsW := 8
	profileW := 16
	ageW := max(6, inner-(teamW+statusW+agentsW+profileW+4))

	line := strings.Repeat(" ", markerW) +
		padRight("TEAM", teamW) + " " +
		padRight("STATUS", statusW) + " " +
		padRight("AGENTS", agentsW) + " " +
		padRight("PROFILE", profileW) + " " +
		padRight("AGE", ageW)
	return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(styleDim.Render(line))
}

func (m *Model) buildTeamRows(width int) []string {
	rows := make([]string, 0, len(m.teams))
	const markerW = 2
	inner := max(12, width-2-markerW)

	for i, row := range m.teams {
		isSel := i == m.teamSel
		marker := "  "
		if isSel {
			marker = kit.StyleAccent.Render("› ")
		}

		teamLabel := teamShortLabel(row)

		if m.isNarrow() {
			teamW := max(6, inner-16)
			line := marker +
				styleMeta.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
				renderTeamStatusCell(row, 14, m.spinFrame)
			rows = append(rows, line)
			continue
		}

		agentStr := fmt.Sprintf("%d/%d", row.RunningAgents, row.TotalAgents)

		if m.isCompact() {
			statusW := 10
			agentsW := 8
			teamW := max(8, inner-(statusW+agentsW+2))
			line := marker +
				styleMeta.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
				renderTeamStatusCell(row, statusW, m.spinFrame) + " " +
				kit.StyleOK.Render(padRight(agentStr, agentsW))
			rows = append(rows, line)
			continue
		}

		// Full width: TEAM, STATUS, AGENTS, PROFILE, AGE.
		teamW := 16
		statusW := 10
		agentsW := 8
		profileW := 16
		ageW := max(6, inner-(teamW+statusW+agentsW+profileW+4))

		line := marker +
			styleMeta.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
			renderTeamStatusCell(row, statusW, m.spinFrame) + " " +
			kit.StyleOK.Render(padRight(agentStr, agentsW)) + " " +
			styleDim.Render(padRight(kit.TruncateRight(kit.Fallback(row.ProfileID, "-"), profileW), profileW)) + " " +
			styleDim.Render(padRight(relativeAge(row.UpdatedAt), ageW))
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
		return styleDim.Render(padRight("registered", width))
	default:
		return styleDim.Render(padRight(kit.Fallback(s, "—"), width))
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
