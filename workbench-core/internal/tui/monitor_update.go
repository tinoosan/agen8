package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/workbench-core/pkg/types"
)

func (m *monitorModel) dispatchUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg, tickMsg, rpcHealthMsg:
		return m.handleWindowAndTick(msg)
	case tailedEventMsg, tailErrMsg, commandLinesMsg, monitorEditorDoneMsg, taskQueuedLocallyMsg, monitorSwitchRunMsg, monitorSwitchTeamMsg, monitorReloadedMsg:
		return m.handleTailAndStreamMessages(msg)
	case inboxLoadedMsg, outboxLoadedMsg, teamStatusLoadedMsg, teamManifestLoadedMsg, teamEventsLoadedMsg, activityLoadedMsg, sessionsListMsg, agentsListMsg, planFilesLoadedMsg, sessionTotalsLoadedMsg, artifactTreeLoadedMsg, artifactContentLoadedMsg, monitorFilePickerPathsMsg, childRunsLoadedMsg:
		return m.handleLoadedDataMessages(msg)
	case uiRefreshMsg, planReloadMsg, sessionTotalsReloadMsg:
		return m.handleMaintenanceMessages(msg)
	case tea.KeyMsg:
		return m.handleKeyMessage(msg)
	}

	return m, nil
}

func (m *monitorModel) handleWindowAndTick(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.artifactViewerOpen {
			m.refreshArtifactViewport()
			return m, nil
		}
		m.dirtyLayout = true
		m.refreshViewports()
		return m, nil

	case tickMsg:
		// Re-render time-based UI (uptime, elapsed timers) even when no new events arrive.
		// The View() computes elapsed durations on demand; no need to rebuild viewports.
		m.statusAnimFrame++
		m.expireStatus()
		if m.isDetached() {
			cmds := []tea.Cmd{m.tick()}
			if !m.rpcChecking {
				m.rpcChecking = true
				cmds = append(cmds, m.checkRPCHealthCmd(false))
			}
			return m, tea.Batch(cmds...)
		}
		if strings.TrimSpace(m.teamID) != "" {
			return m, tea.Batch(m.tick(), m.loadInboxPage(), m.loadOutboxPage(), m.loadActivityPage(), m.loadTeamStatus(), m.loadTeamEvents(), m.loadPlanFilesCmd(), m.loadTeamManifestCmd())
		}
		return m, m.tick()

	case rpcHealthMsg:
		m.rpcChecking = false
		m.rpcHealthKnown = true
		m.rpcReachable = msg.reachable
		if msg.err != nil {
			m.rpcLastErr = msg.err.Error()
		} else {
			m.rpcLastErr = ""
		}
		if msg.manual {
			if msg.reachable {
				m.appendAgentOutput("[system] Daemon RPC connected at " + strings.TrimSpace(m.rpcEndpoint))
			} else {
				m.appendAgentOutput("[system] Daemon RPC disconnected: " + strings.TrimSpace(m.rpcLastErr) + " (retry with /reconnect)")
			}
		}
		return m, m.scheduleUIRefresh()
	}
	return m, nil
}

