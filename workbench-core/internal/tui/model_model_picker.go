package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/cost"
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
	line := truncateRight(it.id, maxW)
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

// openModelPicker initializes and opens the model picker modal.
func (m *Model) openModelPicker() tea.Cmd {
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
		return turnDoneMsg{final: final, err: err}
	}
}

func (m Model) renderModelPicker(base string) string {
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

	// Style the modal
	modalStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Height(modalHeight).
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6bbcff")).
		Foreground(lipgloss.Color("#eaeaea"))

	modalContent := modalStyle.Render(content)

	// Split modal content into lines
	modalLines := strings.Split(modalContent, "\n")
	modalHeightActual := len(modalLines)
	modalWidthActual := 0
	for _, line := range modalLines {
		if w := lipgloss.Width(line); w > modalWidthActual {
			modalWidthActual = w
		}
	}

	// Calculate centering position
	topPos := (m.height - modalHeightActual) / 2
	if topPos < 0 {
		topPos = 0
	}
	leftPos := (m.width - modalWidthActual) / 2
	if leftPos < 0 {
		leftPos = 0
	}

	// Render over a blank backdrop.
	//
	// We intentionally avoid "overlaying" onto the base UI by slicing strings,
	// because the base view contains ANSI escape codes (and byte-slicing them
	// corrupts styles, causing the kind of weird highlight blocks you reported).
	result := make([]string, m.height)
	for i := 0; i < m.height; i++ {
		result[i] = strings.Repeat(" ", max(1, m.width))

		// Overlay modal lines
		if i >= topPos && i < topPos+modalHeightActual {
			lineIdx := i - topPos
			if lineIdx < len(modalLines) {
				modalLine := modalLines[lineIdx]
				result[i] = strings.Repeat(" ", leftPos) + modalLine
			}
		}
	}

	_ = base
	return strings.Join(result, "\n")
}
