package tui

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/kit"
)

// monitorFilePickerItem implements list.Item for the file picker.
type monitorFilePickerItem struct {
	rel string // workdir-relative, slash-separated
}

func (f monitorFilePickerItem) FilterValue() string { return f.rel }
func (f monitorFilePickerItem) Title() string       { return f.rel }
func (f monitorFilePickerItem) Description() string { return "" }

// openFilePicker initializes and opens the file picker modal.
func (m *monitorModel) openFilePicker(query string) tea.Cmd {
	m.helpModalOpen = false
	m.closeAllPickers()
	m.filePickerCtrl.Open(kit.PickerConfig{
		Title:            "Select File",
		ShowPagination:   true,
		CursorPageKeyNav: true,
		Delegate:         kit.NewPickerDelegate(kit.DefaultPickerDelegateStyles(), nil),
		ModalWidth:       80,
		ModalHeight:      22,
		ModalMinWidth:    40,
		ModalMinHeight:   10,
		ModalMarginX:     8,
		ModalMarginY:     2,
	})
	m.filePickerCtrl.SetFilter(strings.TrimSpace(query))
	m.syncFilePickerState()

	// Scan files in background
	return m.scanFilesForPicker()
}

// closeFilePicker closes the file picker modal.
func (m *monitorModel) closeFilePicker() {
	m.filePickerCtrl.Close()
	m.syncFilePickerState()
	m.filePickerAllPaths = nil
	m.filePickerQuery = ""
}

// scanFilesForPicker scans the workspace directory for files.
func (m *monitorModel) scanFilesForPicker() tea.Cmd {
	return func() tea.Msg {
		wsDir := m.workspaceDir()
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
	m.ensureFilePickerCtrl()
	action := m.filePickerCtrl.Update(msg)
	m.syncFilePickerState()
	switch action.Type {
	case kit.PickerActionClose:
		m.closeFilePicker()
		return m, nil
	case kit.PickerActionAccept:
		return m, m.selectFileFromPicker()
	}
	return m, action.Cmd
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
	m.filePickerCtrl.SetItems(items)
	m.filePickerCtrl.SetFilter(m.filePickerQuery)
	m.syncFilePickerState()
}

// selectFileFromPicker inserts the selected file path into the composer.
func (m *monitorModel) selectFileFromPicker() tea.Cmd {
	m.ensureFilePickerCtrl()
	selected := m.filePickerCtrl.SelectedItem()
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
	m.ensureFilePickerCtrl()
	title := "Select File"
	if m.filePickerQuery != "" {
		title += " (@" + kit.TruncateMiddle(m.filePickerQuery, 32) + ")"
	}
	m.filePickerCtrl.SetTitle(title)
	m.syncFilePickerState()

	_ = base
	return m.filePickerCtrl.Render(m.width, m.height, "", "")
}