func (m *monitorModel) handleTailAndStreamMessages(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tailedEventMsg:
		if msg.ev.Event.EventID != "" {
			m.offset = msg.ev.NextOffset
			m.observeEvent(msg.ev.Event)
		}
		cmds := []tea.Cmd{m.listenEvent()}
		if shouldReloadPlanOnEvent(msg.ev.Event) {
			cmds = append(cmds, m.schedulePlanReload())
		}
		switch msg.ev.Event.Type {
		case "llm.cost.total", "llm.usage.total":
			cmds = append(cmds, m.scheduleSessionTotalsReload())
		}
		// Keep paginated lists in sync without loading everything into memory.
		switch msg.ev.Event.Type {
		case "task.queued", "webhook.task.queued", "task.start":
			cmds = append(cmds, m.loadInboxPage())
		case "task.done", "task.quarantined":
			cmds = append(cmds, m.loadInboxPage(), m.loadOutboxPage())
		case "task.delivered":
			cmds = append(cmds, m.loadOutboxPage())
		case "agent.op.request", "agent.op.response":
			if m.activityFollowingTail {
				// If a new page is created, overshoot so loadActivityPage clamps to the new last page.
				m.activityPage = max(0, (m.activityTotalCount+m.activityPageSize-1)/max(1, m.activityPageSize))
				cmds = append(cmds, m.loadActivityPage())
			} else if msg.ev.Event.Type == "agent.op.response" {
				opID := strings.TrimSpace(msg.ev.Event.Data["opId"])
				if opID != "" {
					for _, a := range m.activityPageItems {
						if strings.TrimSpace(a.ID) == opID {
							cmds = append(cmds, m.loadActivityPage())
							break
						}
					}
				}
			}
			if msg.ev.Event.Type == "agent.op.response" {
				op := strings.TrimSpace(msg.ev.Event.Data["op"])
				tag := strings.TrimSpace(msg.ev.Event.Data["tag"])
				if op == "agent_spawn" || op == "task_create" || tag == "task_create" {
					cmds = append(cmds, m.loadChildRuns())
				}
			}
		case "subagent.spawned":
			cmds = append(cmds, m.loadChildRuns())
		}
		cmds = append(cmds, m.scheduleUIRefresh())
		return m, tea.Batch(cmds...)

	case tailErrMsg:
		if msg.err != nil {
			m.appendAgentOutput("[error] " + msg.err.Error())
		}
		return m, tea.Batch(m.listenErr(), m.scheduleUIRefresh())

	case commandLinesMsg:
		if len(msg.lines) != 0 {
			for _, line := range msg.lines {
				m.appendAgentOutput(line)
			}
			if strings.HasPrefix(strings.TrimSpace(msg.lines[0]), "[memory] search:") {
				m.memResults = msg.lines[1:]
				m.dirtyMemory = true
			}
		}
		return m, m.scheduleUIRefresh()

	case monitorEditorDoneMsg:
		m.handleEditorDone(msg)
		return m, m.scheduleUIRefresh()

	case taskQueuedLocallyMsg:
		m.inbox[msg.TaskID] = taskState{TaskID: msg.TaskID, Goal: msg.Goal, Status: string(types.TaskStatusPending)}
		m.appendAgentOutput(fmt.Sprintf("[%s] task.queued %s %s", time.Now().Local().Format("15:04:05"), shortID(msg.TaskID), truncateText(strings.TrimSpace(msg.Goal), 80)))
		m.dirtyInbox = true
		return m, tea.Batch(m.loadInboxPage(), m.scheduleUIRefresh())

	case monitorSwitchRunMsg:
		runID := strings.TrimSpace(msg.RunID)
		if runID == "" {
			return m, m.scheduleUIRefresh()
		}
		return m, m.reloadAsRun(runID)

	case monitorSwitchTeamMsg:
		teamID := strings.TrimSpace(msg.TeamID)
		if teamID == "" {
			return m, m.scheduleUIRefresh()
		}
		return m, m.reloadAsTeam(teamID)

	case monitorReloadedMsg:
		if msg.err != nil {
			m.appendAgentOutput("[switch] error: " + msg.err.Error())
			return m, m.scheduleUIRefresh()
		}
		if msg.model == nil {
			return m, m.scheduleUIRefresh()
		}
		return msg.model, tea.Batch(msg.model.Init(), msg.model.scheduleUIRefresh())
	}
	return m, nil
}

