package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	layoutmgr "github.com/tinoosan/workbench-core/internal/tui/layout"
	"github.com/tinoosan/workbench-core/pkg/types"
)

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

// renderCompact builds the view for compact mode: header + tab bar + main content + composer.
func (m *monitorModel) renderCompact(grid layoutmgr.GridLayout, headerLine string) string {
	tabBar := m.renderCompactTabBar()
	content := m.renderCompactTabContent(grid)
	sections := []string{headerLine}
	if m.rpcHealthKnown && !m.rpcReachable {
		sections = append(sections, kit.StyleDim.Render("Daemon disconnected. Start `workbench daemon` and run /reconnect."))
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
	if m.dashboardSideTab >= 0 && m.dashboardSideTab < len(dashboardSideTabs) {
		return dashboardSideTabs[m.dashboardSideTab].FocusPanel
	}
	if len(dashboardSideTabs) == 0 {
		return panelActivity
	}
	return dashboardSideTabs[0].FocusPanel
}

func (m *monitorModel) syncDashboardSideTabFromFocus() {
	for i, tab := range dashboardSideTabs {
		for _, panel := range tab.Panels {
			if panel == m.focusedPanel {
				m.dashboardSideTab = i
				return
			}
		}
	}
}

func (m *monitorModel) dashboardTabFocusCycle() []panelID {
	if m.dashboardSideTab >= 0 && m.dashboardSideTab < len(dashboardSideTabs) {
		return dashboardSideTabs[m.dashboardSideTab].FocusCycle
	}
	if len(dashboardSideTabs) == 0 {
		return []panelID{panelComposer}
	}
	return dashboardSideTabs[0].FocusCycle
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
	parts := make([]string, len(dashboardSideTabs))
	for i, tab := range dashboardSideTabs {
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
	if m.dashboardSideTab >= 0 && m.dashboardSideTab < len(dashboardSideTabs) {
		if render := dashboardSideTabs[m.dashboardSideTab].Render; render != nil {
			return render(m, grid)
		}
	}
	if len(dashboardSideTabs) == 0 || dashboardSideTabs[0].Render == nil {
		return ""
	}
	return dashboardSideTabs[0].Render(m, grid)
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
	return m.panelStyle(panelOutbox).
		Width(grid.Outbox.InnerWidth()).
		Height(grid.Outbox.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Outbox") + "\n" + m.outboxVP.View())
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
		inbox := m.panelStyle(panelInbox).
			Width(grid.Inbox.InnerWidth()).
			Height(grid.Inbox.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Inbox") + "\n" + m.inboxVP.View())
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
	var lines []string
	currentRunID := strings.TrimSpace(m.runID)
	if strings.TrimSpace(m.teamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
		currentRunID = strings.TrimSpace(m.focusedRunID)
	}
	// Only show active (running) subagents; completed ones are cleaned up and no longer need to be in the list.
	activeChildRuns := make([]types.Run, 0, len(m.childRuns))
	for _, r := range m.childRuns {
		if strings.EqualFold(strings.TrimSpace(r.Status), types.RunStatusRunning) {
			activeChildRuns = append(activeChildRuns, r)
		}
	}
	completedCount := len(m.childRuns) - len(activeChildRuns)

	if len(m.childRuns) == 0 {
		if strings.TrimSpace(m.childRunsLoadErr) != "" {
			lines = []string{kit.StyleDim.Render("Could not load subagents: " + strings.TrimSpace(m.childRunsLoadErr))}
		} else if currentRunID != "" {
			if run, err := store.LoadRun(m.cfg, currentRunID); err == nil && strings.TrimSpace(run.ParentRunID) != "" {
				lines = []string{kit.StyleDim.Render("You are viewing a subagent. Switch to the parent run to see all subagents.")}
			} else {
				lines = []string{kit.StyleDim.Render("No subagents spawned yet.")}
			}
		} else {
			lines = []string{kit.StyleDim.Render("No subagents spawned yet.")}
		}
	} else if len(activeChildRuns) == 0 {
		msg := "No active subagents."
		if completedCount > 0 {
			msg = fmt.Sprintf("No active subagents. (%d completed.)", completedCount)
		}
		lines = []string{kit.StyleDim.Render(msg)}
	} else {
		for i, run := range activeChildRuns {
			idx := i + 1
			if run.SpawnIndex > 0 {
				idx = run.SpawnIndex
			}
			statusStyle := kit.StyleStatusValue
			status := strings.ToLower(run.Status)
			switch status {
			case "succeeded":
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950"))
			case "failed", "canceled":
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
			case "running":
				statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6bbcff"))
			}

			dur := ""
			if run.StartedAt != nil {
				end := time.Now()
				if run.FinishedAt != nil {
					end = *run.FinishedAt
				}
				dur = end.Sub(*run.StartedAt).Round(time.Second).String()
			}

			costStr := ""
			if run.CostUSD > 0 {
				costStr = fmt.Sprintf(" ($%.4f)", run.CostUSD)
			}

			// Format: "1. [running] Goal... (2m30s) ($0.12)"
			line := fmt.Sprintf("%d. [%s] %s (%s)%s",
				idx,
				statusStyle.Render(status),
				truncateText(strings.TrimSpace(run.Goal), 60),
				dur,
				costStr,
			)
			lines = append(lines, line)
		}
	}

	content := strings.Join(lines, "\n")
	m.subagentsVP.SetContent(content)
	m.subagentsVP.Width = grid.Plan.InnerWidth()
	m.subagentsVP.Height = grid.Plan.InnerHeight()

	// Ensure cursor stays within bounds if content shrinks
	if m.subagentsVP.YOffset > 0 {
		maxOffset := max(0, len(lines)-m.subagentsVP.Height)
		if m.subagentsVP.YOffset > maxOffset {
			m.subagentsVP.YOffset = maxOffset
		}
	}

	return m.panelStyle(panelSubagents).
		Width(grid.Plan.InnerWidth()).
		Height(grid.Plan.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Subagents") + "\n" + m.subagentsVP.View())
}
