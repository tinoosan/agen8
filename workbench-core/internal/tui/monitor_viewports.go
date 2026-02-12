package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

func (m *monitorModel) refreshPlanView() {
	if m.renderer == nil {
		return
	}
	prevYOffset := m.planViewport.YOffset
	w := imax(24, m.planViewport.Width-4)
	detailsBody := ""
	detailsText := strings.TrimSpace(m.planDetails)
	if detailsText == "" {
		if strings.TrimSpace(m.planDetailsErr) != "" {
			detailsBody = fmt.Sprintf("_Failed to load plan details: %s_", m.planDetailsErr)
		} else {
			detailsBody = "_No plan details have been created yet._"
		}
	} else {
		detailsBody = detailsText
	}

	currentStep := ""
	progress := ""
	checklistBody := ""
	planText := strings.TrimSpace(m.planMarkdown)
	if planText == "" {
		if strings.TrimSpace(m.planLoadErr) != "" {
			checklistBody = fmt.Sprintf("_Failed to load checklist: %s_", m.planLoadErr)
		} else {
			checklistBody = "_No checklist has been created yet._"
		}
	} else {
		highlighted, active, done, total := highlightPlanChecklist(m.planMarkdown)
		if active != "" {
			currentStep = fmt.Sprintf("_Current step: %s_\n\n", active)
		}
		if total > 0 {
			progress = fmt.Sprintf("_Progress: %d/%d complete._\n\n", done, total)
		}
		if strings.TrimSpace(m.planLoadErr) != "" {
			checklistBody = fmt.Sprintf("_Failed to load checklist: %s_\n\n%s", m.planLoadErr, highlighted)
		} else {
			checklistBody = highlighted
		}
	}

	detailsSection := "### Plan Details\n\n" + detailsBody
	checklistSection := "### Checklist\n\n" + currentStep + progress + checklistBody
	content := detailsSection + "\n\n---\n\n" + checklistSection
	if strings.TrimSpace(content) == "" {
		content = "_Plan view is preparing…_"
	}
	rendered := strings.TrimRight(m.renderer.RenderMarkdown(content, w), "\n")
	rendered = wrapViewportText(rendered, imax(10, m.planViewport.Width))
	m.planViewport.SetContent(rendered)
	if m.planFollowingTop {
		m.planViewport.GotoTop()
	} else {
		m.planViewport.SetYOffset(prevYOffset)
	}
}