func (m *monitorModel) handleLoadedDataMessages(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case childRunsLoadedMsg:
		if msg.err != nil {
			m.childRunsLoadErr = msg.err.Error()
			m.dirtyInbox = true
			return m, m.scheduleUIRefresh()
		}
		m.childRunsLoadErr = ""
		m.childRuns = msg.runs
		m.dirtyInbox = true // Re-render dashboard
		return m, m.scheduleUIRefresh()

	case inboxLoadedMsg:
		m.inboxList = msg.tasks
		m.inboxTotalCount = msg.totalCount
		m.inboxPage = msg.page
		m.dirtyInbox = true
		return m, m.scheduleUIRefresh()

	case outboxLoadedMsg:
		m.outboxResults = msg.entries
		m.outboxTotalCount = msg.totalCount
		m.outboxPage = msg.page
		if strings.TrimSpace(m.teamID) != "" {
			for _, entry := range msg.entries {
				if strings.TrimSpace(entry.TaskID) == "" {
					continue
				}
				if _, ok := m.seenOutboxByTask[entry.TaskID]; ok {
					continue
				}
				m.seenOutboxByTask[entry.TaskID] = struct{}{}
				rolePrefix := strings.TrimSpace(entry.AssignedRole)
				if rolePrefix == "" {
					rolePrefix = "team"
				}
				ts := entry.Timestamp.Local().Format("15:04:05")
				line := fmt.Sprintf("[%s] [%s] task.done %s %s", ts, rolePrefix, shortID(entry.TaskID), strings.TrimSpace(entry.Status))
				if summary := strings.TrimSpace(entry.Summary); summary != "" {
					line += " - " + truncateText(summary, 120)
				}
				m.appendAgentOutputForRun(line, strings.TrimSpace(entry.RunID))
			}
		}
		m.dirtyOutbox = true
		return m, m.scheduleUIRefresh()

	case teamStatusLoadedMsg:
		m.teamPendingCount = msg.pending
		m.teamActiveCount = msg.active
		m.teamDoneCount = msg.done
		m.teamRoles = msg.roles
		if len(msg.runIDs) != 0 {
			m.teamRunIDs = msg.runIDs
		}
		if len(msg.roleByRunID) != 0 {
			if m.teamRoleByRunID == nil {
				m.teamRoleByRunID = map[string]string{}
			}
			for runID, role := range msg.roleByRunID {
				m.teamRoleByRunID[runID] = role
			}
		}
		m.stats.totalTokensIn = msg.totalTokensIn
		m.stats.totalTokensOut = msg.totalTokensOut
		m.stats.totalTokens = msg.totalTokens
		m.stats.totalCostUSD = msg.totalCostUSD
		m.stats.pricingKnown = msg.pricingKnown
		return m, tea.Batch(m.ensureFocusedRunStillValid(), m.scheduleUIRefresh())

	case teamManifestLoadedMsg:
		if msg.err != nil || msg.manifest == nil {
			return m, nil
		}
		manifest := msg.manifest
		if profileID := strings.TrimSpace(manifest.ProfileID); profileID != "" {
			m.profile = profileID
		}
		if modelChange := manifest.ModelChange; modelChange != nil {
			requested := strings.TrimSpace(modelChange.RequestedModel)
			status := strings.ToLower(strings.TrimSpace(modelChange.Status))
			if requested != "" && (status == "pending" || status == "applied") {
				m.model = requested
			} else if teamModel := strings.TrimSpace(manifest.TeamModel); teamModel != "" {
				m.model = teamModel
			}
		} else if teamModel := strings.TrimSpace(manifest.TeamModel); teamModel != "" {
			m.model = teamModel
		}

		// Re-enforce session ActiveModel if available, as it is the source of truth for the *current*
		// runtime state, whereas manifest reflects configuration/requested state.
		if m.session != nil && strings.TrimSpace(m.sessionID) != "" {
			if sess, err := m.session.LoadSession(m.ctx, strings.TrimSpace(m.sessionID)); err == nil {
				if active := strings.TrimSpace(sess.ActiveModel); active != "" {
					m.model = active
				}
			}
		}
		m.teamModelChange = manifest.ModelChange
		m.teamCoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
		m.teamCoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
		m.sessionID = resolveTeamControlSessionID(manifest, m.sessionID)
		if len(manifest.Roles) != 0 {
			roleByRun := map[string]string{}
			runIDs := make([]string, 0, len(manifest.Roles))
			for _, role := range manifest.Roles {
				runID := strings.TrimSpace(role.RunID)
				if runID == "" {
					continue
				}
				runIDs = append(runIDs, runID)
				roleByRun[runID] = strings.TrimSpace(role.RoleName)
			}
			if len(runIDs) != 0 {
				m.teamRunIDs = runIDs
				m.teamRoleByRunID = roleByRun
			}
		}
		return m, tea.Batch(m.ensureFocusedRunStillValid(), m.scheduleUIRefresh())

	case teamEventsLoadedMsg:
		if len(msg.cursors) != 0 {
			if m.teamEventCursor == nil {
				m.teamEventCursor = map[string]int64{}
			}
			for runID, cursor := range msg.cursors {
				m.teamEventCursor[runID] = cursor
			}
		}
		if len(msg.events) != 0 {
			reloadPlan := false
			for _, ev := range msg.events {
				m.observeEvent(ev)
				if shouldReloadPlanOnEvent(ev) {
					reloadPlan = true
				}
			}
			if reloadPlan {
				return m, tea.Batch(m.loadPlanFilesCmd(), m.scheduleUIRefresh())
			}
		}
		return m, m.scheduleUIRefresh()

	case activityLoadedMsg:
		m.activityPageItems = msg.activities
		m.activityTotalCount = msg.totalCount
		m.activityPage = msg.page
		m.dirtyActivity = true
		return m, m.scheduleUIRefresh()

	case sessionsListMsg:
		if msg.err != nil {
			m.sessionPickerErr = msg.err.Error()
			m.sessionPickerList.SetItems(nil)
			return m, m.scheduleUIRefresh()
		}
		m.sessionPickerErr = ""
		m.sessionPickerTotal = msg.total
		m.sessionPickerPage = msg.page
		items := msg.items
		if len(items) == 0 {
			items = sessionsToPickerItems(msg.sessions)
		}
		m.sessionPickerList.SetItems(items)
		if strings.TrimSpace(m.sessionPickerFilter) == "" {
			m.sessionPickerList.SetFilterText("")
			m.sessionPickerList.SetFilterState(list.Unfiltered)
		} else {
			m.sessionPickerList.SetFilterText(strings.TrimSpace(m.sessionPickerFilter))
			m.sessionPickerList.SetFilterState(list.Filtering)
		}
		if len(items) > 0 {
			m.sessionPickerList.Select(0)
		}
		return m, m.scheduleUIRefresh()

	case agentsListMsg:
		if msg.err != nil {
			m.agentPickerErr = msg.err.Error()
			m.agentPickerList.SetItems(nil)
			return m, m.scheduleUIRefresh()
		}
		m.agentPickerErr = ""
		items := agentsToPickerItems(msg.agents)
		m.agentPickerList.SetItems(items)
		if len(items) > 0 {
			m.agentPickerList.Select(0)
		}
		return m, m.scheduleUIRefresh()

	case planFilesLoadedMsg:
		m.planMarkdown = msg.checklist
		m.planLoadErr = msg.checklistErr
		m.planDetails = msg.details
		m.planDetailsErr = msg.detailsErr
		m.dirtyPlan = true
		return m, m.scheduleUIRefresh()

	case sessionTotalsLoadedMsg:
		if msg.err == nil {
			m.stats.totalTokensIn = msg.session.InputTokens
			m.stats.totalTokensOut = msg.session.OutputTokens
			m.stats.totalTokens = msg.session.TotalTokens
			m.stats.totalCostUSD = msg.session.CostUSD
			m.stats.pricingKnown = msg.pricingKnown
			if msg.tasksDone > 0 {
				m.stats.tasksDone = msg.tasksDone
			}
		}
		return m, m.scheduleUIRefresh()

	case artifactTreeLoadedMsg:
		return m, tea.Batch(m.handleArtifactTreeLoaded(msg), m.scheduleUIRefresh())

	case artifactContentLoadedMsg:
		m.handleArtifactContentLoaded(msg)
		return m, m.scheduleUIRefresh()

	case monitorFilePickerPathsMsg:
		m.handleFilePickerPaths(msg.paths)
		return m, nil
	}
	return m, nil
}

