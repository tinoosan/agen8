package modelpicker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

type EventType string

const (
	EventNone          EventType = ""
	EventClosed        EventType = "closed"
	EventModelSelected EventType = "model_selected"
	EventError         EventType = "error"
)

type Event struct {
	Type    EventType
	ModelID string
	Err     error
}

type Controller struct {
	endpoint  string
	sessionID string

	open         bool
	loading      bool
	providerView bool
	provider     string
	query        string

	list      list.Model
	lastItems []Item
}

type State struct {
	Open         bool
	Loading      bool
	ProviderView bool
	Provider     string
	Query        string
	List         list.Model
	Items        []Item
}

type Item struct {
	ID         string
	Provider   string
	IsProvider bool
	Count      int
	InputPerM  float64
	OutputPerM float64
}

type listLoadedMsg struct {
	providerView bool
	provider     string
	query        string
	items        []pickerListItem
	err          error
}

type pickerListItem struct {
	id         string
	provider   string
	isProvider bool
	count      int
	inputPerM  float64
	outputPerM float64
}

func (i pickerListItem) FilterValue() string {
	if i.isProvider {
		return strings.TrimSpace(i.provider)
	}
	return strings.TrimSpace(i.id)
}

func (i pickerListItem) Title() string {
	if i.isProvider {
		if i.count > 0 {
			return fmt.Sprintf("%s (%d)", strings.TrimSpace(i.provider), i.count)
		}
		return strings.TrimSpace(i.provider)
	}
	return FormatModelTitle(i.id, i.inputPerM, i.outputPerM)
}

func (i pickerListItem) Description() string { return "" }

func (c *Controller) Open(endpoint, sessionID string) tea.Cmd {
	c.endpoint = strings.TrimSpace(endpoint)
	c.sessionID = strings.TrimSpace(sessionID)
	c.open = true
	c.loading = true
	c.providerView = true
	c.provider = ""
	c.query = ""
	c.lastItems = nil

	l := list.New(nil, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), nil), 0, 0)
	l.Title = "Select Provider"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(false)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)
	c.list = l

	return c.fetchCmd(true, "", "")
}

func (c *Controller) IsOpen() bool { return c.open }

func (c *Controller) State() State {
	return State{
		Open:         c.open,
		Loading:      c.loading,
		ProviderView: c.providerView,
		Provider:     c.provider,
		Query:        c.query,
		List:         c.list,
		Items:        append([]Item(nil), c.lastItems...),
	}
}

func (c *Controller) Close() {
	c.open = false
	c.loading = false
	c.providerView = true
	c.provider = ""
	c.query = ""
	c.lastItems = nil
	c.list = list.Model{}
}

func (c *Controller) SetSize(screenW, screenH int) kit.ModalDims {
	dims := kit.ComputeModalDims(screenW, screenH, 70, 22, 46, 10, 8, 6)
	if !c.open {
		return dims
	}
	// Reserve lines for scope, search, blank, help.
	listH := dims.ListHeight - 4
	if listH < 4 {
		listH = 4
	}
	c.list.SetWidth(dims.ModalWidth - 4)
	c.list.SetHeight(listH)
	return dims
}

func (c *Controller) View() string {
	scope := "Global"
	if !c.providerView {
		scope = "Provider: " + strings.TrimSpace(c.provider)
	}
	searchLine := kit.StyleDim.Render("Search: ") + kit.StyleStatusValue.Render(c.query)
	helpLine := kit.StyleDim.Render("Enter select · Esc back/close · type to search")
	if c.loading {
		return scope + "\n" + searchLine + "\n\n" + "loading…" + "\n" + helpLine
	}
	return scope + "\n" + searchLine + "\n\n" + c.list.View() + "\n" + helpLine
}

