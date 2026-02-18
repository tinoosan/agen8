package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/protocol"
)

func (m *monitorModel) openHistoryClearConfirm() tea.Cmd {
	if m == nil {
		return nil
	}
	m.confirmModalOpen = true
	m.confirmAction = confirmActionClearHistory
	m.confirmSessionID = ""
	if strings.TrimSpace(m.teamID) != "" {
		m.confirmModalTitle = "Clear Team History"
		m.confirmModalBody = "Clear history/events for all team runs including coordinator?"
	} else {
		m.confirmModalTitle = "Clear Session History"
		m.confirmModalBody = "Clear history/events for this session?"
	}
	m.confirmModalHint = "Enter to confirm, Esc to cancel"
	return nil
}

func (m *monitorModel) openSessionDeleteConfirm(sessionID string) tea.Cmd {
	if m == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	m.confirmModalOpen = true
	m.confirmAction = confirmActionDeleteSession
	m.confirmSessionID = sessionID
	m.confirmModalTitle = "Delete Session"
	m.confirmModalBody = "Delete session " + shortID(sessionID) + "? This cannot be undone."
	m.confirmModalHint = "Enter to confirm, Esc to cancel"
	return nil
}

func (m *monitorModel) closeConfirmModal() {
	if m == nil {
		return
	}
	m.confirmModalOpen = false
	m.confirmModalTitle = ""
	m.confirmModalBody = ""
	m.confirmModalHint = ""
	m.confirmAction = ""
	m.confirmSessionID = ""
}

func (m *monitorModel) updateConfirmModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m == nil || !m.confirmModalOpen {
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		m.closeConfirmModal()
		return m, nil
	case tea.KeyEnter:
		action := m.confirmAction
		sessionID := strings.TrimSpace(m.confirmSessionID)
		m.closeConfirmModal()
		return m, m.executeConfirmAction(action, sessionID)
	default:
		return m, nil
	}
}

func (m *monitorModel) executeConfirmAction(action monitorConfirmAction, sessionID string) tea.Cmd {
	switch action {
	case confirmActionClearHistory:
		return m.runHistoryClear()
	case confirmActionDeleteSession:
		return m.runSessionDelete(sessionID)
	default:
		return nil
	}
}

func (m *monitorModel) clearHistoryBuffers() {
	if m == nil {
		return
	}
	m.agentOutput = nil
	m.agentOutputRunID = nil
	m.agentOutputLineHeights = nil
	m.agentOutputWindowStartLine = 0
	m.activityPageItems = nil
	m.activityTotalCount = 0
	m.childRuns = nil
	m.childRunsLoadErr = ""
}

func (m *monitorModel) runHistoryClear() tea.Cmd {
	if m == nil {
		return nil
	}
	if m.isDetached() {
		return func() tea.Msg {
			return historyClearedMsg{err: fmt.Errorf("no active context")}
		}
	}
	return func() tea.Msg {
		params := protocol.SessionClearHistoryParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
		} else {
			params.SessionID = strings.TrimSpace(m.sessionID)
		}
		var res protocol.SessionClearHistoryResult
		if err := m.rpcRoundTrip(protocol.MethodSessionClearHistory, params, &res); err != nil {
			return historyClearedMsg{err: err}
		}
		return historyClearedMsg{result: res}
	}
}

func (m *monitorModel) runSessionDelete(sessionID string) tea.Cmd {
	sessionID = strings.TrimSpace(sessionID)
	if m == nil || sessionID == "" {
		return nil
	}
	return func() tea.Msg {
		err := m.rpcRoundTrip(protocol.MethodSessionDelete, protocol.SessionDeleteParams{
			SessionID: sessionID,
		}, &protocol.SessionDeleteResult{})
		if err != nil {
			return sessionDeletedMsg{sessionID: sessionID, err: err}
		}
		return sessionDeletedMsg{
			sessionID:      sessionID,
			deletedCurrent: strings.EqualFold(sessionID, strings.TrimSpace(m.sessionID)),
		}
	}
}

func (m *monitorModel) renderConfirmModal(base string) string {
	title := strings.TrimSpace(m.confirmModalTitle)
	if title == "" {
		title = "Confirm Action"
	}
	body := strings.TrimSpace(m.confirmModalBody)
	if body == "" {
		body = "Proceed?"
	}
	hint := strings.TrimSpace(m.confirmModalHint)
	if hint == "" {
		hint = "Enter to confirm, Esc to cancel"
	}
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Render(title),
		"",
		body,
		"",
		kit.StyleDim.Render(hint),
	)
	opts := kit.ModalOptions{
		Content:      content,
		ScreenWidth:  m.width,
		ScreenHeight: m.height,
		Width:        min(64, max(40, m.width-8)),
		Height:       min(10, max(8, m.height-6)),
		Padding:      [2]int{1, 2},
		BorderStyle:  lipgloss.RoundedBorder(),
		BorderColor:  lipgloss.Color("#6bbcff"),
		Foreground:   lipgloss.Color("#eaeaea"),
	}
	_ = base
	return kit.RenderOverlay(opts)
}
