package kit

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type PickerActionType int

const (
	PickerActionNone PickerActionType = iota
	PickerActionAccept
	PickerActionClose
	PickerActionPageNext
	PickerActionPagePrev
	PickerActionFilterChanged
)

type PickerConfig struct {
	Title             string
	FilteringEnabled  bool
	ShowFilter        bool
	ShowPagination    bool
	ShowStatusBar     bool
	PageSize          int
	PageKeyNav        bool
	CursorPageKeyNav  bool
	Delegate          list.ItemDelegate
	ModalWidth        int
	ModalHeight       int
	ModalMinWidth     int
	ModalMinHeight    int
	ModalMarginX      int
	ModalMarginY      int
}

type PickerState struct {
	Open     bool
	List     list.Model
	Filter   string
	Page     int
	PageSize int
	Total    int
}

type PickerAction struct {
	Type   PickerActionType
	Cmd    tea.Cmd
	Filter string
}

type PickerController struct {
	config PickerConfig
	state  PickerState
}

func (c *PickerController) Open(config PickerConfig) {
	c.config = withPickerDefaults(config)
	c.state = PickerState{
		Open:     true,
		Filter:   "",
		Page:     0,
		PageSize: c.config.PageSize,
		Total:    0,
		List:     newPickerList(c.config),
	}
}

func (c *PickerController) Close() {
	c.state = PickerState{}
}

func (c *PickerController) IsOpen() bool {
	return c.state.Open
}

func (c *PickerController) State() PickerState {
	return c.state
}

func (c *PickerController) SelectedItem() list.Item {
	if !c.state.Open || c.state.List.Items() == nil || len(c.state.List.Items()) == 0 {
		return nil
	}
	return c.state.List.SelectedItem()
}

func (c *PickerController) SetItems(items []list.Item) {
	c.state.List.SetItems(items)
	if len(items) > 0 {
		c.state.List.Select(0)
	}
}

func (c *PickerController) Select(index int) {
	if index < 0 {
		index = 0
	}
	c.state.List.Select(index)
}

func (c *PickerController) SetLoadingTitle(title string) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = c.config.Title
	}
	c.state.List.Title = title
}

func (c *PickerController) SetTitle(title string) {
	c.SetLoadingTitle(title)
}

func (c *PickerController) SetFilter(filter string) {
	c.state.Filter = strings.TrimSpace(filter)
	if !c.state.List.FilteringEnabled() {
		return
	}
	c.state.List.SetFilterText(c.state.Filter)
	if c.state.Filter == "" {
		c.state.List.SetFilterState(list.Unfiltered)
		return
	}
	c.state.List.SetFilterState(list.Filtering)
}

func (c *PickerController) SetPage(page, total, pageSize int) {
	c.state.Page = max(page, 0)
	c.state.Total = max(total, 0)
	if pageSize > 0 {
		c.state.PageSize = pageSize
	}
}

func (c *PickerController) Update(msg tea.Msg) PickerAction {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.Type {
		case tea.KeyEsc:
			return PickerAction{Type: PickerActionClose}
		case tea.KeyEnter:
			return PickerAction{Type: PickerActionAccept}
		case tea.KeyCtrlN, tea.KeyPgDown:
			if c.config.PageKeyNav {
				return PickerAction{Type: PickerActionPageNext}
			}
			if c.config.CursorPageKeyNav {
				c.state.List.CursorDown()
				return PickerAction{}
			}
		case tea.KeyCtrlP, tea.KeyPgUp:
			if c.config.PageKeyNav {
				return PickerAction{Type: PickerActionPagePrev}
			}
			if c.config.CursorPageKeyNav {
				c.state.List.CursorUp()
				return PickerAction{}
			}
		case tea.KeyCtrlU:
			if c.config.CursorPageKeyNav {
				c.state.List.CursorUp()
				return PickerAction{}
			}
		case tea.KeyCtrlF:
			if c.config.CursorPageKeyNav {
				c.state.List.CursorDown()
				return PickerAction{}
			}
		case tea.KeyUp:
			c.state.List.CursorUp()
			return PickerAction{}
		case tea.KeyDown:
			c.state.List.CursorDown()
			return PickerAction{}
		}
	}

	var cmd tea.Cmd
	c.state.List, cmd = c.state.List.Update(msg)
	if c.state.List.FilteringEnabled() {
		nextFilter := strings.TrimSpace(c.state.List.FilterInput.Value())
		if nextFilter != c.state.Filter {
			c.state.Filter = nextFilter
			return PickerAction{Type: PickerActionFilterChanged, Cmd: cmd, Filter: nextFilter}
		}
	}
	return PickerAction{Cmd: cmd}
}

func (c *PickerController) Render(screenW, screenH int, footer string, err string) string {
	dims := ComputeModalDims(
		screenW,
		screenH,
		c.config.ModalWidth,
		c.config.ModalHeight,
		c.config.ModalMinWidth,
		c.config.ModalMinHeight,
		c.config.ModalMarginX,
		c.config.ModalMarginY,
	)
	c.state.List.SetWidth(dims.ModalWidth - 4)
	c.state.List.SetHeight(dims.ListHeight)

	content := c.state.List.View()
	if strings.TrimSpace(err) != "" {
		errLine := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8080")).Render("Error: " + err)
		content = errLine + "\n\n" + content
	}
	if strings.TrimSpace(footer) != "" {
		content += "\n" + footer
	}
	return RenderOverlay(DefaultPickerModalOpts(content, screenW, screenH, dims.ModalWidth, dims.ModalHeight))
}

func newPickerList(config PickerConfig) list.Model {
	l := list.New(nil, config.Delegate, 0, 0)
	l.Title = config.Title
	l.SetShowHelp(false)
	l.SetShowStatusBar(config.ShowStatusBar)
	l.SetShowPagination(config.ShowPagination)
	l.SetFilteringEnabled(config.FilteringEnabled)
	l.SetShowFilter(config.ShowFilter)
	l.SetFilterText("")
	l.SetFilterState(list.Unfiltered)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)
	return l
}

func withPickerDefaults(config PickerConfig) PickerConfig {
	if config.Title == "" {
		config.Title = "Select"
	}
	if config.PageSize <= 0 {
		config.PageSize = 50
	}
	if config.Delegate == nil {
		config.Delegate = NewPickerDelegate(DefaultPickerDelegateStyles(), nil)
	}
	if config.ModalWidth <= 0 {
		config.ModalWidth = 80
	}
	if config.ModalHeight <= 0 {
		config.ModalHeight = 22
	}
	if config.ModalMinWidth <= 0 {
		config.ModalMinWidth = 40
	}
	if config.ModalMinHeight <= 0 {
		config.ModalMinHeight = 10
	}
	if config.ModalMarginX <= 0 {
		config.ModalMarginX = 8
	}
	if config.ModalMarginY <= 0 {
		config.ModalMarginY = 4
	}
	return config
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