func (m *monitorModel) handleMaintenanceMessages(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case uiRefreshMsg:
		m.uiRefreshScheduled = false
		m.refreshViewports()
		return m, nil

	case planReloadMsg:
		m.planReloadScheduled = false
		return m, m.loadPlanFilesCmd()

	case sessionTotalsReloadMsg:
		m.sessionTotalsReloadScheduled = false
		return m, m.loadSessionTotalsCmd()
	default:
		_ = msg
	}
	return m, nil
}

func (m *monitorModel) handleKeyMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if handled, model, cmd := m.routeArtifactViewerKey(msg); handled {
		return model, cmd
	}
	if handled, model, cmd := m.routeModalKey(msg); handled {
		return model, cmd
	}
	if handled, model, cmd := m.routeHelpHotkey(msg); handled {
		return model, cmd
	}
	if handled, model, cmd := m.routeComposerPaletteSubmitKey(msg); handled {
		return model, cmd
	}
	if handled, model, cmd := m.routeLayoutNavigationKey(msg); handled {
		return model, cmd
	}
	if handled, model, cmd := m.routeGlobalShortcutKey(msg); handled {
		return model, cmd
	}
	if m.isCompactMode() {
		// Keep explicit focus on Activity Details if the user selected it.
		if !(m.compactTab == 1 && m.focusedPanel == panelActivityDetail) {
			m.focusedPanel = m.compactTabToPanel()
		}
		m.updateFocus()
	}
	if handled, model, cmd := m.routePaginationKey(msg); handled {
		return model, cmd
	}
	return m.routeKeyToFocusedPanel(msg)
}

