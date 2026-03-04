package mail

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
)

type mailReconnectNotificationMsg struct{}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		if m.notice != "" && time.Since(m.noticeAt) > 4*time.Second {
			m.notice = ""
		}
		if m.followProjectState {
			return m, tea.Batch(syncSessionCmd(m.projectRoot, m.sessionID), tickCmd())
		}
		return m, tickCmd()

	case adapter.NotificationConnErrorMsg:
		return m, tea.Sequence(
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return mailReconnectNotificationMsg{} }),
		)

	case mailReconnectNotificationMsg:
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
		m.inbox = msg.inbox
		m.outbox = msg.outbox
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
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.detailOpen {
			m.detailOpen = false
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

	case "r":
		return m, fetchDataCmd(m.endpoint, m.sessionID)
	}
	return m, nil
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
