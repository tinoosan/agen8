package dashboardtui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

var (
	colorOK      = lipgloss.Color("#98c379")
	colorErr     = lipgloss.Color("#e06c75")
	colorPending = lipgloss.Color("#e5c07b")
	colorAccent  = lipgloss.Color("#7aa2f7")
	colorPaused  = lipgloss.Color("#bb9af7")

	styleOK      = lipgloss.NewStyle().Foreground(colorOK).Bold(true)
	styleErr     = lipgloss.NewStyle().Foreground(colorErr).Bold(true)
	stylePending = lipgloss.NewStyle().Foreground(colorPending).Bold(true)
	styleAccent  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	stylePaused  = lipgloss.NewStyle().Foreground(colorPaused).Bold(true)

	styleHeader = lipgloss.NewStyle().Bold(true)
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const (
	compactWidth = 80
	narrowWidth  = 60
	smallHeight  = 14
)

func (m *Model) isCompact() bool { return m.width < compactWidth }
func (m *Model) isNarrow() bool  { return m.width < narrowWidth }
func (m *Model) isShort() bool   { return m.height < smallHeight }

func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	if m.detailOpen && m.selectedAgent() != nil {
		return m.renderDetailView()
	}
	return m.renderListView()
}

func (m *Model) renderListView() string {
	header := m.renderHeader()
	footer := m.renderFooter()
	summary := ""
	reserved := 2 // header + footer
	if !m.isShort() {
		summary = m.renderSummaryBar()
		reserved++
	}

	bodyHeight := m.height - reserved
	if bodyHeight < 1 {
		out := header + "\n" + footer
		return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
	}

	body := m.renderAgentTable(m.width, bodyHeight)
	body = lipgloss.NewStyle().MaxHeight(bodyHeight).Render(body)

	out := header + "\n" + body + "\n" + footer
	if summary != "" {
		out = header + "\n" + summary + "\n" + body + "\n" + footer
	}
	return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
}

func (m *Model) renderDetailView() string {
	header := m.renderDetailHeader()
	footer := m.renderDetailFooter()

	bodyHeight := m.height - 2 // header + footer
	if bodyHeight < 1 {
		out := header + "\n" + footer
		return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
	}

	body := m.renderDetailBody(m.width, bodyHeight)
	body = lipgloss.NewStyle().MaxHeight(bodyHeight).Render(body)

	out := header + "\n" + body + "\n" + footer
	return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(out)
}

func (m *Model) renderHeader() string {
	status := styleOK.Render("● connected")
	if !m.connected {
		status = styleErr.Render("● disconnected")
	}

	sid := strings.TrimSpace(m.sessionID)
	if len(sid) > 12 {
		sid = sid[:12]
	}

	line := styleHeader.Render("agen8 dashboard") +
		kit.StyleDim.Render("  ·  session: ") + styleAccent.Render(fallback(sid, "-")) +
		kit.StyleDim.Render("  ·  ") + status +
		kit.StyleDim.Render("  ·  mode: ") + kit.StyleStatusValue.Render(fallback(m.sessionMode, "standalone"))

	if strings.TrimSpace(m.teamID) != "" && !m.isNarrow() {
		line += kit.StyleDim.Render("  ·  team: ") + styleAccent.Render(kit.TruncateRight(m.teamID, 12))
	}
	if m.lastErr != "" {
		line += kit.StyleDim.Render("  ·  ") + styleErr.Render("err: "+truncate(m.lastErr, 40))
	}
	if m.notice != "" {
		line += kit.StyleDim.Render("  ·  ") + stylePending.Render(truncate(m.notice, 28))
	}

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Padding(0, 1).
		Render(line)
}

