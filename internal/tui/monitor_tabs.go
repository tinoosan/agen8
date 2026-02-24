package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/app"
	"github.com/tinoosan/agen8/internal/tui/kit"
	layoutmgr "github.com/tinoosan/agen8/internal/tui/layout"
	"github.com/tinoosan/agen8/pkg/types"
)

// subagentListItem is a list item for the Subagents tab (run to switch to, or "Back to parent").
type subagentListItem struct {
	RunID string
	Label string
}

func (s subagentListItem) FilterValue() string { return strings.TrimSpace(s.RunID + " " + s.Label) }
func (s subagentListItem) Title() string       { return strings.TrimSpace(s.Label) }

func renderSubagentLine(item list.Item, maxWidth int) string {
	it, ok := item.(subagentListItem)
	if !ok {
		return kit.TruncateRight(strings.TrimSpace(item.FilterValue()), maxWidth)
	}
	return kit.TruncateRight(strings.TrimSpace(it.Label), maxWidth)
}

func newSubagentsList() list.Model {
	l := list.New(nil, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), renderSubagentLine), 0, 0)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	return l
}

type compactTabDef struct {
	Name   string
	Panel  panelID
	Render func(m *monitorModel, grid layoutmgr.GridLayout) string
}

type dashboardSideTabDef struct {
	Name       string
	Panels     []panelID
	FocusPanel panelID
	FocusCycle []panelID
	Render     func(m *monitorModel, grid layoutmgr.GridLayout) string
}

var compactTabs = []compactTabDef{
	{Name: "Output", Panel: panelOutput, Render: renderCompactOutputTab},
	{Name: "Activity", Panel: panelActivity, Render: renderCompactActivityTab},
	{Name: "Plan", Panel: panelPlan, Render: renderCompactPlanTab},
	{Name: "Outbox", Panel: panelOutbox, Render: renderCompactOutboxTab},
}

var dashboardSideTabs = []dashboardSideTabDef{
	{
		Name:       "Activity",
		Panels:     []panelID{panelActivity, panelActivityDetail},
		FocusPanel: panelActivity,
		FocusCycle: []panelID{panelComposer, panelOutput, panelActivity, panelActivityDetail},
		Render:     renderDashboardActivityTab,
	},
	{
		Name:       "Plan",
		Panels:     []panelID{panelPlan},
		FocusPanel: panelPlan,
		FocusCycle: []panelID{panelComposer, panelOutput, panelPlan},
		Render:     renderDashboardPlanTab,
	},
	{
		Name:       "Tasks",
		Panels:     []panelID{panelCurrentTask, panelInbox, panelOutbox},
		FocusPanel: panelCurrentTask,
		FocusCycle: []panelID{panelComposer, panelOutput, panelCurrentTask, panelInbox, panelOutbox},
		Render:     renderDashboardTasksTab,
	},
	{
		Name:       "Thoughts",
		Panels:     []panelID{panelThinking},
		FocusPanel: panelThinking,
		FocusCycle: []panelID{panelComposer, panelOutput, panelThinking},
		Render:     renderDashboardThoughtsTab,
	},
	{
		Name:       "Subagents",
		Panels:     []panelID{panelSubagents},
		FocusPanel: panelSubagents,
		FocusCycle: []panelID{panelComposer, panelOutput, panelSubagents},
		Render:     renderDashboardSubagentsTab,
	},
}

func (m *monitorModel) activeDashboardSideTabs() []dashboardSideTabDef {
	if m == nil {
		return dashboardSideTabs
	}
	// Show Subagents tab when allow_subagents is true for the current run's role.
	if strings.TrimSpace(m.teamID) != "" {
		runID := strings.TrimSpace(m.runID)
		if strings.TrimSpace(m.focusedRunID) != "" {
			runID = strings.TrimSpace(m.focusedRunID)
		}
		roleName := ""
		if m.teamRoleByRunID != nil && runID != "" {
			roleName = strings.TrimSpace(m.teamRoleByRunID[runID])
		}
		if !app.ResolveRoleAllowSubagents(m.cfg, strings.TrimSpace(m.profile), roleName) {
			out := make([]dashboardSideTabDef, 0, len(dashboardSideTabs))
			for _, tab := range dashboardSideTabs {
				if strings.EqualFold(strings.TrimSpace(tab.Name), "Subagents") {
					continue
				}
				out = append(out, tab)
			}
			return out
		}
	}
	return dashboardSideTabs
}

