package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

type activityItem struct {
	act Activity
}

func (a activityItem) Title() string {
	// Keep title compact; status icon is rendered by the delegate.
	return a.act.Title
}

func (a activityItem) Description() string {
	// Show tool id/action for tool.run; otherwise keep empty.
	if a.act.Kind == "tool.run" {
		id := strings.TrimSpace(a.act.ToolID)
		act := strings.TrimSpace(a.act.ActionID)
		if id != "" && act != "" {
			return fmt.Sprintf("%s/%s", id, act)
		}
		if id != "" {
			return id
		}
	}
	return ""
}

func (a activityItem) FilterValue() string { return a.act.Title }

type activityDelegate struct {
	styleRow    lipgloss.Style
	styleSel    lipgloss.Style
	styleDim    lipgloss.Style
	styleStatus lipgloss.Style
}

func newActivityDelegate() activityDelegate {
	return activityDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		styleSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Background(lipgloss.Color("#303030")),
		styleDim:    lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		styleStatus: lipgloss.NewStyle().Foreground(lipgloss.Color("#9ad0ff")),
	}
}

func (d activityDelegate) Height() int  { return 2 }
func (d activityDelegate) Spacing() int { return 1 }
func (d activityDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d activityDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(activityItem)
	if !ok {
		return
	}

	isSel := index == m.Index()
	rowStyle := d.styleRow
	if isSel {
		rowStyle = d.styleSel
	}

	prefix := "  "
	if isSel {
		prefix = "› "
	}

	status := it.act.ShortStatus()
	kind := strings.TrimSpace(it.act.Kind)
	if kind == "" {
		kind = "op"
	}

	title := kit.TruncateRight(it.act.Title, max(1, m.Width()-6))
	line1 := fmt.Sprintf("%s%s %s", prefix, d.styleStatus.Render(status), rowStyle.Render(title))

	desc := strings.TrimSpace(it.Description())
	meta := []string{}
	if kind != "" {
		meta = append(meta, kind)
	}
	if desc != "" {
		meta = append(meta, desc)
	}
	if it.act.Duration > 0 {
		meta = append(meta, it.act.Duration.String())
	}
	line2 := d.styleDim.Render("    " + strings.Join(meta, " • "))

	_, _ = fmt.Fprint(w, line1)
	_, _ = fmt.Fprint(w, "\n")
	_, _ = fmt.Fprint(w, rowStyle.Render(kit.TruncateRight(line2, max(1, m.Width()-2))))
}