func (m *monitorModel) refreshThinkingViewport() {
	if len(m.thinkingEntries) == 0 {
		m.thinkingVP.SetContent(kit.StyleDim.Render("No thoughts captured yet."))
		m.thinkingAutoScroll = true
		m.thinkingVP.GotoTop()
		return
	}
	prevYOffset := m.thinkingVP.YOffset

	// Timeline view: colored nodes with a dimmed vertical spine.
	w := imax(10, m.thinkingVP.Width)
	const timelinePrefixW = 3 // keep glyph prefixes visually aligned across fonts/styles
	renderTimelinePrefix := func(prefix string) string {
		p := prefix
		if pad := timelinePrefixW - lipgloss.Width(prefix); pad > 0 {
			p += strings.Repeat(" ", pad)
		}
		return p
	}
	contentW := imax(1, w-timelinePrefixW)

	// Styles for the timeline
	nodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a371f7")) // Purple node
	spineStyle := kit.StyleDim
	titleStyle := kit.StyleBold

	filtered := make([]thinkingEntry, 0, len(m.thinkingEntries))
	for _, e := range m.thinkingEntries {
		if strings.TrimSpace(m.focusedRunID) != "" {
			entryRunID := strings.TrimSpace(e.RunID)
			focusedRunID := strings.TrimSpace(m.focusedRunID)
			if entryRunID != focusedRunID {
				// Backward compatibility: some stored events may not have run IDs.
				if !(entryRunID == "" && strings.TrimSpace(m.focusedRunRole) != "" && strings.EqualFold(strings.TrimSpace(e.Role), strings.TrimSpace(m.focusedRunRole))) {
					continue
				}
			}
		}
		if strings.TrimSpace(e.Summary) == "" {
			continue
		}
		filtered = append(filtered, e)
	}

	out := make([]string, 0, len(filtered)*3)
	last := len(filtered) - 1

	for i, e := range filtered {
		summary := strings.TrimSpace(e.Summary)
		rolePrefix := ""
		if role := strings.TrimSpace(e.Role); role != "" {
			rolePrefix = "[" + role + "] "
		}

		// Render content with markdown
		body := rolePrefix + summary
		if m.renderer != nil {
			body = strings.TrimRight(m.renderer.RenderMarkdown(body, contentW), "\n")
		}
		body = wrapViewportText(body, contentW)
		rawLines := strings.Split(body, "\n")
		lines := make([]string, 0, len(rawLines))
		for _, line := range rawLines {
			lines = append(lines, strings.TrimRight(line, " "))
		}
		if len(lines) == 0 {
			continue
		}

		// Fixed-gutter renderer: every row gets a deterministic gutter prefix.
		for lineIdx, line := range lines {
			prefix := strings.Repeat(" ", timelinePrefixW)
			if lineIdx == 0 {
				prefix = renderTimelinePrefix(nodeStyle.Render("●"))
				out = append(out, prefix+titleStyle.Render(line))
				continue
			}
			if i != last {
				prefix = renderTimelinePrefix(spineStyle.Render("│"))
			}
			out = append(out, prefix+line)
		}

		// Spacer between entries (if not last)
		if i < last {
			out = append(out, renderTimelinePrefix(spineStyle.Render("│")))
		}
	}

	if len(out) == 0 {
		m.thinkingVP.SetContent(kit.StyleDim.Render("No thoughts captured yet."))
		m.thinkingVP.GotoTop()
		return
	}

	m.thinkingVP.SetContent(strings.Join(out, "\n"))
	if m.thinkingAutoScroll {
		m.thinkingVP.GotoBottom()
	} else {
		m.thinkingVP.SetYOffset(prevYOffset)
	}
}

func (m *monitorModel) refreshViewports() {
	if m.width == 0 || m.height == 0 {
		return
	}
	grid := m.layout()

	resizeVP := func(vp *viewport.Model, w, h int) bool {
		if vp == nil {
			return false
		}
		changed := vp.Width != w || vp.Height != h
		vp.Width = w
		vp.Height = h
		return changed
	}

	compact := m.isCompactMode()

	// Resize first (size changes can force re-wrap).
	if resizeVP(&m.agentOutputVP, imax(10, grid.AgentOutput.ContentWidth), imax(1, grid.AgentOutput.ContentHeight)) {
		m.dirtyAgentOutput = true
	}
	if resizeVP(&m.outboxVP, imax(10, grid.Outbox.ContentWidth), imax(1, grid.Outbox.ContentHeight)) {
		m.dirtyOutbox = true
	}
	if resizeVP(&m.activityDetail, imax(10, grid.ActivityDetail.ContentWidth), imax(1, grid.ActivityDetail.ContentHeight)) {
		m.dirtyActivity = true
	}
	if resizeVP(&m.planViewport, imax(10, grid.Plan.ContentWidth), imax(1, grid.Plan.ContentHeight)) {
		m.dirtyPlan = true
	}
	// Thoughts share the same spec as Plan/SidePanel in dashboard mode; in compact mode it's not visible.
	if resizeVP(&m.thinkingVP, m.planViewport.Width, m.planViewport.Height) {
		m.dirtyThinking = true
	}
	if resizeVP(&m.inboxVP, imax(10, grid.Inbox.ContentWidth), imax(1, grid.Inbox.ContentHeight)) {
		m.dirtyInbox = true
	}
	if resizeVP(&m.memoryVP, imax(10, grid.Memory.ContentWidth), imax(1, grid.Memory.ContentHeight)) {
		m.dirtyMemory = true
	}

	feedW := imax(10, grid.ActivityFeed.ContentWidth)
	feedH := imax(1, grid.ActivityFeed.ContentHeight)
	m.activityList.SetSize(feedW, feedH)

	m.input.SetWidth(imax(10, grid.Composer.ContentWidth))

	// Refresh only what is (or will be) visible, and only when dirty.
	outputVisible := !compact || m.compactTab == 0
	activityVisible := (compact && m.compactTab == 1) || (!compact && m.dashboardSideTab == 0)
	planVisible := (compact && m.compactTab == 2) || (!compact && m.dashboardSideTab == 1)
	outboxVisible := (compact && m.compactTab == 3) || (!compact && m.dashboardSideTab == 2)
	inboxVisible := !compact && m.dashboardSideTab == 2
	thinkingVisible := !compact && m.dashboardSideTab == 3
	memoryVisible := grid.Memory.Height > 0 || m.focusedPanel == panelMemory

	if outputVisible && (m.dirtyLayout || m.dirtyAgentOutput) {
		m.refreshAgentOutputViewport()
		m.dirtyAgentOutput = false
	}
	if activityVisible && (m.dirtyLayout || m.dirtyActivity) {
		m.refreshActivityList()
		m.refreshActivityDetail(false)
		m.dirtyActivity = false
	}
	if planVisible && (m.dirtyLayout || m.dirtyPlan) {
		m.refreshPlanView()
		m.dirtyPlan = false
	}
	if outboxVisible && (m.dirtyLayout || m.dirtyOutbox) {
		w := m.outboxVP.Width
		m.outboxVP.SetContent(wrapViewportText(m.outboxViewportContent(w), w))
		m.dirtyOutbox = false
	}
	if inboxVisible && (m.dirtyLayout || m.dirtyInbox) {
		w := m.inboxVP.Width
		m.inboxVP.SetContent(wrapViewportText(m.inboxViewportContent(w), w))
		m.dirtyInbox = false
	}
	if thinkingVisible && (m.dirtyLayout || m.dirtyThinking) {
		m.refreshThinkingViewport()
		m.dirtyThinking = false
	}
	if memoryVisible && (m.dirtyLayout || m.dirtyMemory) {
		m.memoryVP.SetContent(wrapViewportText(renderMemResults(m.memResults), m.memoryVP.Width))
		m.dirtyMemory = false
	}

	m.dirtyLayout = false
}

