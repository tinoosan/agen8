package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type monitorPanel struct {
	id        panelID
	name      string
	handleKey func(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd)
}

func (p monitorPanel) ID() panelID {
	return p.id
}

func (p monitorPanel) Name() string {
	return p.name
}

func (p monitorPanel) HandleKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if p.handleKey == nil {
		return m, nil
	}
	return p.handleKey(m, msg)
}

var panelRegistry = map[panelID]monitorPanel{}

func registerPanel(p monitorPanel) {
	panelRegistry[p.ID()] = p
}

func init() {
	registerPanel(monitorPanel{id: panelActivity, name: "Activity", handleKey: handleActivityPanelKey})
	registerPanel(monitorPanel{id: panelActivityDetail, name: "Details", handleKey: handleActivityDetailPanelKey})
	registerPanel(monitorPanel{id: panelPlan, name: "Plan", handleKey: handlePlanPanelKey})
	registerPanel(monitorPanel{id: panelCurrentTask, name: "Current Task", handleKey: handleCurrentTaskPanelKey})
	registerPanel(monitorPanel{id: panelOutput, name: "Output", handleKey: handleOutputPanelKey})
	registerPanel(monitorPanel{id: panelInbox, name: "Inbox", handleKey: handleInboxPanelKey})
	registerPanel(monitorPanel{id: panelOutbox, name: "Outbox", handleKey: handleOutboxPanelKey})
	registerPanel(monitorPanel{id: panelMemory, name: "Memory", handleKey: handleMemoryPanelKey})
	registerPanel(monitorPanel{id: panelComposer, name: "Composer", handleKey: handleComposerPanelKey})
	registerPanel(monitorPanel{id: panelThinking, name: "Thoughts", handleKey: handleThinkingPanelKey})
}

func handleActivityPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	prev := m.activityList.Index()
	m.activityList, cmd = m.activityList.Update(msg)
	if m.activityList.Index() != prev {
		m.refreshActivityDetail(true)
	}
	if isScrollKey(msg) {
		m.activityFollowingTail = false
	}
	return m, cmd
}

func handleActivityDetailPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.activityDetail, cmd = m.activityDetail.Update(msg)
	return m, cmd
}

func handlePlanPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.planViewport, cmd = m.planViewport.Update(msg)
	if isScrollKey(msg) {
		m.planFollowingTop = m.planViewport.YOffset <= 0
	}
	return m, cmd
}

func handleCurrentTaskPanelKey(m *monitorModel, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Current task panel is static, no interactive model.
	return m, nil
}

func handleOutputPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isScrollKey(msg) {
		m.applyAgentOutputScroll(msg)
		m.refreshAgentOutputViewport()
		m.agentOutputFollow = m.agentOutputAtBottom()
		return m, nil
	}
	// Non-scroll keys: nothing to do (viewport is read-only).
	return m, nil
}

func handleInboxPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.inboxVP, cmd = m.inboxVP.Update(msg)
	return m, cmd
}

func handleOutboxPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.outboxVP, cmd = m.outboxVP.Update(msg)
	return m, cmd
}

func handleMemoryPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.memoryVP, cmd = m.memoryVP.Update(msg)
	return m, cmd
}

func handleThinkingPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.thinkingVP, cmd = m.thinkingVP.Update(msg)
	if isScrollKey(msg) {
		m.thinkingAutoScroll = m.thinkingAtBottom()
	}
	return m, cmd
}

func handleComposerPanelKey(m *monitorModel, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.updateCommandPalette()
	return m, cmd
}

func (m *monitorModel) panelStyle(panel panelID) lipgloss.Style {
	if m.focusedPanel == panel {
		return m.styles.panelFocused
	}
	return m.styles.panel
}

func (m *monitorModel) focusedPanelName() string {
	if p, ok := panelRegistry[m.focusedPanel]; ok {
		if name := p.Name(); name != "" {
			return name
		}
	}
	return "Unknown"
}

func (m *monitorModel) routeKeyToFocusedPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if p, ok := panelRegistry[m.focusedPanel]; ok {
		return p.HandleKey(m, msg)
	}
	return m, nil
}
