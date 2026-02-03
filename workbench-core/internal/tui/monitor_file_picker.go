package tui

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
)

// monitorFilePickerItem implements list.Item for the file picker.
type monitorFilePickerItem struct {
	rel string // workdir-relative, slash-separated
}

func (f monitorFilePickerItem) FilterValue() string { return f.rel }
func (f monitorFilePickerItem) Title() string       { return f.rel }
func (f monitorFilePickerItem) Description() string { return "" }

type monitorFilePickerDelegate struct {
	styleRow lipgloss.Style
	styleSel lipgloss.Style
}

func newMonitorFilePickerDelegate() monitorFilePickerDelegate {
	return monitorFilePickerDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		styleSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
	}
}

func (d monitorFilePickerDelegate) Height() int  { return 1 }
func (d monitorFilePickerDelegate) Spacing() int { return 0 }
func (d monitorFilePickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d monitorFilePickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(monitorFilePickerItem)
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
	line := kit.TruncateRight(it.rel, maxW)
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

// openFilePicker initializes and opens the file picker modal.
func (m *monitorModel) openFilePicker(query string) tea.Cmd {
	m.filePickerOpen = true
	m.filePickerQuery = strings.TrimSpace(query)

	l := list.New([]list.Item{}, newMonitorFilePickerDelegate(), 0, 0)
	l.Title = "Select File"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(false)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)
	m.filePickerList = l

	// Scan files in background
	return m.scanFilesForPicker()
}

// closeFilePicker closes the file picker modal.
func (m *monitorModel) closeFilePicker() {
	m.filePickerOpen = false
	m.filePickerList = list.Model{}
	m.filePickerAllPaths = nil
	m.filePickerQuery = ""
}

// scanFilesForPicker scans the workspace directory for files.
func (m *monitorModel) scanFilesForPicker() tea.Cmd {
	return func() tea.Msg {
		wsDir := fsutil.GetWorkspaceDir(m.cfg.DataDir, m.runID)
		paths, _ := scanMonitorWorkdirFiles(wsDir, 5000)
		return monitorFilePickerPathsMsg{paths: paths}
	}
}

type monitorFilePickerPathsMsg struct {
	paths []string
}

func scanMonitorWorkdirFiles(baseDir string, maxVisited int) ([]string, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, nil
	}
	if maxVisited <= 0 {
		maxVisited = 5000
	}

	paths := make([]string, 0, 256)
	visited := 0
	err := filepath.WalkDir(baseDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != baseDir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}

		visited++
		if maxVisited > 0 && visited > maxVisited {
			return fs.SkipAll
		}

		rel, err := filepath.Rel(baseDir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		rel = strings.TrimSpace(rel)
		if rel == "" || rel == "." {
			return nil
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

// updateFilePicker handles keyboard input when the file picker is open.
func (m *monitorModel) updateFilePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "escape":
		m.closeFilePicker()
		return m, nil
	case "enter":
		return m, m.selectFileFromPicker()
	}

	var cmd tea.Cmd
	m.filePickerList, cmd = m.filePickerList.Update(msg)
	return m, cmd
}

// handleFilePickerPaths updates the file picker with scanned paths.
func (m *monitorModel) handleFilePickerPaths(paths []string) {
	m.filePickerAllPaths = paths
	m.applyFilePickerFilter()
}

// applyFilePickerFilter filters the file list based on the query.
func (m *monitorModel) applyFilePickerFilter() {
	needle := strings.ToLower(m.filePickerQuery)
	items := make([]list.Item, 0, 200)
	for _, rel := range m.filePickerAllPaths {
		if needle == "" || strings.Contains(strings.ToLower(rel), needle) {
			items = append(items, monitorFilePickerItem{rel: rel})
			if len(items) >= 500 {
				break
			}
		}
	}
	m.filePickerList.SetItems(items)
	if len(items) > 0 {
		m.filePickerList.Select(0)
	}
}

// selectFileFromPicker inserts the selected file path into the composer.
func (m *monitorModel) selectFileFromPicker() tea.Cmd {
	if m.filePickerList.Items() == nil || len(m.filePickerList.Items()) == 0 {
		return nil
	}
	selected := m.filePickerList.SelectedItem()
	it, ok := selected.(monitorFilePickerItem)
	if !ok {
		m.closeFilePicker()
		return nil
	}

	// Insert file reference into input
	current := m.input.Value()
	// Remove the @ trigger if present at the end
	current = strings.TrimSuffix(strings.TrimSpace(current), "@")
	current = strings.TrimSpace(current)
	if current != "" {
		current += " "
	}
	current += "@" + it.rel + " "
	m.input.SetValue(current)

	m.closeFilePicker()
	return nil
}

func (m *monitorModel) renderFilePicker(base string) string {
	modalWidth := 80
	if modalWidth > m.width-8 {
		modalWidth = m.width - 8
	}
	if modalWidth < 40 {
		modalWidth = 40
	}
	modalHeight := 22
	if modalHeight > m.height-8 {
		modalHeight = m.height - 8
	}
	if modalHeight < 10 {
		modalHeight = 10
	}

	listHeight := modalHeight - 2
	if listHeight < 4 {
		listHeight = 4
	}
	m.filePickerList.SetWidth(modalWidth - 4)
	m.filePickerList.SetHeight(listHeight)

	title := "Select File"
	if m.filePickerQuery != "" {
		title += " (@" + kit.TruncateMiddle(m.filePickerQuery, 32) + ")"
	}
	m.filePickerList.Title = title

	content := m.filePickerList.View()

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
