package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type approvalPickerItem struct {
	title       string
	description string
	value       string
}

func (a approvalPickerItem) Title() string       { return a.title }
func (a approvalPickerItem) Description() string { return a.description }
func (a approvalPickerItem) FilterValue() string { return a.value }

var approvalPickerOptions = []approvalPickerItem{
	{
		title:       "Require Approval (Default)",
		description: "Agent requires approval for writes, shell commands, and network access.",
		value:       "enabled",
	},
	{
		title:       "Full Access",
		description: "Agent runs autonomously without interruption. Use with caution.",
		value:       "disabled",
	},
}

func (m *Model) openApprovalPicker() {
	if m.runtimeChangeLocked("changing approval mode") {
		return
	}
	m.approvalPickerOpen = true
	m.commandPaletteOpen = false
	m.commandPaletteMatches = nil
	m.commandPaletteSelected = 0

	sel := 0
	cur := strings.ToLower(strings.TrimSpace(m.approvalsMode))
	for i, opt := range approvalPickerOptions {
		if cur != "" && cur == opt.value {
			sel = i
			break
		}
	}
	m.approvalPickerSelected = sel
	m.layout()
}

func (m *Model) closeApprovalPicker() {
	m.approvalPickerOpen = false
	m.approvalPickerSelected = 0
	m.layout()
}

func (m *Model) selectApprovalOptionFromPicker() tea.Cmd {
	if !m.approvalPickerOpen {
		return nil
	}
	i := m.approvalPickerSelected
	if i < 0 || i >= len(approvalPickerOptions) {
		i = 0
	}
	val := approvalPickerOptions[i].value

	m.approvalsMode = val
	m.approvalPickerOpen = false
	m.approvalPickerSelected = 0
	m.layout()

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, "/approval "+val)
		return turnDoneMsg{final: final, err: err, preserveScroll: true}
	}
}