func (m *monitorModel) routeArtifactViewerKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	if m.artifactViewerOpen {
		model, cmd := m.updateArtifactViewer(msg)
		return true, model, cmd
	}
	return false, m, nil
}

func (m *monitorModel) routeModalKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	// Modal overlay handling - if any modal is open, handle it first.
	if m.helpModalOpen {
		switch msg.String() {
		case "esc", "escape", "?":
			m.closeHelpModal()
			return true, m, nil
		}
		return true, m, nil // Consume all other keys when help is open.
	}
	if m.sessionPickerOpen {
		model, cmd := m.updateSessionPicker(msg)
		return true, model, cmd
	}
	if m.newSessionWizardOpen {
		model, cmd := m.updateNewSessionWizard(msg)
		return true, model, cmd
	}
	if m.agentPickerOpen {
		model, cmd := m.updateAgentPicker(msg)
		return true, model, cmd
	}
	if m.profilePickerOpen {
		model, cmd := m.updateProfilePicker(msg)
		return true, model, cmd
	}
	if m.teamPickerOpen {
		model, cmd := m.updateTeamPicker(msg)
		return true, model, cmd
	}
	if m.modelPickerOpen {
		model, cmd := m.updateModelPicker(msg)
		return true, model, cmd
	}
	if m.reasoningEffortPickerOpen {
		model, cmd := m.updateReasoningEffortPicker(msg)
		return true, model, cmd
	}
	if m.reasoningSummaryPickerOpen {
		model, cmd := m.updateReasoningSummaryPicker(msg)
		return true, model, cmd
	}
	if m.filePickerOpen {
		model, cmd := m.updateFilePicker(msg)
		return true, model, cmd
	}
	return false, m, nil
}

func (m *monitorModel) routeHelpHotkey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	// Help modal hotkey:
	// - When not composing, "?" should always open help.
	// - When composing, allow "?" to open help only if the composer is empty
	//   (otherwise treat it as a literal character the user wants to type).
	if msg.String() == "?" {
		if m.focusedPanel != panelComposer || strings.TrimSpace(m.input.Value()) == "" {
			m.openHelpModal()
			return true, m, nil
		}
	}
	return false, m, nil
}

func (m *monitorModel) routeComposerPaletteSubmitKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	if m.focusedPanel != panelComposer {
		return false, m, nil
	}
	// Handle command palette key events first.
	if cmd, ok := m.handleCommandPaletteKey(msg); ok {
		return true, m, cmd
	}

	if strings.EqualFold(msg.String(), "ctrl+e") {
		seed := strings.TrimSpace(m.input.Value())
		m.input.SetValue("")
		return true, m, m.openComposeEditor(seed)
	}

	key := strings.ToLower(msg.String())
	if key == "ctrl+enter" ||
		key == "ctrl+j" ||
		key == "ctrl+m" ||
		key == "ctrl+o" ||
		msg.Type == tea.KeyCtrlJ ||
		msg.Type == tea.KeyCtrlM ||
		msg.Type == tea.KeyCtrlO ||
		(msg.Type == tea.KeyEnter && msg.Alt) {
		cmd := strings.TrimSpace(m.input.Value())
		m.input.SetValue("")
		if cmd == "" {
			return true, m, nil
		}
		return true, m, m.handleCommand(cmd)
	}
	return false, m, nil
}

func (m *monitorModel) routeLayoutNavigationKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	// Compact mode: allow switching tabs and focusing Activity subpanels so
	// long details can be scrolled.
	if m.isCompactMode() {
		switch msg.String() {
		case "ctrl+]":
			m.compactTab = (m.compactTab + 1) % len(compactTabs)
			if m.focusedPanel != panelComposer {
				m.focusedPanel = m.compactTabToPanel()
			}
			m.updateFocus()
			m.refreshViewports()
			return true, m, nil
		case "ctrl+[":
			m.compactTab = (m.compactTab + len(compactTabs) - 1) % len(compactTabs)
			if m.focusedPanel != panelComposer {
				m.focusedPanel = m.compactTabToPanel()
			}
			m.updateFocus()
			m.refreshViewports()
			return true, m, nil
		case "ctrl+down", "ctrl+j":
			if m.compactTab == 1 && m.focusedPanel != panelComposer { // Activity tab
				m.focusedPanel = panelActivityDetail
				m.updateFocus()
				return true, m, nil
			}
		case "ctrl+up", "ctrl+k":
			if m.compactTab == 1 && m.focusedPanel != panelComposer { // Activity tab
				m.focusedPanel = panelActivity
				m.updateFocus()
				return true, m, nil
			}
		}
	}
	// Dashboard mode: quick focus toggle between Activity Feed and Details.
	if !m.isCompactMode() && m.dashboardSideTab == 0 && m.focusedPanel != panelComposer {
		switch msg.String() {
		case "ctrl+down":
			m.focusedPanel = panelActivityDetail
			m.updateFocus()
			return true, m, nil
		case "ctrl+up":
			m.focusedPanel = panelActivity
			m.updateFocus()
			return true, m, nil
		}
	}
	return false, m, nil
}

