package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

type teamPickerItem struct {
	runID   string
	role    string
	isClear bool
}

func (t teamPickerItem) FilterValue() string {
	return strings.TrimSpace(strings.Join([]string{t.role, t.runID}, " "))
}

func (t teamPickerItem) Title() string {
	if t.isClear {
		return "Clear Focus (All Agents)"
	}
	role := strings.TrimSpace(t.role)
	if role == "" {
		role = "unknown"
	}
	return role
}

func (t teamPickerItem) Description() string {
	if t.isClear {
		return ""
	}
	runID := strings.TrimSpace(t.runID)
	if runID == "" {
		return ""
	}
	return "run: " + shortID(runID)
}

func renderTeamPickerLine(item list.Item, maxWidth int) string {
	it, ok := item.(teamPickerItem)
	if !ok {
		return kit.TruncateRight(strings.TrimSpace(item.FilterValue()), maxWidth)
	}
	line := strings.TrimSpace(it.Title())
	if desc := strings.TrimSpace(it.Description()); desc != "" {
		line += " — " + desc
	}
	return kit.TruncateRight(line, maxWidth)
}

func (m *monitorModel) openTeamPicker() tea.Cmd {
	m.closeHelpModal()
	m.closeAllPickers()

	items := []teamPickerItem{{isClear: true}}
	seen := map[string]struct{}{}
	runIDs := append([]string(nil), m.teamRunIDs...)
	for runID := range m.teamRoleByRunID {
		runIDs = append(runIDs, runID)
	}
	// When team has one agent, ensure current run is in the list.
	if len(runIDs) == 0 {
		if r := strings.TrimSpace(m.runID); r != "" && !strings.HasPrefix(r, "team:") {
			runIDs = append(runIDs, r)
		}
	}
	for _, runID := range runIDs {
		runID = strings.TrimSpace(runID)
		if runID == "" {
			continue
		}
		if _, ok := seen[runID]; ok {
			continue
		}
		seen[runID] = struct{}{}
		items = append(items, teamPickerItem{
			runID: runID,
			role:  strings.TrimSpace(m.teamRoleByRunID[runID]),
		})
	}
	sort.Slice(items[1:], func(i, j int) bool {
		a := items[i+1]
		b := items[j+1]
		ak := strings.ToLower(strings.TrimSpace(a.role))
		bk := strings.ToLower(strings.TrimSpace(b.role))
		if ak == bk {
			return a.runID < b.runID
		}
		return ak < bk
	})

	listItems := make([]list.Item, 0, len(items))
	selectedIdx := 0
	for i, it := range items {
		listItems = append(listItems, it)
		if strings.TrimSpace(m.focusedRunID) != "" && it.runID == strings.TrimSpace(m.focusedRunID) {
			selectedIdx = i
		}
	}

	l := list.New(listItems, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), renderTeamPickerLine), 0, 0)
	l.Title = "Focus Team Run"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	l.SetFilterText("")
	l.SetFilterState(list.Filtering)
	l.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")).Bold(true)
	if len(listItems) > 0 {
		l.Select(selectedIdx)
	}

	m.teamPickerList = l
	m.teamPickerOpen = true
	return nil
}

func (m *monitorModel) closeTeamPicker() {
	m.teamPickerOpen = false
	m.teamPickerList = list.Model{}
}

func (m *monitorModel) updateTeamPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "escape":
		m.closeTeamPicker()
		return m, nil
	case "enter":
		return m, m.selectFromTeamPicker()
	case "up", "k":
		m.teamPickerList.CursorUp()
		return m, nil
	case "down", "j":
		m.teamPickerList.CursorDown()
		return m, nil
	}

	var cmd tea.Cmd
	m.teamPickerList, cmd = m.teamPickerList.Update(msg)
	return m, cmd
}

func (m *monitorModel) selectFromTeamPicker() tea.Cmd {
	if m.teamPickerList.Items() == nil || len(m.teamPickerList.Items()) == 0 {
		return nil
	}
	selectedItem := m.teamPickerList.SelectedItem()
	if selectedItem == nil {
		return nil
	}
	item, ok := selectedItem.(teamPickerItem)
	if !ok {
		return nil
	}

	if item.isClear {
		m.focusedRunID = ""
		m.focusedRunRole = ""
	} else {
		m.focusedRunID = strings.TrimSpace(item.runID)
		m.focusedRunRole = strings.TrimSpace(item.role)
		if m.focusedRunRole == "" {
			m.focusedRunRole = shortID(m.focusedRunID)
		}
	}
	m.closeTeamPicker()
	return m.applyFocusLens()
}

func (m *monitorModel) renderTeamPicker(base string) string {
	dims := kit.ComputeModalDims(m.width, m.height, 64, 18, 42, 10, 8, 4)
	m.teamPickerList.SetWidth(dims.ModalWidth - 4)
	m.teamPickerList.SetHeight(dims.ListHeight)

	opts := kit.DefaultPickerModalOpts(m.teamPickerList.View(), m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}
