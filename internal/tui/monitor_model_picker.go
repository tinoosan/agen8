package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/protocol"
)

// modelPickerItem implements list.Item for the model picker.
type monitorModelPickerItem struct {
	id         string
	provider   string
	isProvider bool
	count      int
	inputPerM  float64
	outputPerM float64
}

func (m monitorModelPickerItem) FilterValue() string {
	if m.isProvider {
		return m.provider
	}
	return m.id
}

func (m monitorModelPickerItem) Title() string {
	if m.isProvider {
		if m.count > 0 {
			return fmt.Sprintf("%s (%d)", m.provider, m.count)
		}
		return m.provider
	}
	return m.id
}

// openModelPicker initializes and opens the provider-first model picker modal.
func (m *monitorModel) openModelPicker() tea.Cmd {
	m.helpModalOpen = false
	m.closeAllPickers()
	m.modelPickerOpen = true
	m.modelPickerProvider = ""
	m.modelPickerQuery = ""
	m.modelPickerProviderView = true

	l := list.New([]list.Item{}, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), nil), 0, 0)
	l.Title = "Select Provider"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(false)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	m.modelPickerList = l
	m.refreshModelPickerItems()
	return nil
}

// closeModelPicker closes the model picker modal.
func (m *monitorModel) closeModelPicker() {
	m.modelPickerOpen = false
	m.modelPickerList = list.Model{}
	m.modelPickerProvider = ""
	m.modelPickerQuery = ""
	m.modelPickerProviderView = false
}

func (m *monitorModel) refreshModelPickerItems() {
	if !m.modelPickerOpen {
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.modelPickerQuery))
	items := make([]list.Item, 0, 64)

	if m.modelPickerProviderView {
		var res protocol.ModelListResult
		_ = m.rpcRoundTrip(protocol.MethodModelList, protocol.ModelListParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Query:    strings.TrimSpace(m.modelPickerQuery),
		}, &res)
		for _, p := range res.Providers {
			candidate := strings.ToLower(strings.TrimSpace(p.Name))
			if q != "" && !strings.Contains(candidate, q) {
				continue
			}
			items = append(items, monitorModelPickerItem{provider: strings.TrimSpace(p.Name), isProvider: true, count: p.Count})
		}
		m.modelPickerList.Title = "Select Provider"
	} else {
		provider := strings.TrimSpace(m.modelPickerProvider)
		var res protocol.ModelListResult
		_ = m.rpcRoundTrip(protocol.MethodModelList, protocol.ModelListParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Provider: provider,
			Query:    strings.TrimSpace(m.modelPickerQuery),
		}, &res)
		for _, info := range res.Models {
			if !strings.EqualFold(strings.TrimSpace(info.Provider), provider) {
				continue
			}
			candidate := strings.ToLower(strings.TrimSpace(info.ID))
			if q != "" && !strings.Contains(candidate, q) {
				continue
			}
			items = append(items, monitorModelPickerItem{
				id:         strings.TrimSpace(info.ID),
				provider:   provider,
				inputPerM:  info.InputPerM,
				outputPerM: info.OutputPerM,
			})
		}
		m.modelPickerList.Title = "Select Model (" + provider + ")"
	}

	m.modelPickerList.SetItems(items)
	if len(items) > 0 {
		m.modelPickerList.Select(0)
	}
}

// updateModelPicker handles keyboard input when the model picker is open.
func (m *monitorModel) updateModelPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := strings.ToLower(msg.String())
	switch s {
	case "esc", "escape":
		if !m.modelPickerProviderView {
			m.modelPickerProviderView = true
			m.modelPickerProvider = ""
			m.modelPickerQuery = ""
			m.refreshModelPickerItems()
			return m, nil
		}
		m.closeModelPicker()
		return m, nil
	case "backspace":
		if m.modelPickerQuery != "" {
			r := []rune(m.modelPickerQuery)
			m.modelPickerQuery = string(r[:len(r)-1])
			m.refreshModelPickerItems()
			return m, nil
		}
	case "enter":
		return m, m.selectModelFromPicker()
	}

	if len(msg.Runes) > 0 {
		for _, r := range msg.Runes {
			if r >= 32 && r != 127 {
				m.modelPickerQuery += string(r)
			}
		}
		m.refreshModelPickerItems()
		return m, nil
	}

	var cmd tea.Cmd
	m.modelPickerList, cmd = m.modelPickerList.Update(msg)
	return m, cmd
}

// selectModelFromPicker selects the currently highlighted provider/model.
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

	if item.isProvider {
		m.modelPickerProviderView = false
		m.modelPickerProvider = strings.TrimSpace(item.provider)
		m.modelPickerQuery = ""
		m.refreshModelPickerItems()
		return nil
	}

	selectedID := item.id
	m.model = selectedID
	m.closeModelPicker()
	if m.teamID != "" {
		return m.writeTeamControl("set_team_model", selectedID)
	}
	return m.writeControl("set_model", map[string]any{"model": selectedID})
}

func (m *monitorModel) renderModelPicker(base string) string {
	dims := kit.ComputeModalDims(m.width, m.height, 70, 22, 46, 10, 8, 6)
	m.modelPickerList.SetWidth(dims.ModalWidth - 4)
	m.modelPickerList.SetHeight(dims.ListHeight)

	scope := "Global"
	if !m.modelPickerProviderView {
		scope = "Provider: " + strings.TrimSpace(m.modelPickerProvider)
	}
	searchLine := kit.StyleDim.Render("Search: ") + kit.StyleStatusValue.Render(fallback(m.modelPickerQuery, ""))
	helpLine := kit.StyleDim.Render("Enter select · Esc back/close · type to search")
	content := scope + "\n" + searchLine + "\n\n" + m.modelPickerList.View() + "\n" + helpLine

	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}

func (m monitorModelPickerItem) Description() string {
	if m.isProvider {
		return ""
	}
	return "in $" + formatPricePerM(m.inputPerM) + "/M · out $" + formatPricePerM(m.outputPerM) + "/M"
}

func formatPricePerM(v float64) string {
	if v <= 0 {
		return "0"
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.4f", v), "0"), ".")
}
