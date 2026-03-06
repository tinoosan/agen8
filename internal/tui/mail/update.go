package mail

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

type mailReconnectNotificationMsg struct{}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.spinFrame = (m.spinFrame + 1) % len(kit.SpinnerFrames)
		if m.notice != "" && time.Since(m.noticeAt) > 4*time.Second {
			m.notice = ""
		}
		switch m.mode {
		case viewProject:
			return m, tea.Batch(fetchProjectDataCmd(m.endpoint, m.projectRoot), tickCmd())
		case viewTeam:
			// Only auto-sync session when running in session-first mode
			// (explicit --session-id without a project overview to navigate).
			if m.followProjectState && m.sessionExplicit {
				return m, tea.Batch(syncSessionCmd(m.projectRoot, m.endpoint, m.sessionID), tickCmd())
			}
			return m, tickCmd()
		}
		return m, tickCmd()

	case adapter.NotificationConnErrorMsg:
		return m, tea.Sequence(
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return mailReconnectNotificationMsg{} }),
		)

	case mailReconnectNotificationMsg:
		fetchCmd := m.activeFetchCmd()
		return m, tea.Batch(
			fetchCmd,
			adapter.StartNotificationListenerCmd(m.endpoint),
		)

	case adapter.EventPushedMsg:
		fetchCmd := m.activeFetchCmd()
		return m, tea.Batch(
			fetchCmd,
			adapter.WaitForNextNotificationCmd(msg.Ch, msg.ErrCh),
		)

	case projectDataLoadedMsg:
		if msg.err != nil {
			m.connected = false
			m.lastErr = msg.err.Error()
			return m, nil
		}
		m.connected = true
		m.lastErr = ""
		m.teams = msg.teams
		m.projectID = msg.projectID
		if m.teamSel >= len(m.teams) {
			m.teamSel = max(0, len(m.teams)-1)
		}
		return m, nil

	case sessionSyncedMsg:
		// Ignore session-sync results in project-first mode.
		if !m.sessionExplicit {
			return m, fetchDataCmd(m.endpoint, m.sessionID)
		}
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			return m, fetchDataCmd(m.endpoint, m.sessionID)
		}
		if msg.changed {
			m.sessionID = strings.TrimSpace(msg.sessionID)
			m.inboxSel = 0
			m.outboxSel = 0
			m.detailOpen = false
			if time.Since(m.noticeAt) > 3*time.Second {
				m.notice = fmt.Sprintf("switched to %s", strings.TrimSpace(msg.sessionID))
				m.noticeAt = time.Now()
			}
		}
		return m, fetchDataCmd(m.endpoint, m.sessionID)

	case dataLoadedMsg:
		if msg.err != nil {
			if msg.preserve {
				m.connected = true
				if !m.followProjectState {
					m.lastErr = msg.err.Error()
				}
				return m, nil
			}
			m.connected = false
			m.lastErr = msg.err.Error()
			return m, nil
		}
		m.connected = true
		m.lastErr = ""
		m.inbox = m.applyExpansionState(msg.inbox)
		m.outbox = m.applyExpansionState(msg.outbox)
		m.currentTask = msg.current

		// Clamp selections
		if m.inboxSel >= len(m.inbox) {
			m.inboxSel = max(0, len(m.inbox)-1)
		}
		if m.outboxSel >= len(m.outbox) {
			m.outboxSel = max(0, len(m.outbox)-1)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case viewProject:
			return m.handleProjectKey(msg)
		case viewTeam:
			return m.handleMailKey(msg)
		}
		return m, nil
	}
	return m, nil
}

// activeFetchCmd returns the appropriate fetch command for the current view mode.
func (m *Model) activeFetchCmd() tea.Cmd {
	if m.mode == viewProject {
		return fetchProjectDataCmd(m.endpoint, m.projectRoot)
	}
	return fetchDataCmd(m.endpoint, m.sessionID)
}

// ---------------------------------------------------------------------------
// Project overview key handling
// ---------------------------------------------------------------------------

func (m *Model) handleProjectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "j", "down":
		m.moveTeamSelection(1)
		return m, nil
	case "k", "up":
		m.moveTeamSelection(-1)
		return m, nil
	case "g", "home":
		m.teamSel = 0
		return m, nil
	case "G", "end":
		if len(m.teams) > 0 {
			m.teamSel = len(m.teams) - 1
		}
		return m, nil
	case "enter":
		return m.drillIntoTeam()
	case "r":
		return m, fetchProjectDataCmd(m.endpoint, m.projectRoot)
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Mail view key handling
// ---------------------------------------------------------------------------

