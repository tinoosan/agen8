package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/cost"
)

// modelPickerItem implements list.Item for the model picker.
type modelPickerItem struct {
	id string
}

func (m modelPickerItem) FilterValue() string { return m.id }
func (m modelPickerItem) Title() string       { return m.id }
func (m modelPickerItem) Description() string { return "" }

// openModelPicker initializes and opens the model picker modal.
func (m *Model) openModelPicker() tea.Cmd {
	if m.runtimeChangeLocked("changing model") {
		return nil
	}
	m.modelPickerOpen = true

	ids := cost.SupportedModels()
	items := make([]list.Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, modelPickerItem{id: id})
	}

	l := list.New(items, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), nil), 0, 0)
	l.Title = "Select Model"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(true)
	l.SetShowFilter(true)
	// Ensure items are visible immediately (VisibleItems uses filteredItems when filterState != Unfiltered).
	// Then put the list into Filtering mode so typing edits the filter input.
	l.SetFilterText("")
	l.SetFilterState(list.Filtering)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	if len(items) > 0 {
		l.Select(0)
	}

	m.modelPickerList = l
	m.layout()
	return nil
}

// closeModelPicker closes the model picker modal.
func (m *Model) closeModelPicker() {
	m.modelPickerOpen = false
	m.modelPickerList = list.Model{}
}

// selectModelFromPicker selects the currently highlighted model and triggers the /model command.
func (m *Model) selectModelFromPicker() tea.Cmd {
	if m.modelPickerList.Items() == nil || len(m.modelPickerList.Items()) == 0 {
		return nil
	}
	selectedItem := m.modelPickerList.SelectedItem()
	if selectedItem == nil {
		return nil
	}
	item, ok := selectedItem.(modelPickerItem)
	if !ok {
		return nil
	}

	selectedID := item.id

	// Optimistically update the model ID so the label updates instantly
	m.modelID = selectedID

	// Close the picker
	m.closeModelPicker()

	// Trigger the host command to persist the change and show transcript message
	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, "/model "+selectedID)
		return turnDoneMsg{final: final, err: err, preserveScroll: true}
	}
}

func (m Model) renderModelPicker(base string) string {
	dims := kit.ComputeModalDims(m.width, m.height, 60, 20, 40, 10, 8, 3)
	m.modelPickerList.SetWidth(dims.ModalWidth - 4)
	m.modelPickerList.SetHeight(dims.ListHeight)

	// Build modal content
	content := m.modelPickerList.View()

	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}
