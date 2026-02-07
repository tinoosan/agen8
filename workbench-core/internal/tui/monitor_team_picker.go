package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
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

type teamPickerDelegate struct {
	styleRow lipgloss.Style
	styleSel lipgloss.Style
	styleDim lipgloss.Style
}

func newTeamPickerDelegate() teamPickerDelegate {
	return teamPickerDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		styleSel: lipgloss.NewStyle().Foreground(lipgloss.Color("#eaeaea")).Bold(true),
		styleDim: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
	}
}

func (d teamPickerDelegate) Height() int  { return 1 }
func (d teamPickerDelegate) Spacing() int { return 0 }
func (d teamPickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d teamPickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(teamPickerItem)
	if !ok {
		return
	}

	isSel := index == m.Index()
	prefix := "  "
	style := d.styleRow
	if isSel {
		prefix = "› "
		style = d.styleSel
	}

	line := it.Title()
	if desc := strings.TrimSpace(it.Description()); desc != "" {
		line += " — " + desc
	}
	maxW := max(1, m.Width()-lipgloss.Width(prefix))
	line = kit.TruncateRight(line, maxW)
	if it.isClear && !isSel {
		line = d.styleDim.Render(line)
	}
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

func (m *monitorModel) openTeamPicker() tea.Cmd {
	m.closeHelpModal()
	if m.sessionPickerOpen {
		m.closeSessionPicker()
	}
	if m.profilePickerOpen {
		m.closeProfilePicker()
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

	items := []teamPickerItem{{isClear: true}}
	seen := map[string]struct{}{}
	runIDs := append([]string(nil), m.teamRunIDs...)
	for runID := range m.teamRoleByRunID {
		runIDs = append(runIDs, runID)
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

	l := list.New(listItems, newTeamPickerDelegate(), 0, 0)
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
	maxModalW := max(1, m.width-8)
	modalWidth := min(64, maxModalW)
	minModalW := min(42, maxModalW)
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
	m.teamPickerList.SetWidth(modalWidth - 4)
	m.teamPickerList.SetHeight(listHeight)

	opts := kit.ModalOptions{
		Content:      m.teamPickerList.View(),
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