func (c *Controller) Update(msg tea.Msg) (tea.Cmd, Event) {
	if !c.open {
		return nil, Event{}
	}

	switch m := msg.(type) {
	case listLoadedMsg:
		if m.err != nil {
			c.loading = false
			return nil, Event{Type: EventError, Err: m.err}
		}
		c.loading = false
		c.providerView = m.providerView
		c.provider = strings.TrimSpace(m.provider)
		c.query = strings.TrimSpace(m.query)
		c.lastItems = make([]Item, 0, len(m.items))
		items := make([]list.Item, 0, len(m.items))
		for _, it := range m.items {
			c.lastItems = append(c.lastItems, Item{
				ID:         it.id,
				Provider:   it.provider,
				IsProvider: it.isProvider,
				Count:      it.count,
				InputPerM:  it.inputPerM,
				OutputPerM: it.outputPerM,
			})
			items = append(items, it)
		}
		if c.providerView {
			c.list.Title = "Select Provider"
		} else {
			c.list.Title = "Select Model (" + c.provider + ")"
		}
		c.list.SetItems(items)
		if len(items) > 0 {
			c.list.Select(0)
		}
		return nil, Event{}
		case tea.KeyMsg:
			s := strings.ToLower(m.String())
		switch s {
		case "esc", "escape":
			if !c.providerView {
				c.loading = true
				c.providerView = true
				c.provider = ""
				c.query = ""
				return c.fetchCmd(true, "", ""), Event{}
			}
			c.Close()
			return nil, Event{Type: EventClosed}
		case "backspace":
			if c.query != "" {
				r := []rune(c.query)
				c.query = string(r[:len(r)-1])
				c.loading = true
				return c.fetchCmd(c.providerView, c.provider, c.query), Event{}
			}
			return nil, Event{}
		case "enter":
			if len(c.lastItems) == 0 || c.list.Items() == nil || len(c.list.Items()) == 0 {
				return nil, Event{}
			}
			idx := c.list.Index()
			if idx < 0 || idx >= len(c.lastItems) {
				return nil, Event{}
			}
			selected := c.lastItems[idx]
			if selected.IsProvider {
				c.loading = true
				c.providerView = false
				c.provider = strings.TrimSpace(selected.Provider)
				c.query = ""
				return c.fetchCmd(false, c.provider, ""), Event{}
			}
			c.Close()
			return nil, Event{Type: EventModelSelected, ModelID: strings.TrimSpace(selected.ID)}
		}

			if len(m.Runes) > 0 {
				for _, r := range m.Runes {
				if r >= 32 && r != 127 {
					c.query += string(r)
				}
			}
			c.loading = true
			return c.fetchCmd(c.providerView, c.provider, c.query), Event{}
		}

		var cmd tea.Cmd
			c.list, cmd = c.list.Update(m)
		return cmd, Event{}
	}
	return nil, Event{}
}

func (c *Controller) fetchCmd(providerView bool, provider, query string) tea.Cmd {
	endpoint := strings.TrimSpace(c.endpoint)
	sessionID := strings.TrimSpace(c.sessionID)
	provider = strings.TrimSpace(provider)
	query = strings.TrimSpace(query)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := Fetch(ctx, endpoint, sessionID, provider, query)
		if err != nil {
			return listLoadedMsg{
				providerView: providerView,
				provider:     provider,
				query:        query,
				err:          err,
			}
		}
		if providerView {
			items := make([]pickerListItem, 0, len(res.Providers))
			for _, p := range res.Providers {
				items = append(items, pickerListItem{
					provider:   strings.TrimSpace(p.Name),
					isProvider: true,
					count:      p.Count,
				})
			}
			return listLoadedMsg{
				providerView: true,
				query:        query,
				items:        items,
			}
		}
		items := make([]pickerListItem, 0, len(res.Models))
		for _, m := range res.Models {
			if !strings.EqualFold(strings.TrimSpace(m.Provider), provider) {
				continue
			}
			items = append(items, pickerListItem{
				id:         strings.TrimSpace(m.ID),
				provider:   strings.TrimSpace(m.Provider),
				inputPerM:  m.InputPerM,
				outputPerM: m.OutputPerM,
			})
		}
		return listLoadedMsg{
			providerView: false,
			provider:     provider,
			query:        query,
			items:        items,
		}
	}
}
