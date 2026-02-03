package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/cost"
)

// modelPickerItem implements list.Item for the model picker.
type monitorModelPickerItem struct {
	id string
}

func (m monitorModelPickerItem) FilterValue() string { return m.id }
func (m monitorModelPickerItem) Title() string       { return m.id }
func (m monitorModelPickerItem) Description() string { return "" }

type monitorModelPickerDelegate struct {
	styleRow lipgloss.Style
	styleSel lipgloss.Style
}

func newMonitorModelPickerDelegate() monitorModelPickerDelegate {
	return monitorModelPickerDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		styleSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
	}
}

func (d monitorModelPickerDelegate) Height() int  { return 1 }
func (d monitorModelPickerDelegate) Spacing() int { return 0 }
func (d monitorModelPickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d monitorModelPickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(monitorModelPickerItem)
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

	maxW := max(1, m.Width()-lipgloss.Width(prefix))
	line := kit.TruncateRight(it.id, maxW)
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

// openModelPicker initializes and opens the model picker modal.
func (m *monitorModel) openModelPicker() tea.Cmd {
	m.modelPickerOpen = true

	ids := cost.SupportedModels()
	items := make([]list.Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, monitorModelPickerItem{id: id})
	}

	l := list.New(items, newMonitorModelPickerDelegate(), 0, 0)
	l.Title = "Select Model"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	l.SetFilterText("")
	l.SetFilterState(list.Filtering)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	if len(items) > 0 {
		l.Select(0)
	}

	m.modelPickerList = l
	return nil
}

// closeModelPicker closes the model picker modal.
func (m *monitorModel) closeModelPicker() {
	m.modelPickerOpen = false
	m.modelPickerList = list.Model{}
}

// updateModelPicker handles keyboard input when the model picker is open.
func (m *monitorModel) updateModelPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "escape":
		m.closeModelPicker()
		return m, nil
	case "enter":
		return m, m.selectModelFromPicker()
	}

	var cmd tea.Cmd
	m.modelPickerList, cmd = m.modelPickerList.Update(msg)
	return m, cmd
}

// selectModelFromPicker selects the currently highlighted model and writes a control file.
func (m *monitorModel) selectModelFromPicker() tea.Cmd {
	if m.modelPickerList.Items() == nil || len(m.modelPickerList.Items()) == 0 {
		return nil
	}
	selectedItem := m.modelPickerList.SelectedItem()
	if selectedItem == nil {
		return nil
	}
	item, ok := selectedItem.(monitorModelPickerItem)
	if !ok {
		return nil
	}

	selectedID := item.id

	// Optimistically update the model label
	m.model = selectedID

	// Close the picker
	m.closeModelPicker()

	// Write control file to inbox
	return m.writeControl("set_model", map[string]any{"model": selectedID})
}

func (m *monitorModel) renderModelPicker(base string) string {
	// Calculate modal dimensions
	modalWidth := 60
	if modalWidth > m.width-8 {
		modalWidth = m.width - 8
	}
	if modalWidth < 40 {
		modalWidth = 40
	}
	modalHeight := 20
	if modalHeight > m.height-8 {
		modalHeight = m.height - 8
	}
	if modalHeight < 10 {
		modalHeight = 10
	}

	// Size the list to fit within the modal
	listHeight := modalHeight - 3 // Account for filter input and borders
	if listHeight < 4 {
		listHeight = 4
	}
	m.modelPickerList.SetWidth(modalWidth - 4) // Account for padding/borders
	m.modelPickerList.SetHeight(listHeight)

	// Build modal content
	content := m.modelPickerList.View()

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
