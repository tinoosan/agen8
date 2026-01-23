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
	"github.com/tinoosan/workbench-core/internal/atref"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
)

// filePickerItem implements list.Item for the file picker.
type filePickerItem struct {
	rel string // workdir-relative, slash-separated
}

func (f filePickerItem) FilterValue() string { return f.rel }
func (f filePickerItem) Title() string       { return f.rel }
func (f filePickerItem) Description() string { return "" }

type filePickerDelegate struct {
	styleRow lipgloss.Style
	styleSel lipgloss.Style
}

func newFilePickerDelegate() filePickerDelegate {
	return filePickerDelegate{
		styleRow: lipgloss.NewStyle().Foreground(lipgloss.Color("#b0b0b0")),
		// Avoid background/underline styling (can look like text selection).
		styleSel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
	}
}

func (d filePickerDelegate) Height() int  { return 1 }
func (d filePickerDelegate) Spacing() int { return 0 }
func (d filePickerDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd {
	return nil
}

func (d filePickerDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	it, ok := item.(filePickerItem)
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
	line := kit.TruncateRight(it.rel, maxW)
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

func scanWorkdirFiles(baseDir string, maxVisited int) ([]string, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, nil
	}
	if maxVisited <= 0 {
		maxVisited = 10000
	}

	paths := make([]string, 0, 256)
	visited := 0
	err := filepath.WalkDir(baseDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip hidden directories to keep the list bounded and less noisy.
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

func (m *Model) activeWorkspaceDir() string {
	if m == nil {
		return ""
	}
	dd := strings.TrimSpace(m.dataDir)
	rid := strings.TrimSpace(m.runID)
	if dd == "" || rid == "" {
		return ""
	}
	return fsutil.GetScratchDir(dd, rid)
}

func (m *Model) scanFilePickerPaths(workdir string) ([]string, error) {
	workdir = strings.TrimSpace(workdir)
	if workdir == "" {
		return nil, nil
	}
	workdirAll, err := scanWorkdirFiles(workdir, 10000)
	if err != nil {
		return nil, err
	}
	wsDir := strings.TrimSpace(m.activeWorkspaceDir())
	workspaceAll := []string(nil)
	if wsDir != "" {
		if all, err := scanWorkdirFiles(wsDir, 10000); err == nil {
			workspaceAll = all
		}
	}
	combined := make([]string, 0, len(workdirAll)+len(workspaceAll))
	combined = append(combined, workdirAll...)
	for _, rel := range workspaceAll {
		rel = strings.TrimSpace(rel)
		if rel == "" || rel == "." {
			continue
		}
		combined = append(combined, "/scratch/"+rel)
	}
	return combined, nil
}

func (m *Model) openFilePicker(initialQuery string) tea.Cmd {
	m.filePickerOpen = true

	l := list.New([]list.Item{}, newFilePickerDelegate(), 0, 0)
	l.Title = "Select File"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	// Important: we do substring filtering ourselves from the input's @token.
	l.SetFilteringEnabled(false)
	l.SetShowFilter(false)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)
	m.filePickerList = l

	m.filePickerAllPaths = nil
	m.filePickerWorkdir = ""
	m.applyFilePickerQuery(initialQuery) // ok even if empty list

	wd := strings.TrimSpace(m.workdir)
	if wd != "" {
		if all, err := m.scanFilePickerPaths(wd); err == nil {
			m.filePickerAllPaths = all
			m.filePickerWorkdir = wd
			m.applyFilePickerQuery(initialQuery)
		}
		m.layout()
		return nil
	}

	// Workdir not known yet (can happen before first turn). Prefetch it without
	// creating a run by calling the host /pwd command.
	m.filePickerList.Title = "Select File (loading workdir…)"
	m.layout()
	return func() tea.Msg {
		wd, err := m.runner.RunTurn(m.ctx, "/pwd")
		return workdirPrefetchMsg{workdir: strings.TrimSpace(wd), err: err}
	}
}

func (m *Model) closeFilePicker() {
	m.filePickerOpen = false
	m.filePickerList = list.Model{}
	m.filePickerAllPaths = nil
	m.filePickerQuery = ""
	m.filePickerWorkdir = ""
}

func (m *Model) applyFilePickerQuery(q string) {
	q = strings.TrimSpace(q)
	m.filePickerQuery = q

	// Substring match (case-insensitive) over rel paths.
	needle := strings.ToLower(q)
	items := make([]list.Item, 0, 200)
	for _, rel := range m.filePickerAllPaths {
		if needle == "" || strings.Contains(strings.ToLower(rel), needle) {
			items = append(items, filePickerItem{rel: rel})
			if len(items) >= 500 {
				break
			}
		}
	}
	m.filePickerList.SetItems(items)
	if len(items) > 0 {
		m.filePickerList.Select(0)
	}

	// Surface the active query since the underlying input is hidden by the modal.
	title := "Select File"
	if strings.TrimSpace(m.filePickerQuery) != "" {
		title += " (@" + kit.TruncateMiddle(m.filePickerQuery, 32) + ")"
	}
	if strings.Contains(m.filePickerList.Title, "loading") {
		// Preserve loading prefix if still loading.
		if strings.TrimSpace(m.filePickerWorkdir) == "" {
			m.filePickerList.Title = "Select File (loading workdir…) (@" + kit.TruncateMiddle(m.filePickerQuery, 32) + ")"
			return
		}
	}
	m.filePickerList.Title = title
}

func isEditorCommand(input string) bool {
	fields := strings.Fields(strings.TrimSpace(input))
	return len(fields) > 0 && fields[0] == "/editor"
}

func (m *Model) syncFilePickerFromInput() tea.Cmd {
	input := m.currentInputValue()
	q, _, _, ok := atref.ActiveAtTokenAtEnd(input)
	if !ok {
		if m.filePickerOpen {
			m.closeFilePicker()
			m.layout()
		}
		return nil
	}
	if !m.filePickerOpen {
		return m.openFilePicker(q)
	}
	m.applyFilePickerQuery(q)
	return nil
}

func (m *Model) selectFileFromPicker() tea.Cmd {
	if m.filePickerList.Items() == nil || len(m.filePickerList.Items()) == 0 {
		return nil
	}
	selected := m.filePickerList.SelectedItem()
	it, ok := selected.(filePickerItem)
	if !ok {
		return nil
	}
	input := m.currentInputValue()
	_, start, end, ok := atref.ActiveAtTokenAtEnd(input)
	if !ok {
		// Token was removed; just close.
		m.closeFilePicker()
		m.layout()
		return nil
	}

	repl := atref.FormatAtRef(it.rel)
	// Add a trailing space so the token is no longer "active" (prevents immediate re-open).
	repl += " "

	rs := []rune(input)
	newRs := make([]rune, 0, len(rs)+len([]rune(repl))+2)
	newRs = append(newRs, rs[:start]...)
	newRs = append(newRs, []rune(repl)...)
	if end < len(rs) {
		newRs = append(newRs, rs[end:]...)
	}
	newInput := string(newRs)
	m.setCurrentInputValue(newInput)

	m.closeFilePicker()
	m.layout()

	// UX: for /editor, selecting a file should immediately run the command.
	if isEditorCommand(newInput) {
		msg := strings.TrimSpace(newInput)
		m.clearCurrentInput()
		if msg == "" {
			return nil
		}
		return m.submit(msg)
	}
	return nil
}

func (m Model) renderFilePicker(base string) string {
	// Calculate modal dimensions.
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

	// Size the list to fit within the modal.
	listHeight := modalHeight - 2 // no filter input, just title/pagination
	if listHeight < 4 {
		listHeight = 4
	}
	m.filePickerList.SetWidth(modalWidth - 4) // Account for padding/borders
	m.filePickerList.SetHeight(listHeight)

	// Build modal content.
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