func (m *Model) handleMailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.detailOpen {
			m.detailOpen = false
			return m, nil
		}
		// Return to project view when navigated from there.
		if !m.sessionExplicit {
			return m.drillOutToProject()
		}
		return m, nil

	case "q", "ctrl+c":
		return m, tea.Quit

	case "tab":
		m.detailOpen = false
		if m.focus == panelInbox {
			m.focus = panelOutbox
		} else {
			m.focus = panelInbox
		}
		return m, nil

	case "j", "down":
		m.moveSelection(1)
		return m, nil

	case "k", "up":
		m.moveSelection(-1)
		return m, nil

	case "enter":
		m.detailOpen = !m.detailOpen
		return m, nil

	case " ":
		if m.detailOpen {
			return m, nil
		}
		m.toggleSelectedGroup()
		return m, nil

	case "r":
		return m, fetchDataCmd(m.endpoint, m.sessionID)

	case "[":
		if !m.sessionExplicit {
			return m.switchTeam(-1)
		}
		return m, nil

	case "]":
		if !m.sessionExplicit {
			return m.switchTeam(1)
		}
		return m, nil
	}
	return m, nil
}

// ---------------------------------------------------------------------------
// Navigation helpers
// ---------------------------------------------------------------------------

func (m *Model) drillIntoTeam() (tea.Model, tea.Cmd) {
	if m.teamSel < 0 || m.teamSel >= len(m.teams) {
		return m, nil
	}
	team := m.teams[m.teamSel]
	if strings.TrimSpace(team.PrimarySessionID) == "" {
		m.notice = fmt.Sprintf("Team %s is inactive. Start the team to resume live activity.", strings.TrimSpace(team.TeamID))
		m.noticeAt = time.Now()
		return m, nil
	}
	m.mode = viewTeam
	m.selectedTeam = &team
	m.sessionID = team.PrimarySessionID
	m.inboxSel = 0
	m.outboxSel = 0
	m.detailOpen = false
	m.focus = panelInbox
	m.inbox = nil
	m.outbox = nil
	m.currentTask = nil
	return m, fetchDataCmd(m.endpoint, m.sessionID)
}

func (m *Model) switchTeam(delta int) (tea.Model, tea.Cmd) {
	if len(m.teams) == 0 {
		return m, nil
	}
	next := m.teamSel + delta
	if next < 0 || next >= len(m.teams) {
		return m, nil
	}
	team := m.teams[next]
	if strings.TrimSpace(team.PrimarySessionID) == "" {
		m.notice = fmt.Sprintf("Team %s is inactive. Start the team to resume live activity.", strings.TrimSpace(team.TeamID))
		m.noticeAt = time.Now()
		return m, nil
	}
	m.teamSel = next
	m.selectedTeam = &team
	m.sessionID = team.PrimarySessionID
	m.inboxSel = 0
	m.outboxSel = 0
	m.detailOpen = false
	m.focus = panelInbox
	m.inbox = nil
	m.outbox = nil
	m.currentTask = nil
	return m, fetchDataCmd(m.endpoint, m.sessionID)
}

func (m *Model) drillOutToProject() (tea.Model, tea.Cmd) {
	m.mode = viewProject
	m.selectedTeam = nil
	m.inbox = nil
	m.outbox = nil
	m.currentTask = nil
	m.inboxSel = 0
	m.outboxSel = 0
	m.detailOpen = false
	m.focus = panelInbox
	return m, fetchProjectDataCmd(m.endpoint, m.projectRoot)
}

func (m *Model) moveTeamSelection(delta int) {
	if len(m.teams) == 0 {
		return
	}
	m.teamSel += delta
	if m.teamSel < 0 {
		m.teamSel = 0
	}
	if m.teamSel >= len(m.teams) {
		m.teamSel = len(m.teams) - 1
	}
}

func (m *Model) moveSelection(delta int) {
	switch m.focus {
	case panelInbox:
		if len(m.inbox) == 0 {
			return
		}
		m.inboxSel += delta
		if m.inboxSel < 0 {
			m.inboxSel = 0
		}
		if m.inboxSel >= len(m.inbox) {
			m.inboxSel = len(m.inbox) - 1
		}
	case panelOutbox:
		if len(m.outbox) == 0 {
			return
		}
		m.outboxSel += delta
		if m.outboxSel < 0 {
			m.outboxSel = 0
		}
		if m.outboxSel >= len(m.outbox) {
			m.outboxSel = len(m.outbox) - 1
		}
	}
}
