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
	colorPaused = lipgloss.Color("#bb9af7")
	stylePaused = lipgloss.NewStyle().Foreground(colorPaused).Bold(true)

	styleHeader = lipgloss.NewStyle().Bold(true)
)

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

	switch m.mode {
	case viewProject:
		return m.renderProjectView()
	case viewTeam:
		if m.detailOpen && m.selectedAgent() != nil {
			return m.renderDetailView()
		}
		return m.renderListView()
	}
	return ""
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
	status := kit.StyleOK.Render("● connected")
	if !m.connected {
		status = kit.StyleErr.Render("● disconnected")
	}

	sid := strings.TrimSpace(m.sessionID)
	if len(sid) > 12 {
		sid = sid[:12]
	}

	prefix := ""
	if m.selectedTeam != nil {
		prefix = kit.StyleDim.Render("← ") + kit.StyleAccent.Render(teamShortLabel(*m.selectedTeam)) + kit.StyleDim.Render("  ·  ")
	}

	line := prefix + styleHeader.Render("agen8 dashboard") +
		kit.StyleDim.Render("  ·  session: ") + kit.StyleAccent.Render(kit.Fallback(sid, "-")) +
		kit.StyleDim.Render("  ·  ") + status +
		kit.StyleDim.Render("  ·  mode: ") + kit.StyleStatusValue.Render(kit.Fallback(m.sessionMode, "team"))

	if strings.TrimSpace(m.teamID) != "" && !m.isNarrow() {
		line += kit.StyleDim.Render("  ·  team: ") + kit.StyleAccent.Render(kit.TruncateRight(m.teamID, 12))
	}
	if m.lastErr != "" {
		line += kit.StyleDim.Render("  ·  ") + kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 40))
	}
	if m.notice != "" {
		line += kit.StyleDim.Render("  ·  ") + kit.StylePending.Render(kit.Truncate(m.notice, 28))
	}

	return lipgloss.NewStyle().
		Width(m.width).
		MaxWidth(m.width).
		MaxHeight(1).
		Padding(0, 1).
		Render(line)
}

func (m *Model) renderSummaryBar() string {
	pending := kit.StylePending.Render(fmt.Sprintf("⏳ %d", m.stats.Pending))
	active := kit.StyleOK.Render(fmt.Sprintf("● %d", m.stats.Active))
	done := kit.StyleDim.Render(fmt.Sprintf("✓ %d", m.stats.Done))
	running := kit.StyleOK.Render(fmt.Sprintf("running:%d", m.stats.RunningCount))
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
	backKey := "q"
	backLabel := "quit"
	if m.selectedTeam != nil {
		backKey = "esc"
		backLabel = "back"
	}

	teamNav := ""
	if m.selectedTeam != nil && !m.isNarrow() {
		teamNav = kit.StyleDim.Render("[/]") + " prev/next team  "
	}

	var hints string
	if m.isNarrow() {
		hints = kit.StyleDim.Render("j/k") + " " +
			kit.StyleDim.Render("↵") + " " +
			kit.StyleDim.Render("r") + " " +
			kit.StyleDim.Render(backKey)
	} else {
		hints = kit.StyleDim.Render("j/k") + " scroll  " +
			kit.StyleDim.Render("enter") + " detail  " +
			teamNav +
			kit.StyleDim.Render("r") + " refresh  " +
			kit.StyleDim.Render(backKey) + " " + backLabel
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
		body := lipgloss.NewStyle().Width(width).Height(max(1, height-1)).Padding(0, 1).Render(empty)
		return header + "\n" + body
	}

	rows := m.buildAgentRows(width)
	visibleRows := max(1, height-1)
	start := m.sel - visibleRows/2
	if start < 0 {
		start = 0
	}
	maxStart := max(0, len(rows)-visibleRows)
	if start > maxStart {
		start = maxStart
	}

	content := kit.ViewportSlice(strings.Join(rows, "\n"), visibleRows, start)
	body := lipgloss.NewStyle().
		Width(width).
		Height(visibleRows).
		Padding(0, 1).
		Render(content)

	return header + "\n" + body
}

