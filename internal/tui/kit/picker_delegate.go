package kit

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type PickerDelegateStyles struct {
	Row lipgloss.Style
	Sel lipgloss.Style
	Dim lipgloss.Style
}

func DefaultPickerDelegateStyles() PickerDelegateStyles {
	return PickerDelegateStyles{
		Row: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		Sel: lipgloss.NewStyle().Foreground(lipgloss.Color("#eaeaea")).Bold(true),
		Dim: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
	}
}

type RenderFunc func(item list.Item, maxWidth int) string

type pickerDelegate struct {
	styles   PickerDelegateStyles
	renderFn RenderFunc
}

func NewPickerDelegate(styles PickerDelegateStyles, renderFn RenderFunc) list.ItemDelegate {
	return pickerDelegate{
		styles:   styles,
		renderFn: renderFn,
	}
}

func (d pickerDelegate) Height() int  { return 1 }
func (d pickerDelegate) Spacing() int { return 0 }
func (d pickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d pickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if item == nil {
		return
	}
	isSel := index == m.Index()
	prefix := "  "
	style := d.styles.Row
	if isSel {
		prefix = "› "
		style = d.styles.Sel
	}

	maxW := maxInt(1, m.Width()-lipgloss.Width(prefix))
	line := ""
	if d.renderFn != nil {
		line = strings.TrimSpace(d.renderFn(item, maxW))
	}
	if line == "" {
		if titled, ok := item.(interface{ Title() string }); ok {
			line = strings.TrimSpace(titled.Title())
		}
	}
	if line == "" {
		line = strings.TrimSpace(item.FilterValue())
	}
	if line == "" {
		line = "(item)"
	}
	line = TruncateRight(line, maxW)

	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}