// renderCompact builds the view for compact mode: header + tab bar + main content + composer.
func (m *monitorModel) renderCompact(grid layoutmgr.GridLayout, headerLine string) string {
	tabBar := m.renderCompactTabBar()
	content := m.renderCompactTabContent(grid)
	sections := []string{headerLine}
	if m.rpcHealthKnown && !m.rpcReachable {
		sections = append(sections, kit.StyleDim.Render("Daemon disconnected. Start `agen8 daemon` and run /reconnect."))
	}
	sections = append(sections, tabBar, content, m.renderComposer(grid.Composer))
	final := lipgloss.JoinVertical(lipgloss.Left, sections...)
	effectiveWidth := m.width
	if effectiveWidth <= 0 {
		effectiveWidth = 80
	}
	return lipgloss.NewStyle().MaxWidth(effectiveWidth).MaxHeight(m.height).Render(final)
}

// renderMainBodyDashboard builds the two-column dashboard body.
func (m *monitorModel) renderMainBodyDashboard(grid layoutmgr.GridLayout) string {
	leftParts := []string{
		m.panelStyle(panelOutput).Width(grid.AgentOutput.InnerWidth()).Height(grid.AgentOutput.InnerHeight()).Render(
			m.styles.sectionTitle.Render("Agent Output") + "\n" + strings.TrimRight(m.agentOutputVP.View(), "\n"),
		),
	}
	left := lipgloss.JoinVertical(lipgloss.Left, leftParts...)

	tabBar := m.renderDashboardSidePanelTabBar(grid)
	rightContent := m.renderDashboardSidePanels(grid)
	right := lipgloss.JoinVertical(lipgloss.Left, tabBar, rightContent)
	right = lipgloss.NewStyle().Width(grid.ActivityFeed.Width).MaxWidth(grid.ActivityFeed.Width).Render(right)

	const gapCols = 1
	gap := strings.Repeat(" ", gapCols)
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
	return lipgloss.NewStyle().MaxWidth(grid.ScreenWidth).MaxHeight(grid.AgentOutput.Height).Render(row)
}

func (m *monitorModel) compactTabToPanel() panelID {
	if m.compactTab == len(compactTabs) {
		return panelComposer
	}
	if m.compactTab >= 0 && m.compactTab < len(compactTabs) {
		return compactTabs[m.compactTab].Panel
	}
	if len(compactTabs) == 0 {
		return panelOutput
	}
	return compactTabs[0].Panel
}

func (m *monitorModel) dashboardSideTabToPanel() panelID {
	tabs := m.activeDashboardSideTabs()
	if m.dashboardSideTab >= 0 && m.dashboardSideTab < len(tabs) {
		return tabs[m.dashboardSideTab].FocusPanel
	}
	if len(tabs) == 0 {
		return panelActivity
	}
	return tabs[0].FocusPanel
}

func (m *monitorModel) syncDashboardSideTabFromFocus() {
	tabs := m.activeDashboardSideTabs()
	for i, tab := range tabs {
		for _, panel := range tab.Panels {
			if panel == m.focusedPanel {
				m.dashboardSideTab = i
				return
			}
		}
	}
}

func (m *monitorModel) dashboardTabFocusCycle() []panelID {
	tabs := m.activeDashboardSideTabs()
	if m.dashboardSideTab >= 0 && m.dashboardSideTab < len(tabs) {
		return tabs[m.dashboardSideTab].FocusCycle
	}
	if len(tabs) == 0 {
		return []panelID{panelComposer}
	}
	return tabs[0].FocusCycle
}

func (m *monitorModel) renderCompactTabBar() string {
	parts := make([]string, len(compactTabs))
	for i, tab := range compactTabs {
		if i == m.compactTab {
			parts[i] = m.styles.sectionTitle.Render(tab.Name)
		} else {
			parts[i] = kit.StyleDim.Render(tab.Name)
		}
	}
	w := m.width
	if w <= 0 {
		w = 80
	}
	return m.styles.header.Copy().MaxWidth(w).Render(strings.Join(parts, "  |  "))
}

func (m *monitorModel) renderCompactTabContent(grid layoutmgr.GridLayout) string {
	if m.compactTab >= 0 && m.compactTab < len(compactTabs) {
		if render := compactTabs[m.compactTab].Render; render != nil {
			return render(m, grid)
		}
	}
	if len(compactTabs) == 0 || compactTabs[0].Render == nil {
		return ""
	}
	return compactTabs[0].Render(m, grid)
}

func (m *monitorModel) renderDashboardSidePanelTabBar(grid layoutmgr.GridLayout) string {
	tabs := m.activeDashboardSideTabs()
	parts := make([]string, len(tabs))
	for i, tab := range tabs {
		if i == m.dashboardSideTab {
			parts[i] = m.styles.sectionTitle.Render(tab.Name)
		} else {
			parts[i] = kit.StyleDim.Render(tab.Name)
		}
	}
	line := strings.Join(parts, "  |  ")
	w := grid.ActivityFeed.Width
	if w <= 0 {
		w = m.width
	}
	if w <= 0 {
		w = 80
	}
	return m.styles.header.Copy().MaxWidth(w).Render(line)
}

