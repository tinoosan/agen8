package tui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

// modelPickerItem implements list.Item for the model picker.
type modelPickerItem struct {
	id string
}

func (m modelPickerItem) FilterValue() string { return m.id }
func (m modelPickerItem) Title() string       { return m.id }
func (m modelPickerItem) Description() string { return "" }

type modelPickerDelegate struct {
	styleRow lipgloss.Style
	styleSel lipgloss.Style
}

func newModelPickerDelegate() modelPickerDelegate {
	return modelPickerDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		// Avoid background/underline styling (can look like text selection).
		styleSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
	}
}

func (d modelPickerDelegate) Height() int  { return 1 }
func (d modelPickerDelegate) Spacing() int { return 0 }
func (d modelPickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d modelPickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(modelPickerItem)
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

	// Keep line within list width.
	maxW := max(1, m.Width()-lipgloss.Width(prefix))
	line := kit.TruncateRight(it.id, maxW)
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

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

	l := list.New(items, newModelPickerDelegate(), 0, 0)
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
	// Calculate modal dimensions
	maxModalW := max(1, m.width-8)
	modalWidth := min(60, maxModalW)
	minModalW := min(40, maxModalW)
	if modalWidth < minModalW {
		modalWidth = minModalW
	}

	maxModalH := max(1, m.height-8)
	modalHeight := min(20, maxModalH)
	minModalH := min(10, maxModalH)
	if modalHeight < minModalH {
		modalHeight = minModalH
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