func (m *Model) renderSummaryBar() string {
	pending := stylePending.Render(fmt.Sprintf("⏳ %d", m.stats.Pending))
	active := styleOK.Render(fmt.Sprintf("● %d", m.stats.Active))
	done := kit.StyleDim.Render(fmt.Sprintf("✓ %d", m.stats.Done))
	running := styleOK.Render(fmt.Sprintf("running:%d", m.stats.RunningCount))
	assigned := kit.StyleStatusValue.Render(fmt.Sprintf("assigned:%d", m.stats.Assigned))
	completed := kit.StyleStatusValue.Render(fmt.Sprintf("completed:%d", m.stats.Completed))

	line := kit.StyleDim.Render("tokens:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("%d", m.stats.TotalTokens)) +
		kit.StyleDim.Render("  ·  cost:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("$%.4f", m.stats.TotalCostUSD)) +
		kit.StyleDim.Render("  ·  ") + assigned +
		kit.StyleDim.Render("  ·  ") + completed +
		kit.StyleDim.Render("  ·  ") + pending +
		kit.StyleDim.Render("  ") + done +
		kit.StyleDim.Render("  ") + active +
		kit.StyleDim.Render("  ·  ") + running

	if m.isCompact() {
		line = kit.StyleDim.Render("tok:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("%d", m.stats.TotalTokens)) +
			kit.StyleDim.Render("  ·  a:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("%d", m.stats.Assigned)) +
			kit.StyleDim.Render("  ·  c:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("%d", m.stats.Completed)) +
			kit.StyleDim.Render("  ·  ") + pending +
			kit.StyleDim.Render("  ") + active +
			kit.StyleDim.Render("  ") + done
	}

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Padding(0, 1).
		Render(line)
}

func (m *Model) renderFooter() string {
	var hints string
	if m.isNarrow() {
		hints = kit.StyleDim.Render("j/k") + " " +
			kit.StyleDim.Render("↵") + " " +
			kit.StyleDim.Render("r") + " " +
			kit.StyleDim.Render("q")
	} else {
		hints = kit.StyleDim.Render("j/k") + " scroll  " +
			kit.StyleDim.Render("enter") + " detail  " +
			kit.StyleDim.Render("g/G") + " first/last  " +
			kit.StyleDim.Render("r") + " refresh  " +
			kit.StyleDim.Render("q") + " quit"
	}

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Padding(0, 1).
		Render(hints)
}

func (m *Model) renderAgentTable(width, height int) string {
	header := m.renderAgentTableHeader(width)
	if len(m.agents) == 0 {
		empty := kit.StyleDim.Render("No agents found for this session.")
		body := lipgloss.NewStyle().Width(width).Height(maxInt(1, height-1)).Padding(0, 1).Render(empty)
		return header + "\n" + body
	}

	rows := m.buildAgentRows(width)
	visibleRows := maxInt(1, height-1)
	start := m.sel - visibleRows/2
	if start < 0 {
		start = 0
	}
	maxStart := maxInt(0, len(rows)-visibleRows)
	if start > maxStart {
		start = maxStart
	}

	content := viewportSlice(strings.Join(rows, "\n"), visibleRows, start)
	body := lipgloss.NewStyle().
		Width(width).
		Height(visibleRows).
		Padding(0, 1).
		Render(content)

	return header + "\n" + body
}

func (m *Model) renderAgentTableHeader(width int) string {
	const markerW = 2
	inner := maxInt(12, width-2-markerW)
	if m.isNarrow() {
		line := strings.Repeat(" ", markerW) +
			padRight("ROLE", maxInt(6, inner-16)) + " " +
			padRight("STATUS", 14)
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
	}
	if m.isCompact() {
		statusW := 12
		modelW := 14
		costW := 9
		roleW := maxInt(8, inner-(statusW+modelW+costW+3))
		line := strings.Repeat(" ", markerW) +
			padRight("ROLE", roleW) + " " +
			padRight("STATUS", statusW) + " " +
			padRight("MODEL", modelW) + " " +
			padRight("COST", costW)
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
	}

	roleW := 16
	statusW := 12
	modelW := 18
	costW := 9
	workerW := 6
	asgnW := 4
	doneW := 4
	startW := 5
	runW := maxInt(8, inner-(roleW+statusW+modelW+costW+workerW+asgnW+doneW+startW+8))

	line := strings.Repeat(" ", markerW) +
		padRight("ROLE", roleW) + " " +
		padRight("STATUS", statusW) + " " +
		padRight("MODEL", modelW) + " " +
		padRight("COST", costW) + " " +
		padRight("WORKER", workerW) + " " +
		padRight("ASGN", asgnW) + " " +
		padRight("DONE", doneW) + " " +
		padRight("START", startW) + " " +
		padRight("RUN", runW)
	return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
}

func (m *Model) buildAgentRows(width int) []string {
	rows := make([]string, 0, len(m.agents))
	const markerW = 2
	inner := maxInt(12, width-2-markerW)

	for i, row := range m.agents {
		isSel := i == m.sel
		marker := "  "
		if isSel {
			marker = styleAccent.Render("› ")
		}

		role := fallback(strings.TrimSpace(row.Role), "-")
		status := fallback(strings.TrimSpace(row.Status), "idle")

		if m.isNarrow() {
			roleW := maxInt(6, inner-16)
			line := marker +
				kit.StyleStatusValue.Render(padRight(kit.TruncateRight(role, roleW), roleW)) + " " +
				renderStatusCell(status, 14, m.spinFrame)
			rows = append(rows, line)
			continue
		}

		if m.isCompact() {
			statusW := 12
			modelW := 14
			costW := 9
			roleW := maxInt(8, inner-(statusW+modelW+costW+3))
			line := marker +
				kit.StyleStatusValue.Render(padRight(kit.TruncateRight(role, roleW), roleW)) + " " +
				renderStatusCell(status, statusW, m.spinFrame) + " " +
				kit.StyleDim.Render(padRight(kit.TruncateRight(fallback(row.Model, "-"), modelW), modelW)) + " " +
				kit.StyleStatusValue.Render(padRight(fmt.Sprintf("$%.4f", row.RunTotalCostUSD), costW))
			rows = append(rows, line)
			continue
		}

		roleW := 16
		statusW := 12
		modelW := 18
		costW := 9
		workerW := 6
		asgnW := 4
		doneW := 4
		startW := 5
		runW := maxInt(8, inner-(roleW+statusW+modelW+costW+workerW+asgnW+doneW+startW+8))

		worker := ""
		if row.WorkerPresent {
			worker = "✓"
		}
		started := startedClock(row.StartedAt)
		run := shortRunID(row.RunID)

		line := marker +
			kit.StyleStatusValue.Render(padRight(kit.TruncateRight(role, roleW), roleW)) + " " +
			renderStatusCell(status, statusW, m.spinFrame) + " " +
			kit.StyleDim.Render(padRight(kit.TruncateRight(fallback(row.Model, "-"), modelW), modelW)) + " " +
			kit.StyleStatusValue.Render(padRight(fmt.Sprintf("$%.4f", row.RunTotalCostUSD), costW)) + " " +
			renderWorkerCell(row.WorkerPresent, worker, workerW) + " " +
			kit.StyleStatusValue.Render(padRight(fmt.Sprintf("%d", row.AssignedTasks), asgnW)) + " " +
			kit.StyleStatusValue.Render(padRight(fmt.Sprintf("%d", row.CompletedTasks), doneW)) + " " +
			kit.StyleDim.Render(padRight(started, startW)) + " " +
			kit.StyleDim.Render(padRight(kit.TruncateRight(run, runW), runW))
		rows = append(rows, line)
	}
	return rows
}

func renderStatusCell(status string, width, spinFrame int) string {
	plain, st := statusDecor(status, spinFrame)
	return st.Render(padRight(plain, width))
}

func renderWorkerCell(present bool, symbol string, width int) string {
	if present {
		return styleOK.Render(padRight(symbol, width))
	}
	return kit.StyleDim.Render(padRight(symbol, width))
}

func statusDecor(status string, spinFrame int) (string, lipgloss.Style) {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "running", "active":
		return "running", styleOK
	case "thinking", "working":
		return spinnerFrames[spinFrame%len(spinnerFrames)] + " " + s, stylePending
	case "pending":
		return "pending", stylePending
	case "paused":
		return "paused", stylePaused
	case "stopped", "failed", "error", "canceled":
		return s, styleErr
	case "idle":
		return "idle", kit.StyleDim
	default:
		if s == "" {
			return "idle", kit.StyleDim
		}
		return s, kit.StyleStatusValue
	}
}

func (m *Model) renderDetailHeader() string {
	agent := m.selectedAgent()
	role := "agent"
	if agent != nil {
		role = fallback(strings.TrimSpace(agent.Role), "agent")
	}
	line := styleAccent.Render("dashboard detail") +
		kit.StyleDim.Render("  ·  role: ") + kit.StyleStatusValue.Render(role)

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Padding(0, 1).
		Render(line)
}

func (m *Model) renderDetailFooter() string {
	hints := kit.StyleDim.Render("esc") + " back  " +
		kit.StyleDim.Render("j/k") + " scroll  " +
		kit.StyleDim.Render("q") + " quit"

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Padding(0, 1).
		Render(hints)
}

func (m *Model) renderDetailBody(width, height int) string {
	agent := m.selectedAgent()
	if agent == nil {
		return lipgloss.NewStyle().Width(width).Height(height).Padding(0, 1).Render(kit.StyleDim.Render("No agent selected."))
	}

	statusLabel, _ := statusDecor(agent.Status, m.spinFrame)
	worker := ""
	if agent.WorkerPresent {
		worker = "✓"
	}

	lines := []string{
		kit.StyleStatusKey.Render("Role:      ") + kit.StyleStatusValue.Render(fallback(agent.Role, "-")),
		kit.StyleStatusKey.Render("Status:    ") + renderStatusCell(agent.Status, 18, m.spinFrame),
		kit.StyleStatusKey.Render("Run:       ") + kit.StyleStatusValue.Render(fallback(agent.RunID, "-")),
		kit.StyleStatusKey.Render("Profile:   ") + kit.StyleStatusValue.Render(fallback(agent.Profile, "-")),
		kit.StyleStatusKey.Render("Model:     ") + kit.StyleStatusValue.Render(fallback(agent.Model, "-")),
		kit.StyleStatusKey.Render("RunCost:   ") + kit.StyleStatusValue.Render(fmt.Sprintf("$%.4f", agent.RunTotalCostUSD)),
		kit.StyleStatusKey.Render("RunTokens: ") + kit.StyleStatusValue.Render(fmt.Sprintf("%d", agent.RunTotalTokens)),
		kit.StyleStatusKey.Render("Assigned:  ") + kit.StyleStatusValue.Render(fmt.Sprintf("%d", agent.AssignedTasks)),
		kit.StyleStatusKey.Render("Completed: ") + kit.StyleStatusValue.Render(fmt.Sprintf("%d", agent.CompletedTasks)),
		kit.StyleStatusKey.Render("Worker:    ") + kit.StyleStatusValue.Render(worker),
		kit.StyleStatusKey.Render("Started:   ") + kit.StyleStatusValue.Render(fallback(startedClock(agent.StartedAt), "—")),
		kit.StyleStatusKey.Render("RawStatus: ") + kit.StyleStatusValue.Render(fallback(statusLabel, "-")),
		"",
		kit.StyleStatusKey.Render("Session:   ") + kit.StyleStatusValue.Render(fallback(m.sessionID, "-")),
		kit.StyleStatusKey.Render("Mode:      ") + kit.StyleStatusValue.Render(fallback(m.sessionMode, "standalone")),
		kit.StyleStatusKey.Render("Team:      ") + kit.StyleStatusValue.Render(fallback(m.teamID, "-")),
		kit.StyleStatusKey.Render("Run:       ") + kit.StyleStatusValue.Render(fallback(m.runID, "-")),
		"",
		kit.StyleStatusKey.Render("Totals:    ") +
			kit.StyleStatusValue.Render(fmt.Sprintf("tokens=%d cost=$%.4f assigned=%d completed=%d pending=%d active=%d done=%d running=%d",
				m.stats.TotalTokens, m.stats.TotalCostUSD, m.stats.Assigned, m.stats.Completed, m.stats.Pending, m.stats.Active, m.stats.Done, m.stats.RunningCount)),
	}

	content := viewportSlice(strings.Join(lines, "\n"), height, m.detailScroll)
	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Padding(0, 1).
		Render(content)
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

func startedClock(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "—"
	}
	ts, ok := parseTime(raw)
	if !ok {
		return kit.TruncateRight(raw, 5)
	}
	return ts.Format("15:04")
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

func shortRunID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "—"
	}
	if len(runID) > 12 {
		return runID[:12]
	}
	return runID
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
