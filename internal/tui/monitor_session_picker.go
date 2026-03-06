package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

func (m *monitorModel) openSessionPicker() tea.Cmd {
	// Enforce single-modal semantics.
	m.helpModalOpen = false
	m.closeAllPickers()

	m.sessionPickerErr = ""
	m.sessionPickerCtrl.Open(kit.PickerConfig{
		Title:            "Select Session",
		FilteringEnabled: true,
		ShowFilter:       true,
		PageKeyNav:       true,
		PageSize:         50,
		Delegate:         newSessionPickerDelegate(),
		ModalWidth:       80,
		ModalHeight:      22,
		ModalMinWidth:    48,
		ModalMinHeight:   12,
		ModalMarginX:     8,
		ModalMarginY:     4,
	})
	m.syncSessionPickerState()
	return m.fetchSessionsPage()
}

func (m *monitorModel) closeSessionPicker() {
	m.sessionPickerCtrl.Close()
	m.syncSessionPickerState()
	m.sessionPickerErr = ""
}

func (m *monitorModel) fetchSessionsPage() tea.Cmd {
	return func() tea.Msg {
		var res protocol.SessionListResult
		err := m.rpcRoundTrip(protocol.MethodSessionList, protocol.SessionListParams{
			ThreadID:      protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			TitleContains: m.sessionPickerFilter,
			Limit:         m.sessionPickerPageSize,
			Offset:        m.sessionPickerPage * m.sessionPickerPageSize,
		}, &res)
		if err != nil {
			return sessionsListMsg{err: err}
		}
		sessions := make([]types.Session, 0, len(res.Sessions))
		for _, it := range res.Sessions {
			sessions = append(sessions, types.Session{
				SessionID:    strings.TrimSpace(it.SessionID),
				Title:        strings.TrimSpace(it.Title),
				CurrentRunID: strings.TrimSpace(it.CurrentRunID),
				ActiveModel:  strings.TrimSpace(it.ActiveModel),
				Mode:         strings.TrimSpace(it.Mode),
				TeamID:       strings.TrimSpace(it.TeamID),
				Profile:      strings.TrimSpace(it.Profile),
			})
		}
		items := sessionsToPickerItems(sessions)
		for i := range res.Sessions {
			if i >= len(items) {
				break
			}
			sp, ok := items[i].(sessionPickerItem)
			if !ok {
				continue
			}
			sp.runningAgents = max(0, res.Sessions[i].RunningAgents)
			sp.pausedAgents = max(0, res.Sessions[i].PausedAgents)
			sp.totalAgents = max(sp.runningAgents+sp.pausedAgents, max(0, res.Sessions[i].TotalAgents))
			items[i] = sp
		}
		return sessionsListMsg{sessions: sessions, items: items, total: res.TotalCount, page: m.sessionPickerPage, err: nil}
	}
}

func (m *monitorModel) updateSessionPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ensureSessionPickerCtrl()
	if msg.Type == tea.KeyRunes && strings.EqualFold(strings.TrimSpace(msg.String()), "d") {
		selected := m.sessionPickerCtrl.SelectedItem()
		item, ok := selected.(sessionPickerItem)
		if ok && strings.TrimSpace(item.id) != "" {
			return m, m.openSessionDeleteConfirm(strings.TrimSpace(item.id))
		}
		return m, nil
	}

	action := m.sessionPickerCtrl.Update(msg)
	m.syncSessionPickerState()
	switch action.Type {
	case kit.PickerActionClose:
		m.closeSessionPicker()
		return m, nil
	case kit.PickerActionAccept:
		return m, m.selectSessionFromPicker()
	case kit.PickerActionPageNext:
		pageSize := m.sessionPickerPageSize
		if pageSize <= 0 {
			pageSize = 50
		}
		maxPage := (m.sessionPickerTotal+pageSize-1)/pageSize - 1
		if maxPage < 0 {
			maxPage = 0
		}
		if m.sessionPickerPage < maxPage {
			m.sessionPickerPage++
			return m, m.fetchSessionsPage()
		}
		return m, nil
	case kit.PickerActionPagePrev:
		if m.sessionPickerPage > 0 {
			m.sessionPickerPage--
			m.sessionPickerCtrl.SetPage(m.sessionPickerPage, m.sessionPickerTotal, m.sessionPickerPageSize)
			m.syncSessionPickerState()
			return m, m.fetchSessionsPage()
		}
		return m, nil
	case kit.PickerActionFilterChanged:
		if action.Filter != m.sessionPickerFilter {
			m.sessionPickerFilter = action.Filter
			m.sessionPickerPage = 0
			m.sessionPickerCtrl.SetPage(0, m.sessionPickerTotal, m.sessionPickerPageSize)
			m.syncSessionPickerState()
			return m, tea.Batch(action.Cmd, m.fetchSessionsPage())
		}
	}
	return m, action.Cmd
}

