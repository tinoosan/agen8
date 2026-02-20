package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/protocol"
	"github.com/tinoosan/agen8/pkg/types"
)

func (m *monitorModel) openSessionPicker() tea.Cmd {
	// Enforce single-modal semantics.
	m.helpModalOpen = false
	m.closeAllPickers()

	m.sessionPickerOpen = true
	m.sessionPickerErr = ""
	if m.sessionPickerPageSize <= 0 {
		m.sessionPickerPageSize = 50
	}
	m.sessionPickerPage = 0
	m.sessionPickerTotal = 0
	m.sessionPickerFilter = ""

	l := list.New(nil, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), renderSessionPickerLine), 0, 0)
	l.Title = "Select Session"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	l.SetFilterText("")
	l.SetFilterState(list.Unfiltered)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	m.sessionPickerList = l
	return m.fetchSessionsPage()
}

func (m *monitorModel) closeSessionPicker() {
	m.sessionPickerOpen = false
	m.sessionPickerList = list.Model{}
	m.sessionPickerErr = ""
	m.sessionPickerPage = 0
	m.sessionPickerTotal = 0
	m.sessionPickerFilter = ""
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
	switch msg.Type {
	case tea.KeyEsc:
		m.closeSessionPicker()
		return m, nil
	case tea.KeyEnter:
		return m, m.selectSessionFromPicker()
	case tea.KeyCtrlN, tea.KeyPgDown:
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
	case tea.KeyCtrlP, tea.KeyPgUp:
		if m.sessionPickerPage > 0 {
			m.sessionPickerPage--
			return m, m.fetchSessionsPage()
		}
		return m, nil
	case tea.KeyUp:
		m.sessionPickerList.CursorUp()
		return m, nil
	case tea.KeyDown:
		m.sessionPickerList.CursorDown()
		return m, nil
	case tea.KeyRunes:
		if m.sessionPickerList.FilterState() != list.Filtering && strings.EqualFold(strings.TrimSpace(msg.String()), "d") {
			selected := m.sessionPickerList.SelectedItem()
			item, ok := selected.(sessionPickerItem)
			if ok && strings.TrimSpace(item.id) != "" {
				return m, m.openSessionDeleteConfirm(strings.TrimSpace(item.id))
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.sessionPickerList, cmd = m.sessionPickerList.Update(msg)
		newFilter := strings.TrimSpace(m.sessionPickerList.FilterInput.Value())
		if newFilter != m.sessionPickerFilter {
			m.sessionPickerFilter = newFilter
			m.sessionPickerPage = 0
			return m, tea.Batch(cmd, m.fetchSessionsPage())
		}
		return m, cmd
	default:
		var cmd tea.Cmd
		m.sessionPickerList, cmd = m.sessionPickerList.Update(msg)
		newFilter := strings.TrimSpace(m.sessionPickerList.FilterInput.Value())
		if newFilter != m.sessionPickerFilter {
			m.sessionPickerFilter = newFilter
			m.sessionPickerPage = 0
			return m, tea.Batch(cmd, m.fetchSessionsPage())
		}
		return m, cmd
	}
}

func (m *monitorModel) selectSessionFromPicker() tea.Cmd {
	if m.sessionPickerList.Items() == nil || len(m.sessionPickerList.Items()) == 0 {
		return nil
	}
	selectedItem := m.sessionPickerList.SelectedItem()
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
	if strings.EqualFold(strings.TrimSpace(item.mode), "team") && strings.TrimSpace(item.teamID) != "" {
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
	dims := kit.ComputeModalDims(m.width, m.height, 80, 22, 48, 12, 8, 4)
	m.sessionPickerList.SetWidth(dims.ModalWidth - 4)
	m.sessionPickerList.SetHeight(dims.ListHeight)

	content := m.sessionPickerList.View()
	if strings.TrimSpace(m.sessionPickerErr) != "" {
		errLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8080")).Render("Error: " + m.sessionPickerErr)
		content = errLine + "\n\n" + content
	}
	content += "\n" + m.renderSessionPickerFooter()

	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}

func (m *monitorModel) renderSessionPickerFooter() string {
	if m.sessionPickerTotal == 0 {
		if strings.TrimSpace(m.sessionPickerErr) != "" {
			return kit.StyleDim.Render("Ctrl+N/P: page")
		}
		return kit.StyleDim.Render("No sessions")
	}

	pageSize := m.sessionPickerPageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	maxPage := (m.sessionPickerTotal + pageSize - 1) / pageSize
	currentPage := m.sessionPickerPage + 1

	pageInfo := fmt.Sprintf("Page %d of %d (%d sessions)", currentPage, maxPage, m.sessionPickerTotal)
	return kit.StyleDim.Render(pageInfo + " • Ctrl+N/P: page")
}
