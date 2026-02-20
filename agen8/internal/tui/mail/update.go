package mail

import tea "github.com/charmbracelet/bubbletea"

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(fetchDataCmd(m.endpoint, m.sessionID), tickCmd())

	case dataLoadedMsg:
		if msg.err != nil {
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
			m.inboxSel = maxInt(0, len(m.inbox)-1)
		}
		if m.outboxSel >= len(m.outbox) {
			m.outboxSel = maxInt(0, len(m.outbox)-1)
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