func (m *monitorModel) refreshAgentOutputViewport() {
	if m == nil {
		return
	}
	w := m.agentOutputVP.Width
	if w <= 0 {
		w = 80
	}
	h := m.agentOutputVP.Height
	if h <= 0 {
		h = 1
	}
	source := m.currentAgentOutputLines()

	m.ensureAgentOutputLayout(w)
	maxY := m.agentOutputMaxYOffset(h)
	if m.agentOutputFollow {
		m.agentOutputLogicalYOffset = maxY
	}
	if m.agentOutputLogicalYOffset < 0 {
		m.agentOutputLogicalYOffset = 0
	}
	if m.agentOutputLogicalYOffset > maxY {
		m.agentOutputLogicalYOffset = maxY
	}

	if len(source) == 0 {
		m.agentOutputWindowStartLine = 0
		m.agentOutputVP.SetContent(kit.StyleDim.Render("No output yet."))
		m.agentOutputVP.SetYOffset(0)
		return
	}

	visibleStart := m.agentOutputLogicalYOffset
	visibleEnd := visibleStart + h
	firstVisible := m.findAgentOutputItemAtLine(visibleStart)
	lastVisible := m.findAgentOutputItemAtLine(visibleEnd)
	firstVisible = max(0, firstVisible)
	lastVisible = min(len(source)-1, lastVisible)

	const bufferItems = 50
	firstRender := max(0, firstVisible-bufferItems)
	lastRender := min(len(source)-1, lastVisible+bufferItems)
	windowStartLine := 0
	if firstRender >= 0 && firstRender < len(m.agentOutputLineStarts) {
		windowStartLine = m.agentOutputLineStarts[firstRender]
	}
	m.agentOutputWindowStartLine = windowStartLine

	lines := make([]string, 0, (lastRender-firstRender+1)*2)
	for i := firstRender; i <= lastRender; i++ {
		lines = append(lines, m.renderAgentOutputLogicalLines(source[i], w)...)
	}
	m.agentOutputVP.SetContent(strings.Join(lines, "\n"))
	rel := m.agentOutputLogicalYOffset - windowStartLine
	if rel < 0 {
		rel = 0
	}
	m.agentOutputVP.SetYOffset(rel)
}