func (m *monitorModel) selectSessionFromPicker() tea.Cmd {
	m.ensureSessionPickerCtrl()
	selectedItem := m.sessionPickerCtrl.SelectedItem()
	if selectedItem == nil {
		return nil
	}
	item, ok := selectedItem.(sessionPickerItem)
	if !ok {
		return nil
	}
	sessID := strings.TrimSpace(item.id)
	if sessID == "" {
		return nil
	}
	if strings.TrimSpace(item.teamID) != "" {
		teamID := strings.TrimSpace(item.teamID)
		m.closeSessionPicker()
		return func() tea.Msg { return monitorSwitchTeamMsg{TeamID: teamID} }
	}

	var agents protocol.AgentListResult
	if err := m.rpcRoundTrip(protocol.MethodAgentList, protocol.AgentListParams{
		ThreadID:  protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
		SessionID: sessID,
	}, &agents); err != nil {
		m.sessionPickerErr = err.Error()
		return nil
	}
	runID := ""
	teamID := ""

	// Candidate selection priorities:
	// 1. Top-level run (ParentRunID=="") that is Running.
	// 2. Top-level run (ParentRunID=="") of any status.
	// 3. Any running run.
	// 4. Any run.
	var bestRunID string
	var bestScore int // 3=TopRunning, 2=Top, 1=Running, 0=Any

	for _, ag := range agents.Agents {
		candidate := strings.TrimSpace(ag.RunID)
		if candidate == "" {
			continue
		}
		if teamID == "" {
			teamID = strings.TrimSpace(ag.TeamID)
		}

		isTopLevel := strings.TrimSpace(ag.ParentRunID) == ""
		isRunning := strings.EqualFold(strings.TrimSpace(ag.Status), types.RunStatusRunning)

		score := 0
		if isTopLevel && isRunning {
			score = 3
		} else if isTopLevel {
			score = 2
		} else if isRunning {
			score = 1
		}

		if bestRunID == "" || score > bestScore {
			bestRunID = candidate
			bestScore = score
		}
	}
	runID = bestRunID
	if strings.TrimSpace(teamID) != "" {
		m.closeSessionPicker()
		targetTeamID := strings.TrimSpace(teamID)
		return func() tea.Msg { return monitorSwitchTeamMsg{TeamID: targetTeamID} }
	}
	if runID == "" {
		m.sessionPickerErr = "session has no current run"
		return nil
	}

	m.closeSessionPicker()
	return func() tea.Msg { return monitorSwitchRunMsg{RunID: runID} }
}

func (m *monitorModel) renderSessionPicker(base string) string {
	m.ensureSessionPickerCtrl()
	_ = base
	return m.sessionPickerCtrl.Render(m.width, m.height, m.renderSessionPickerFooter(), m.sessionPickerErr)
}

func (m *monitorModel) renderSessionPickerFooter() string {
	return pickerFooter(m.sessionPickerTotal, m.sessionPickerPage, m.sessionPickerPageSize, strings.TrimSpace(m.sessionPickerErr), "No sessions", kit.StyleDim)
}
