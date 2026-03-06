package activitytui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

type activitytuiReconnectNotificationMsg struct{}

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
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return activitytuiReconnectNotificationMsg{} }),
		)

	case activitytuiReconnectNotificationMsg:
		fetchCmd := m.activeFetchCmd()
		return m, tea.Batch(
			fetchCmd,
			adapter.StartNotificationListenerCmd(m.endpoint),
		)

	case adapter.EventPushedMsg:
		act, ok := adapter.EventRecordToActivity(msg.Record)
		if !ok {
			fetchCmd := m.activeFetchCmd()
			return m, tea.Batch(
				fetchCmd,
				adapter.WaitForNextNotificationCmd(msg.Ch, msg.ErrCh),
			)
		}

		prevLen := len(m.activities)
		m.activities = append(m.activities, act)
		m.totalCount++

		if m.liveFollow && len(m.activities) > prevLen {
			m.sel = len(m.activities) - 1
		}
		if m.sel >= len(m.activities) {
			m.sel = max(0, len(m.activities)-1)
		}

		return m, adapter.WaitForNextNotificationCmd(msg.Ch, msg.ErrCh)

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
			m.sel = 0
			m.detailOpen = false
			m.detailScroll = 0
			m.liveFollow = true
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
				return m, nil
			}
			m.connected = false
			m.lastErr = msg.err.Error()
			return m, nil
		}
		m.connected = true
		m.lastErr = ""

		prevLen := len(m.activities)
		m.activities = msg.activities
		m.totalCount = msg.totalCount

		// Auto-select newest item when live-following and new data arrives
		if m.liveFollow && len(m.activities) > prevLen {
			m.sel = len(m.activities) - 1
		}

		// Clamp selection
		if m.sel >= len(m.activities) {
			m.sel = max(0, len(m.activities)-1)
		}
		return m, nil

	case tea.KeyMsg:
		switch m.mode {
		case viewProject:
			return m.handleProjectKey(msg)
		case viewTeam:
			return m.handleActivityKey(msg)
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
// Activity view key handling
// ---------------------------------------------------------------------------

func (m *Model) handleActivityKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.detailOpen {
			m.detailOpen = false
			m.detailScroll = 0
			return m, nil
		}
		// Return to project view when navigated from there.
		if !m.sessionExplicit {
			return m.drillOutToProject()
		}
		return m, nil

	case "q", "ctrl+c":
		return m, tea.Quit

	case "j", "down":
		if m.detailOpen {
			m.detailScroll++
			return m, nil
		}
		m.moveSelection(1)
		// If we're at the bottom, re-enable live follow
		if m.sel >= len(m.activities)-1 {
			m.liveFollow = true
		}
		return m, nil

	case "k", "up":
		if m.detailOpen {
			m.detailScroll--
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
			return m, nil
		}
		m.liveFollow = false
		m.moveSelection(-1)
		return m, nil

	case "g", "home":
		m.sel = 0
		m.detailScroll = 0
		return m, nil

	case "G", "end":
		if len(m.activities) > 0 {
			m.sel = len(m.activities) - 1
		}
		m.liveFollow = true
		m.detailScroll = 0
		return m, nil

	case "enter":
		m.detailOpen = !m.detailOpen
		m.detailScroll = 0
		return m, nil

	case "pgdown":
		if m.detailOpen {
			m.detailScroll += 5
		} else {
			m.moveSelection(10)
			if m.sel >= len(m.activities)-1 {
				m.liveFollow = true
			}
		}
		return m, nil

	case "pgup":
		if m.detailOpen {
			m.detailScroll -= 5
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
		} else {
			m.liveFollow = false
			m.moveSelection(-10)
		}
		return m, nil

	case "r":
		return m, fetchDataCmd(m.endpoint, m.sessionID)

	case "t":
		m.showTimestamps = !m.showTimestamps
		return m, nil

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
	m.sel = 0
	m.detailOpen = false
	m.detailScroll = 0
	m.liveFollow = true
	m.activities = nil
	m.totalCount = 0
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
	m.sel = 0
	m.detailOpen = false
	m.detailScroll = 0
	m.liveFollow = true
	m.activities = nil
	m.totalCount = 0
	return m, fetchDataCmd(m.endpoint, m.sessionID)
}

func (m *Model) drillOutToProject() (tea.Model, tea.Cmd) {
	m.mode = viewProject
	m.selectedTeam = nil
	m.activities = nil
	m.totalCount = 0
	m.sel = 0
	m.detailOpen = false
	m.detailScroll = 0
	m.liveFollow = true
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
	if len(m.activities) == 0 {
		return
	}
	m.sel += delta
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= len(m.activities) {
		m.sel = len(m.activities) - 1
	}
	m.detailScroll = 0
}