func (m *monitorModel) agentOutputAtBottom() bool {
	if m == nil {
		return true
	}
	h := m.agentOutputVP.Height
	if h <= 0 {
		h = 1
	}
	maxY := m.agentOutputMaxYOffset(h)
	return m.agentOutputLogicalYOffset >= maxY
}

func (m *monitorModel) thinkingAtBottom() bool {
	return m.thinkingVP.AtBottom()
}

func (m *monitorModel) ensureAgentOutputLayout(width int) {
	if m == nil {
		return
	}
	if width <= 0 {
		width = 80
	}
	source := m.currentAgentOutputLines()
	if m.agentOutputLayoutWidth == width && len(m.agentOutputLineStarts) == len(source) && len(m.agentOutputLineHeights) == len(source) {
		return
	}

	m.agentOutputLayoutWidth = width
	m.agentOutputLineStarts = make([]int, len(source))
	m.agentOutputLineHeights = make([]int, len(source))
	lineNo := 0
	for i, rawLine := range source {
		m.agentOutputLineStarts[i] = lineNo
		h := len(m.renderAgentOutputLogicalLines(rawLine, width))
		if h < 1 {
			h = 1
		}
		m.agentOutputLineHeights[i] = h
		lineNo += h
	}
	m.agentOutputTotalLines = lineNo
}

func (m *monitorModel) agentOutputMaxYOffset(viewportHeight int) int {
	if m == nil {
		return 0
	}
	if viewportHeight <= 0 {
		viewportHeight = 1
	}
	return max(0, m.agentOutputTotalLines-viewportHeight)
}

func (m *monitorModel) findAgentOutputItemAtLine(lineNo int) int {
	if len(m.agentOutputLineStarts) == 0 {
		return 0
	}
	if lineNo <= 0 {
		return 0
	}
	// Largest start <= lineNo
	i := sort.Search(len(m.agentOutputLineStarts), func(i int) bool {
		return m.agentOutputLineStarts[i] > lineNo
	})
	if i <= 0 {
		return 0
	}
	if i >= len(m.agentOutputLineStarts) {
		return len(m.agentOutputLineStarts) - 1
	}
	return i - 1
}

func (m *monitorModel) applyAgentOutputScroll(msg tea.KeyMsg) {
	if m == nil {
		return
	}
	h := m.agentOutputVP.Height
	if h <= 0 {
		h = 1
	}
	maxY := m.agentOutputMaxYOffset(h)
	delta := 0
	abs := false
	absVal := 0

	switch msg.Type {
	case tea.KeyUp:
		delta = -1
	case tea.KeyDown:
		delta = 1
	case tea.KeyPgUp, tea.KeyCtrlB:
		delta = -h
	case tea.KeyPgDown, tea.KeyCtrlF:
		delta = h
	case tea.KeyCtrlU:
		delta = -max(1, h/2)
	case tea.KeyCtrlD:
		delta = max(1, h/2)
	case tea.KeyHome:
		abs = true
		absVal = 0
	case tea.KeyEnd:
		abs = true
		absVal = maxY
	case tea.KeyRunes:
		switch strings.TrimSpace(msg.String()) {
		case "k", "K":
			delta = -1
		case "j", "J":
			delta = 1
		}
	}

	if abs {
		m.agentOutputLogicalYOffset = absVal
	} else {
		m.agentOutputLogicalYOffset += delta
	}
	if m.agentOutputLogicalYOffset < 0 {
		m.agentOutputLogicalYOffset = 0
	}
	if m.agentOutputLogicalYOffset > maxY {
		m.agentOutputLogicalYOffset = maxY
	}
	m.agentOutputFollow = m.agentOutputLogicalYOffset >= maxY
}

func isScrollKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd,
		tea.KeyCtrlU, tea.KeyCtrlD, tea.KeyCtrlB, tea.KeyCtrlF:
		return true
	case tea.KeyRunes:
		switch strings.TrimSpace(msg.String()) {
		case "j", "k", "J", "K":
			return true
		}
	}
	return false
}