func (m *monitorModel) renderDashboardSidePanels(grid layoutmgr.GridLayout) string {
	tabs := m.activeDashboardSideTabs()
	if m.dashboardSideTab >= 0 && m.dashboardSideTab < len(tabs) {
		if render := tabs[m.dashboardSideTab].Render; render != nil {
			return render(m, grid)
		}
	}
	if len(tabs) == 0 || tabs[0].Render == nil {
		return ""
	}
	return tabs[0].Render(m, grid)
}

func renderCompactOutputTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	return m.panelStyle(panelOutput).
		Width(grid.AgentOutput.InnerWidth()).
		Height(grid.AgentOutput.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Agent Output") + "\n" + strings.TrimRight(m.agentOutputVP.View(), "\n"))
}

func renderCompactActivityTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	return m.renderActivitySideContent(grid)
}

func renderCompactPlanTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	return m.panelStyle(panelPlan).
		Width(grid.Plan.InnerWidth()).
		Height(grid.Plan.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Plan") + "\n" + m.planViewport.View())
}

func renderCompactOutboxTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	body := m.outboxVP.View()
	if footer := m.renderPaginationFooter(m.outboxPage, m.outboxPageSize, m.outboxTotalCount); footer != "" {
		body = strings.TrimRight(body, "\n") + "\n" + footer
	}
	return m.panelStyle(panelOutbox).
		Width(grid.Outbox.InnerWidth()).
		Height(grid.Outbox.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Outbox") + "\n" + body)
}

func renderDashboardActivityTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	return m.renderActivitySideContent(grid)
}

func renderDashboardPlanTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	return m.panelStyle(panelPlan).
		Width(grid.Plan.InnerWidth()).
		Height(grid.Plan.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Plan") + "\n" + m.planViewport.View())
}

func renderDashboardTasksTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	w := imax(10, grid.CurrentTask.ContentWidth)
	currentTaskBody := kit.StyleDim.Render("No active task")
	sectionTitle := "Current Task"
	if m.currentTask != nil {
		t := m.currentTask
		duration := time.Since(t.StartedAt).Round(time.Second)
		currentTaskBody = strings.Join([]string{
			kit.StyleStatusKey.Render("Goal:    ") + kit.StyleStatusValue.Render(truncateText(t.Goal, imax(10, w-12))),
			kit.StyleStatusKey.Render("Status:  ") + kit.StyleStatusValue.Render(fallback(t.Status, "unknown")),
			kit.StyleStatusKey.Render("Started: ") + t.StartedAt.Format("15:04:05"),
			kit.StyleStatusKey.Render("Elapsed: ") + duration.String(),
		}, "\n")
	}
	if strings.TrimSpace(m.teamID) != "" {
		sectionTitle = "Role Status"
		if len(m.teamRoles) == 0 {
			currentTaskBody = kit.StyleDim.Render("No role activity yet.")
		} else {
			lines := make([]string, 0, len(m.teamRoles))
			for _, role := range m.teamRoles {
				lines = append(lines, fmt.Sprintf("- %s: %s", strings.TrimSpace(role.Role), strings.TrimSpace(role.Info)))
			}
			currentTaskBody = strings.Join(lines, "\n")
		}
	}
	current := m.panelStyle(panelCurrentTask).
		Width(grid.CurrentTask.InnerWidth()).
		Height(grid.CurrentTask.InnerHeight()).
		Render(m.styles.sectionTitle.Render(sectionTitle) + "\n" + currentTaskBody)
	parts := []string{current}
	if grid.Inbox.Height > 0 {
		inboxBody := m.inboxVP.View()
		if footer := m.renderPaginationFooter(m.inboxPage, m.inboxPageSize, m.inboxTotalCount); footer != "" {
			inboxBody = strings.TrimRight(inboxBody, "\n") + "\n" + footer
		}
		inbox := m.panelStyle(panelInbox).
			Width(grid.Inbox.InnerWidth()).
			Height(grid.Inbox.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Inbox") + "\n" + inboxBody)
		parts = append(parts, inbox)
	}
	if grid.Outbox.Height > 0 {
		parts = append(parts, m.renderOutbox(grid.Outbox))
	}
	joined := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return lipgloss.NewStyle().MaxHeight(grid.Plan.Height).Render(joined)
}

func renderDashboardThoughtsTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	return m.panelStyle(panelThinking).
		Width(grid.Plan.InnerWidth()).
		Height(grid.Plan.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Thoughts") + "\n" + m.thinkingVP.View())
}

