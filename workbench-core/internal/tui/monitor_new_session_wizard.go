package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

type newSessionWizardItem struct {
	mode  string
	title string
	desc  string
}

func (i newSessionWizardItem) Title() string       { return i.title }
func (i newSessionWizardItem) Description() string { return i.desc }
func (i newSessionWizardItem) FilterValue() string {
	return strings.TrimSpace(i.mode + " " + i.title + " " + i.desc)
}

func (m *monitorModel) openNewSessionWizard() tea.Cmd {
	m.helpModalOpen = false
	if m.sessionPickerOpen {
		m.closeSessionPicker()
	}
	if m.agentPickerOpen {
		m.closeAgentPicker()
	}
	if m.profilePickerOpen {
		m.closeProfilePicker()
	}
	if m.teamPickerOpen {
		m.closeTeamPicker()
	}
	if m.modelPickerOpen {
		m.closeModelPicker()
	}
	if m.reasoningEffortPickerOpen {
		m.closeReasoningEffortPicker()
	}
	if m.reasoningSummaryPickerOpen {
		m.closeReasoningSummaryPicker()
	}
	if m.filePickerOpen {
		m.closeFilePicker()
	}

	items := []list.Item{
		newSessionWizardItem{mode: "standalone", title: "Standalone Session", desc: "single agent; choose profile and start"},
		newSessionWizardItem{mode: "team", title: "Team Session", desc: "multi-role team; profile is immutable"},
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "New Session Wizard"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(false)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")).Bold(true)
	l.Select(0)
	m.newSessionWizardList = l
	m.newSessionWizardOpen = true
	return nil
}

func (m *monitorModel) closeNewSessionWizard() {
	m.newSessionWizardOpen = false
	m.newSessionWizardList = list.Model{}
}

func (m *monitorModel) updateNewSessionWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.closeNewSessionWizard()
		return m, nil
	case tea.KeyEnter:
		item, ok := m.newSessionWizardList.SelectedItem().(newSessionWizardItem)
		if !ok {
			return m, nil
		}
		m.closeNewSessionWizard()
		if strings.EqualFold(strings.TrimSpace(item.mode), "team") {
			return m, m.openProfilePickerFor("new-team", true)
		}
		return m, m.openProfilePickerFor("new-standalone", false)
	default:
		var cmd tea.Cmd
		m.newSessionWizardList, cmd = m.newSessionWizardList.Update(msg)
		return m, cmd
	}
}

func (m *monitorModel) renderNewSessionWizard(base string) string {
	maxModalW := max(1, m.width-8)
	modalWidth := min(84, maxModalW)
	minModalW := min(56, maxModalW)
	if modalWidth < minModalW {
		modalWidth = minModalW
	}
	maxModalH := max(1, m.height-8)
	modalHeight := min(18, maxModalH)
	minModalH := min(10, maxModalH)
	if modalHeight < minModalH {
		modalHeight = minModalH
	}
	listHeight := modalHeight - 4
	if listHeight < 4 {
		listHeight = 4
	}
	m.newSessionWizardList.SetWidth(modalWidth - 4)
	m.newSessionWizardList.SetHeight(listHeight)

	content := m.newSessionWizardList.View() + "\n" + kit.StyleDim.Render("Enter: next • Esc: close")
	opts := kit.ModalOptions{
		Content:      content,
		ScreenWidth:  m.width,
		ScreenHeight: m.height,
		Width:        modalWidth,
		Height:       modalHeight,
		Padding:      [2]int{1, 2},
		BorderStyle:  lipgloss.RoundedBorder(),
		BorderColor:  lipgloss.Color("#6bbcff"),
		Foreground:   lipgloss.Color("#eaeaea"),
	}
	_ = base
	return kit.RenderOverlay(opts)
}
