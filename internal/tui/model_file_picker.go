package tui

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/agen8/internal/atref"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/pkg/fsutil"
)

// filePickerItem implements list.Item for the file picker.
type filePickerItem struct {
	rel string // workdir-relative, slash-separated
}

func (f filePickerItem) FilterValue() string { return f.rel }
func (f filePickerItem) Title() string       { return f.rel }
func (f filePickerItem) Description() string { return "" }

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
	return fsutil.GetWorkspaceDir(dd, rid)
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
		combined = append(combined, "/workspace/"+rel)
	}
	return combined, nil
}

func (m *Model) openFilePicker(initialQuery string) tea.Cmd {
	m.filePickerOpen = true

	l := list.New([]list.Item{}, kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), nil), 0, 0)
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
	dims := kit.ComputeModalDims(m.width, m.height, 80, 22, 40, 10, 8, 2)
	m.filePickerList.SetWidth(dims.ModalWidth - 4)
	m.filePickerList.SetHeight(dims.ListHeight)

	// Build modal content.
	content := m.filePickerList.View()

	opts := kit.DefaultPickerModalOpts(content, m.width, m.height, dims.ModalWidth, dims.ModalHeight)

	_ = base
	return kit.RenderOverlay(opts)
}