func (m *Model) renderAgentTableHeader(width int) string {
	const markerW = 2
	inner := max(12, width-2-markerW)
	if m.isNarrow() {
		line := strings.Repeat(" ", markerW) +
			padRight("ROLE", max(6, inner-16)) + " " +
			padRight("STATUS", 14)
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
	}
	if m.isCompact() {
		statusW := 12
		modelW := 14
		costW := 9
		roleW := max(8, inner-(statusW+modelW+costW+3))
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
	runW := max(8, inner-(roleW+statusW+modelW+costW+workerW+asgnW+doneW+startW+8))

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
	inner := max(12, width-2-markerW)

	for i, row := range m.agents {
		isSel := i == m.sel
		marker := "  "
		if isSel {
			marker = kit.StyleAccent.Render("› ")
		}

		role := kit.Fallback(strings.TrimSpace(row.Role), "-")
		status := kit.Fallback(strings.TrimSpace(row.Status), "idle")

		if m.isNarrow() {
			roleW := max(6, inner-16)
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
			roleW := max(8, inner-(statusW+modelW+costW+3))
			line := marker +
				kit.StyleStatusValue.Render(padRight(kit.TruncateRight(role, roleW), roleW)) + " " +
				renderStatusCell(status, statusW, m.spinFrame) + " " +
				kit.StyleDim.Render(padRight(kit.TruncateRight(kit.Fallback(row.Model, "-"), modelW), modelW)) + " " +
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
		runW := max(8, inner-(roleW+statusW+modelW+costW+workerW+asgnW+doneW+startW+8))

		worker := ""
		if row.WorkerPresent {
			worker = "✓"
		}
		started := startedClock(row.StartedAt)
		run := shortRunID(row.RunID)

		line := marker +
			kit.StyleStatusValue.Render(padRight(kit.TruncateRight(role, roleW), roleW)) + " " +
			renderStatusCell(status, statusW, m.spinFrame) + " " +
			kit.StyleDim.Render(padRight(kit.TruncateRight(kit.Fallback(row.Model, "-"), modelW), modelW)) + " " +
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
		return kit.StyleOK.Render(padRight(symbol, width))
	}
	return kit.StyleDim.Render(padRight(symbol, width))
}

func statusDecor(status string, spinFrame int) (string, lipgloss.Style) {
	s := strings.ToLower(strings.TrimSpace(status))
	switch s {
	case "running", "active":
		return "running", kit.StyleOK
	case "thinking", "working":
		return kit.SpinnerFrames[spinFrame%len(kit.SpinnerFrames)] + " " + s, kit.StylePending
	case "pending":
		return "pending", kit.StylePending
	case "paused":
		return "paused", stylePaused
	case "stopped", "failed", "error", "canceled":
		return s, kit.StyleErr
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
		role = kit.Fallback(strings.TrimSpace(agent.Role), "agent")
	}
	line := kit.StyleAccent.Render("dashboard detail") +
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
		kit.StyleStatusKey.Render("Role:      ") + kit.StyleStatusValue.Render(kit.Fallback(agent.Role, "-")),
		kit.StyleStatusKey.Render("Status:    ") + renderStatusCell(agent.Status, 18, m.spinFrame),
		kit.StyleStatusKey.Render("Run:       ") + kit.StyleStatusValue.Render(kit.Fallback(agent.RunID, "-")),
		kit.StyleStatusKey.Render("Profile:   ") + kit.StyleStatusValue.Render(kit.Fallback(agent.Profile, "-")),
		kit.StyleStatusKey.Render("Model:     ") + kit.StyleStatusValue.Render(kit.Fallback(agent.Model, "-")),
		kit.StyleStatusKey.Render("Worker:    ") + kit.StyleStatusValue.Render(worker),
		kit.StyleStatusKey.Render("Started:   ") + kit.StyleStatusValue.Render(kit.Fallback(startedClock(agent.StartedAt), "—")),
		kit.StyleStatusKey.Render("RawStatus: ") + kit.StyleStatusValue.Render(kit.Fallback(statusLabel, "-")),
		"",
		kit.StyleDim.Render("── Agent Metrics ──"),
		kit.StyleStatusKey.Render("Cost:      ") + kit.StyleStatusValue.Render(fmt.Sprintf("$%.4f", agent.RunTotalCostUSD)),
		kit.StyleStatusKey.Render("Tokens:    ") + kit.StyleStatusValue.Render(fmt.Sprintf("%d", agent.RunTotalTokens)),
		kit.StyleStatusKey.Render("Assigned:  ") + kit.StyleStatusValue.Render(fmt.Sprintf("%d", agent.AssignedTasks)),
		kit.StyleStatusKey.Render("Completed: ") + kit.StyleStatusValue.Render(fmt.Sprintf("%d", agent.CompletedTasks)),
		"",
		kit.StyleDim.Render("── Context ──"),
		kit.StyleStatusKey.Render("Session:   ") + kit.StyleStatusValue.Render(kit.Fallback(m.sessionID, "-")),
		kit.StyleStatusKey.Render("Team:      ") + kit.StyleStatusValue.Render(kit.Fallback(m.teamID, "-")),
	}

	content := kit.ViewportSlice(strings.Join(lines, "\n"), height, m.detailScroll)
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

	line := styleHeader.Render("agen8 project") +
		kit.StyleDim.Render("  ·  ") + kit.StyleAccent.Render(pid) +
		kit.StyleDim.Render("  ·  ") + kit.StyleStatusValue.Render(teamCount) +
		kit.StyleDim.Render("  ·  ") + status

	if m.lastErr != "" {
		line += kit.StyleDim.Render("  ·  ") + kit.StyleErr.Render("err: "+kit.Truncate(m.lastErr, 40))
	}
	if m.notice != "" {
		line += kit.StyleDim.Render("  ·  ") + kit.StylePending.Render(kit.Truncate(m.notice, 28))
	}

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(line)
}

func (m *Model) renderProjectSummaryBar() string {
	var totalTokens int
	var totalCostUSD float64
	var pending, active, done, runningAgents int
	for _, t := range m.teams {
		totalTokens += t.TotalTokens
		totalCostUSD += t.TotalCostUSD
		pending += t.Pending
		active += t.Active
		done += t.Done
		runningAgents += t.RunningAgents
	}

	pendingLabel := kit.StylePending.Render(fmt.Sprintf("⏳ %d", pending))
	activeLabel := kit.StyleOK.Render(fmt.Sprintf("● %d", active))
	doneLabel := kit.StyleDim.Render(fmt.Sprintf("✓ %d", done))
	runningLabel := kit.StyleOK.Render(fmt.Sprintf("agents:%d", runningAgents))

	var line string
	if m.isCompact() {
		line = kit.StyleDim.Render("tok:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("%d", totalTokens)) +
			kit.StyleDim.Render("  ·  cost:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("$%.2f", totalCostUSD)) +
			kit.StyleDim.Render("  ·  ") + pendingLabel +
			kit.StyleDim.Render("  ") + activeLabel +
			kit.StyleDim.Render("  ") + doneLabel
	} else {
		line = kit.StyleDim.Render("tokens:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("%d", totalTokens)) +
			kit.StyleDim.Render("  ·  cost:") + " " + kit.StyleStatusValue.Render(fmt.Sprintf("$%.2f", totalCostUSD)) +
			kit.StyleDim.Render("  ·  ") + pendingLabel +
			kit.StyleDim.Render("  ") + activeLabel +
			kit.StyleDim.Render("  ") + doneLabel +
			kit.StyleDim.Render("  ·  ") + runningLabel
	}

	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(line)
}

func (m *Model) renderProjectFooter() string {
	var hints string
	if m.isNarrow() {
		hints = kit.StyleDim.Render("j/k") + " " +
			kit.StyleDim.Render("↵") + " " +
			kit.StyleDim.Render("r") + " " +
			kit.StyleDim.Render("q")
	} else {
		hints = kit.StyleDim.Render("j/k") + " scroll  " +
			kit.StyleDim.Render("enter") + " open team  " +
			kit.StyleDim.Render("g/G") + " first/last  " +
			kit.StyleDim.Render("r") + " refresh  " +
			kit.StyleDim.Render("q") + " quit"
	}
	return lipgloss.NewStyle().
		Width(m.width).MaxWidth(m.width).MaxHeight(1).Padding(0, 1).
		Render(hints)
}

func (m *Model) renderTeamTable(width, height int) string {
	header := m.renderTeamTableHeader(width)
	if len(m.teams) == 0 {
		empty := kit.StyleDim.Render("No teams found for this project.")
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
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
	}
	if m.isCompact() {
		statusW := 10
		profileW := 14
		tasksW := 12
		teamW := max(8, inner-(statusW+profileW+tasksW+3))
		line := strings.Repeat(" ", markerW) +
			padRight("TEAM", teamW) + " " +
			padRight("STATUS", statusW) + " " +
			padRight("PROFILE", profileW) + " " +
			padRight("TASKS", tasksW)
		return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
	}

	// Full width columns.
	teamW := 14
	statusW := 10
	profileW := 14
	coordW := 12
	agentsW := 8
	pendW := 5
	actW := 5
	doneW := 5
	costW := 10
	ageW := max(6, inner-(teamW+statusW+profileW+coordW+agentsW+pendW+actW+doneW+costW+9))

	line := strings.Repeat(" ", markerW) +
		padRight("TEAM", teamW) + " " +
		padRight("STATUS", statusW) + " " +
		padRight("PROFILE", profileW) + " " +
		padRight("COORD", coordW) + " " +
		padRight("AGENTS", agentsW) + " " +
		padRight("PEND", pendW) + " " +
		padRight("ACT", actW) + " " +
		padRight("DONE", doneW) + " " +
		padRight("COST", costW) + " " +
		padRight("AGE", ageW)
	return lipgloss.NewStyle().Padding(0, 1).Width(width).Render(kit.StyleDim.Render(line))
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
				kit.StyleStatusValue.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
				renderTeamStatusCell(row, 14, m.spinFrame)
			rows = append(rows, line)
			continue
		}

		if m.isCompact() {
			statusW := 10
			profileW := 14
			tasksW := 12
			teamW := max(8, inner-(statusW+profileW+tasksW+3))
			tasksSummary := fmt.Sprintf("%d/%d/%d", row.Pending, row.Active, row.Done)
			line := marker +
				kit.StyleStatusValue.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
				renderTeamStatusCell(row, statusW, m.spinFrame) + " " +
				kit.StyleDim.Render(padRight(kit.TruncateRight(kit.Fallback(row.ProfileID, "-"), profileW), profileW)) + " " +
				kit.StyleStatusValue.Render(padRight(tasksSummary, tasksW))
			rows = append(rows, line)
			continue
		}

		// Full width.
		teamW := 14
		statusW := 10
		profileW := 14
		coordW := 12
		agentsW := 8
		pendW := 5
		actW := 5
		doneW := 5
		costW := 10
		ageW := max(6, inner-(teamW+statusW+profileW+coordW+agentsW+pendW+actW+doneW+costW+9))

		agentStr := fmt.Sprintf("%d/%d", row.RunningAgents, row.TotalAgents)
		coordLabel := kit.Fallback(kit.TruncateRight(row.CoordinatorRole, coordW), "-")

		line := marker +
			kit.StyleStatusValue.Render(padRight(kit.TruncateRight(teamLabel, teamW), teamW)) + " " +
			renderTeamStatusCell(row, statusW, m.spinFrame) + " " +
			kit.StyleDim.Render(padRight(kit.TruncateRight(kit.Fallback(row.ProfileID, "-"), profileW), profileW)) + " " +
			kit.StyleStatusValue.Render(padRight(coordLabel, coordW)) + " " +
			kit.StyleOK.Render(padRight(agentStr, agentsW)) + " " +
			kit.StylePending.Render(padRight(fmt.Sprintf("%d", row.Pending), pendW)) + " " +
			kit.StyleOK.Render(padRight(fmt.Sprintf("%d", row.Active), actW)) + " " +
			kit.StyleDim.Render(padRight(fmt.Sprintf("%d", row.Done), doneW)) + " " +
			kit.StyleStatusValue.Render(padRight(fmt.Sprintf("$%.2f", row.TotalCostUSD), costW)) + " " +
			kit.StyleDim.Render(padRight(relativeAge(row.UpdatedAt), ageW))
		rows = append(rows, line)
	}
	return rows
}

// teamShortLabel returns a compact display name for a team.
// If profileID is set: "profile·hash" (e.g. "startup·a3f2").
// Otherwise: short hash from teamID (e.g. "a3f2e1b4").
func teamShortLabel(row teamRow) string {
	id := strings.TrimSpace(row.TeamID)
	profile := strings.TrimSpace(row.ProfileID)

	// Extract short hash: strip "team-" prefix, take first 8 chars.
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
		return kit.StyleDim.Render(padRight("registered", width))
	default:
		return kit.StyleDim.Render(padRight(kit.Fallback(s, "—"), width))
	}
}