func (m *monitorModel) renderActivitySideContent(grid layoutmgr.GridLayout) string {
	feedBody := m.activityList.View()
	if footer := m.renderPaginationFooter(m.activityPage, m.activityPageSize, m.activityTotalCount); footer != "" {
		feedBody = strings.TrimRight(feedBody, "\n") + "\n" + footer
	}
	feed := m.panelStyle(panelActivity).
		Width(grid.ActivityFeed.InnerWidth()).
		Height(grid.ActivityFeed.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Activity Feed") + "\n" + feedBody)
	detail := m.panelStyle(panelActivityDetail).
		Width(grid.ActivityDetail.InnerWidth()).
		Height(grid.ActivityDetail.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Activity Details") + "\n" + m.activityDetail.View())
	return lipgloss.JoinVertical(lipgloss.Left, feed, detail)
}

func renderDashboardSubagentsTab(m *monitorModel, grid layoutmgr.GridLayout) string {
	if m == nil {
		return ""
	}
	currentRunID := strings.TrimSpace(m.runID)
	if strings.TrimSpace(m.teamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
		currentRunID = strings.TrimSpace(m.focusedRunID)
	}
	// Build list items: "Back to parent" when viewing a child run, then active subagents.
	var items []list.Item
	isViewingChild := false
	if currentRunID != "" && m.session != nil && m.ctx != nil {
		if run, err := m.session.LoadRun(m.ctx, currentRunID); err == nil && strings.TrimSpace(run.ParentRunID) != "" {
			isViewingChild = true
			items = append(items, subagentListItem{RunID: backToParentRunID, Label: "← Back to parent"})
		}
	}

	activeChildRuns := make([]types.Run, 0, len(m.childRuns))
	for _, r := range m.childRuns {
		if strings.EqualFold(strings.TrimSpace(r.Status), types.RunStatusRunning) {
			activeChildRuns = append(activeChildRuns, r)
		}
	}
	completedCount := len(m.childRuns) - len(activeChildRuns)

	if len(m.childRuns) == 0 {
		if strings.TrimSpace(m.childRunsLoadErr) != "" {
			items = append(items, subagentListItem{Label: "Could not load subagents: " + strings.TrimSpace(m.childRunsLoadErr)})
		} else if !isViewingChild {
			items = append(items, subagentListItem{Label: "No subagents spawned yet."})
		}
	} else if len(activeChildRuns) == 0 && !isViewingChild {
		msg := "No active subagents."
		if completedCount > 0 {
			msg = fmt.Sprintf("No active subagents. (%d completed.)", completedCount)
		}
		items = append(items, subagentListItem{Label: msg})
	} else {
		for _, run := range activeChildRuns {
			idx := run.SpawnIndex
			if idx <= 0 {
				idx = 1
			}
			goal := strings.TrimSpace(run.Goal)
			if goal == "" {
				goal = "(no goal)"
			}
			dur := ""
			if run.StartedAt != nil {
				end := time.Now()
				if run.FinishedAt != nil {
					end = *run.FinishedAt
				}
				dur = end.Sub(*run.StartedAt).Round(time.Second).String()
			}
			runID := strings.TrimSpace(run.RunID)
			assigned := 0
			completed := 0
			active := 0
			if m.childRunAssignedByRunID != nil {
				assigned = m.childRunAssignedByRunID[runID]
			}
			if m.childRunCompletedByRunID != nil {
				completed = m.childRunCompletedByRunID[runID]
			}
			if m.childRunActiveByRunID != nil {
				active = m.childRunActiveByRunID[runID]
			}
			workState := "idle"
			workGlyph := "·"
			if active > 0 {
				workState = "working"
				workGlyph = "●"
			} else if assigned > completed {
				workState = "queued"
				if completed > 0 {
					workState = "awaiting_review"
				}
				workGlyph = "○"
			}
			label := fmt.Sprintf(
				"%s Subagent-%d · %s (%s) · tasks %d/%d · %s",
				workGlyph,
				idx,
				truncateText(goal, 45),
				dur,
				completed,
				assigned,
				workState,
			)
			items = append(items, subagentListItem{RunID: run.RunID, Label: label})
		}
	}

	if len(items) == 0 {
		items = append(items, subagentListItem{Label: "No subagents spawned yet."})
	}

	m.subagentsList.SetItems(items)
	m.subagentsList.SetWidth(grid.Plan.InnerWidth())
	m.subagentsList.SetHeight(grid.Plan.InnerHeight())

	return m.panelStyle(panelSubagents).
		Width(grid.Plan.InnerWidth()).
		Height(grid.Plan.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Subagents") + "\nEnter: switch run · " + m.subagentsList.View())
}