func (m *monitorModel) routeGlobalShortcutKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.cancel != nil {
			m.cancel()
		}
		return true, m, tea.Quit
	case "ctrl+g":
		if strings.TrimSpace(m.teamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
			m.focusedRunID = ""
			m.focusedRunRole = ""
			return true, m, m.applyFocusLens()
		}
	case "ctrl+y":
		if !m.isCompactMode() {
			m.dashboardSideTab = 3
			m.focusedPanel = panelThinking
			m.updateFocus()
			m.refreshViewports()
			return true, m, nil
		}
	case "tab", "shift+tab":
		if m.isCompactMode() {
			// Toggle focus between the Composer and the current tab's panel.
			if m.focusedPanel == panelComposer {
				m.focusedPanel = m.compactTabToPanel()
			} else {
				m.focusedPanel = panelComposer
			}
			m.updateFocus()
			return true, m, nil
		}
		cycle := m.dashboardTabFocusCycle()
		if len(cycle) == 0 {
			cycle = []panelID{panelComposer}
		}
		idx := slices.Index(cycle, m.focusedPanel)
		if idx < 0 {
			idx = 0
		}
		switch msg.String() {
		case "tab":
			idx = (idx + 1) % len(cycle)
		case "shift+tab":
			idx = (idx + len(cycle) - 1) % len(cycle)
		}
		m.focusedPanel = cycle[idx]
		m.syncDashboardSideTabFromFocus()
		m.updateFocus()
		m.refreshViewports()
		cmd := tea.Cmd(nil)
		tabs := m.activeDashboardSideTabs()
		if m.dashboardSideTab < len(tabs) && tabs[m.dashboardSideTab].Name == "Subagents" {
			cmd = m.loadChildRuns()
		}
		return true, m, cmd
	case "ctrl+]":
		if !m.isCompactMode() {
			tabs := m.activeDashboardSideTabs()
			if len(tabs) == 0 {
				return true, m, nil
			}
			m.dashboardSideTab = (m.dashboardSideTab + 1) % len(tabs)
			m.focusedPanel = m.dashboardSideTabToPanel()
			m.updateFocus()
			m.refreshViewports()
			cmd := tea.Cmd(nil)
			if m.dashboardSideTab < len(tabs) && tabs[m.dashboardSideTab].Name == "Subagents" {
				cmd = m.loadChildRuns()
			}
			return true, m, cmd
		}
	case "ctrl+[":
		if !m.isCompactMode() {
			tabs := m.activeDashboardSideTabs()
			if len(tabs) == 0 {
				return true, m, nil
			}
			m.dashboardSideTab = (m.dashboardSideTab + len(tabs) - 1) % len(tabs)
			m.focusedPanel = m.dashboardSideTabToPanel()
			m.updateFocus()
			m.refreshViewports()
			cmd := tea.Cmd(nil)
			if m.dashboardSideTab < len(tabs) && tabs[m.dashboardSideTab].Name == "Subagents" {
				cmd = m.loadChildRuns()
			}
			return true, m, cmd
		}
	}
	return false, m, nil
}

func (m *monitorModel) routePaginationKey(msg tea.KeyMsg) (bool, tea.Model, tea.Cmd) {
	// Pagination controls for paginated panels when focused.
	if m.focusedPanel == panelActivity || m.focusedPanel == panelInbox || m.focusedPanel == panelOutbox {
		switch msg.String() {
		case "n", "right":
			model, cmd := m.handleNextPage()
			return true, model, cmd
		case "p", "left":
			model, cmd := m.handlePrevPage()
			return true, model, cmd
		case "g":
			model, cmd := m.handleFirstPage()
			return true, model, cmd
		case "G":
			model, cmd := m.handleLastPage()
			return true, model, cmd
		}
	}
	return false, m, nil
}
