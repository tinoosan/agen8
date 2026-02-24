package dashboardtui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
)

type dashboardtuiReconnectNotificationMsg struct{}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.spinFrame = (m.spinFrame + 1) % len(spinnerFrames)
		if m.notice != "" && time.Since(m.noticeAt) > 4*time.Second {
			m.notice = ""
		}
		if m.followProjectState {
			return m, tea.Batch(syncSessionCmd(m.projectRoot, m.sessionID), tickCmd())
		}
		return m, tickCmd()

	case adapter.NotificationConnErrorMsg:
		return m, tea.Sequence(
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return dashboardtuiReconnectNotificationMsg{} }),
		)

	case dashboardtuiReconnectNotificationMsg:
		return m, tea.Batch(
			fetchDataCmd(m.endpoint, m.sessionID),
			adapter.StartNotificationListenerCmd(m.endpoint),
		)

	case adapter.EventPushedMsg:
		return m, tea.Batch(
			fetchDataCmd(m.endpoint, m.sessionID),
			adapter.WaitForNextNotificationCmd(msg.Ch, msg.ErrCh),
		)

	case sessionSyncedMsg:
		if msg.err != nil {
			m.lastErr = msg.err.Error()
			return m, fetchDataCmd(m.endpoint, m.sessionID)
		}
		if msg.changed {
			m.sessionID = strings.TrimSpace(msg.sessionID)
			m.sel = 0
			m.detailOpen = false
			m.detailScroll = 0
			now := time.Now()
			if time.Since(m.noticeAt) > 3*time.Second {
				m.notice = fmt.Sprintf("switched to %s", strings.TrimSpace(msg.sessionID))
				m.noticeAt = now
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
		m.agents = msg.agents
		m.stats = msg.stats
		m.sessionMode = msg.sessionMode
		m.teamID = msg.teamID
		m.runID = msg.runID
		m.reviewerRole = msg.reviewerRole

		if m.sel >= len(m.agents) {
			m.sel = maxInt(0, len(m.agents)-1)
		}
		if len(m.agents) == 0 {
			m.sel = 0
			m.detailOpen = false
			m.detailScroll = 0
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.detailOpen {
			m.detailOpen = false
			m.detailScroll = 0
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
		return m, nil

	case "k", "up":
		if m.detailOpen {
			m.detailScroll--
			if m.detailScroll < 0 {
				m.detailScroll = 0
			}
			return m, nil
		}
		m.moveSelection(-1)
		return m, nil

	case "g", "home":
		m.sel = 0
		m.detailScroll = 0
		return m, nil

	case "G", "end":
		if len(m.agents) > 0 {
			m.sel = len(m.agents) - 1
		}
		m.detailScroll = 0
		return m, nil

	case "enter":
		if len(m.agents) == 0 {
			return m, nil
		}
		m.detailOpen = !m.detailOpen
		m.detailScroll = 0
		return m, nil

	case "r":
		return m, fetchDataCmd(m.endpoint, m.sessionID)
	}
	return m, nil
}

func (m *Model) moveSelection(delta int) {
	if len(m.agents) == 0 {
		return
	}
	m.sel += delta
	if m.sel < 0 {
		m.sel = 0
	}
	if m.sel >= len(m.agents) {
		m.sel = len(m.agents) - 1
	}
	m.detailScroll = 0
}
