package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/cost"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

const composeVPath = "/workdir/.workbench/compose.md"

// TurnRunner executes one user turn and returns the agent final response.
//
// The host (internal/app) owns the actual agent loop, memory commit policy,
// and persistence. The TUI calls this interface and renders events as they stream.
type TurnRunner interface {
	RunTurn(ctx context.Context, userMsg string) (final string, err error)
}

// vfsAccessor is an optional extension interface implemented by the app TurnRunner.
//
// The chat transcript is driven by RunTurn(), but some UI workflows (like the in-TUI
// editor and artifact preview) need direct access to mounted VFS paths.
//
// This keeps the agent loop contract unchanged: the editor is a host UX feature.
type vfsAccessor interface {
	ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error)
	WriteVFS(ctx context.Context, path string, data []byte) error
}

type eventMsg events.Event

type turnDoneMsg struct {
	final string
	err   error
}

type fileViewMsg struct {
	path      string
	content   string
	truncated bool
	err       error
}

type editorLoadMsg struct {
	vpath   string
	content string
	notice  string
	err     error
}

type editorSaveMsg struct {
	vpath string
	err   error
}

type editorExternalDoneMsg struct {
	vpath string
	err   error
}

type editorComposeLoadMsg struct {
	vpath string
	text  string
	err   error
}

type workdirPrefetchMsg struct {
	workdir string
	err     error
}

type preinitStatusMsg struct {
	workdir string
	modelID string
	err     error
}

type Model struct {
	ctx context.Context

	runner TurnRunner
	events <-chan events.Event

	transcript     viewport.Model
	activityList   list.Model
	activityDetail viewport.Model

	transcriptItems         []transcriptItem
	transcriptItemStartLine []int
	lastTurnUserItemIdx     int

	activities        []Activity
	activityIndexByID map[string]int
	pendingActivityID string
	activitySeq       int
	expandOutput      bool

	single    textarea.Model
	multiline textarea.Model
	isMulti   bool

	width  int
	height int

	showDetails   bool
	showTelemetry bool

	focus focusTarget

	turnInFlight bool
	turnStarted  time.Time
	turnTitle    string
	turnN        int

	pendingActionIdx       int
	pendingActionText      string
	waitingForAction       bool
	pendingActionIsToolRun bool

	fileViewOpen      bool
	fileViewPath      string
	fileViewContent   string
	fileViewTruncated bool
	fileViewErr       string

	editorOpen   bool
	editorVPath  string
	editorBuf    textarea.Model
	editorDirty  bool
	editorErr    string
	editorNotice string
	// Tracks an external editor session that is composing a chat message (not editing a file).
	externalEditorComposeVPath string

	sessionTitle  string
	workflowTitle string
	workdir       string
	modelID       string
	sessionID     string
	runID         string

	lastTurnTokensIn  int
	lastTurnTokensOut int
	lastTurnTokens    int
	totalTokens       int
	lastTurnCostUSD   string
	lastTurnDuration  string
	lastTurnSteps     string
	totalCostUSD      float64

	styleHeaderBar lipgloss.Style
	styleHeaderApp lipgloss.Style
	styleHeaderMid lipgloss.Style
	styleHeaderRHS lipgloss.Style

	styleDim lipgloss.Style

	styleUserBox   lipgloss.Style
	styleUserLabel lipgloss.Style
	styleAgentBox  lipgloss.Style
	styleAgent     lipgloss.Style
	styleAction    lipgloss.Style
	styleTelemetry lipgloss.Style
	styleOutcome   lipgloss.Style
	styleError     lipgloss.Style

	styleInputBox            lipgloss.Style
	styleComposerCardFocused lipgloss.Style
	styleComposerCardBlurred lipgloss.Style
	styleComposerAccentFocus lipgloss.Style
	styleComposerAccentBlur  lipgloss.Style
	styleComposerStatusKey   lipgloss.Style
	styleComposerStatusVal   lipgloss.Style
	styleHint                lipgloss.Style

	renderer *ContentRenderer

	// Command palette state
	commandPaletteOpen     bool
	commandPaletteMatches  []string
	commandPaletteSelected int

	// Model picker state
	modelPickerOpen bool
	modelPickerList list.Model

	// Help modal (Ctrl+P)
	helpModalOpen  bool
	helpViewport   viewport.Model
	helpModalText  string
	helpModalLines int

	// File picker state (workdir-scoped, triggered by typing '@' in input)
	filePickerOpen     bool
	filePickerList     list.Model
	filePickerAllPaths []string // canonical rel paths (slash-separated)
	filePickerQuery    string   // last applied query (for substring filtering)
	filePickerWorkdir  string   // workdir used for filePickerAllPaths (absolute)
}

func (m *Model) prefetchWorkdir() tea.Cmd {
	// Workdir can be unknown before first turn. Prefetch it without creating a run.
	return func() tea.Msg {
		wd, err := m.runner.RunTurn(m.ctx, "/pwd")
		return workdirPrefetchMsg{workdir: strings.TrimSpace(wd), err: err}
	}
}

type focusTarget int

const (
	focusInput focusTarget = iota
	focusActivityList
)

type transcriptItemKind int

const (
	transcriptSpacer transcriptItemKind = iota
	transcriptUser
	transcriptAgent
	transcriptAction
	transcriptError
)

type transcriptItem struct {
	kind transcriptItemKind

	// For user/agent/error content (raw, unwrapped).
	text string

	// For action lines.
	actionText        string
	actionCompletion  string
	actionIsToolRun   bool
	actionIsCompleted bool
}

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
	line := truncateRight(it.rel, maxW)
	_, _ = fmt.Fprint(w, style.Render(prefix+line))
}

func (m *Model) currentInputValue() string {
	if m.isMulti {
		return m.multiline.Value()
	}
	return m.single.Value()
}

func (m *Model) setCurrentInputValue(v string) {
	if m.isMulti {
		m.multiline.SetValue(v)
	} else {
		m.single.SetValue(v)
	}
}

func (m *Model) clearCurrentInput() {
	if m.isMulti {
		m.multiline.SetValue("")
	} else {
		m.single.SetValue("")
	}
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
		if all, err := scanWorkdirFiles(wd, 10000); err == nil {
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
		title += " (@" + truncateMiddle(m.filePickerQuery, 32) + ")"
	}
	if strings.Contains(m.filePickerList.Title, "loading") {
		// Preserve loading prefix if still loading.
		if strings.TrimSpace(m.filePickerWorkdir) == "" {
			m.filePickerList.Title = "Select File (loading workdir…) (@" + truncateMiddle(m.filePickerQuery, 32) + ")"
			return
		}
	}
	m.filePickerList.Title = title
}

func atQuoteClose(open rune) rune {
	switch open {
	case '"':
		return '"'
	case '\'':
		return '\''
	case '“':
		return '”'
	case '‘':
		return '’'
	default:
		return 0
	}
}

// activeAtTokenAtEnd finds the last @token that is currently being edited at the end
// of the input and returns:
// - query: token text (without leading @ and without surrounding quotes)
// - replaceStart/replaceEnd: rune indices to replace with the selected @ref
func activeAtTokenAtEnd(input string) (query string, replaceStart int, replaceEnd int, ok bool) {
	rs := []rune(input)
	n := len(rs)
	if n == 0 {
		return "", 0, 0, false
	}

	for i := n - 1; i >= 0; i-- {
		if rs[i] != '@' {
			continue
		}
		// Require a token boundary before '@' to avoid matching email-like text.
		if i > 0 && !unicode.IsSpace(rs[i-1]) {
			continue
		}

		// "@": empty query.
		if i+1 >= n {
			return "", i, n, true
		}

		open := rs[i+1]
		if close := atQuoteClose(open); close != 0 {
			// Quoted token @"..."/@'...'/@“...”/@‘...’
			start := i + 2
			if start > n {
				start = n
			}
			// Find a closing quote.
			closeIdx := -1
			for j := start; j < n; j++ {
				if rs[j] == close {
					closeIdx = j
					break
				}
			}
			if closeIdx == -1 {
				// Still typing (no closing quote); active token must be at end.
				return string(rs[start:]), i, n, true
			}
			// If the quote closed before the end, treat it as not the active token.
			if closeIdx != n-1 {
				continue
			}
			return string(rs[start:closeIdx]), i, n, true
		}

		// Unquoted token: consume until whitespace.
		j := i + 1
		for j < n && !unicode.IsSpace(rs[j]) {
			j++
		}
		// Active token must reach end-of-input.
		if j != n {
			continue
		}
		return string(rs[i+1 : n]), i, n, true
	}
	return "", 0, 0, false
}

func formatAtRef(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "@"
	}
	// Quote only when needed (spaces/tabs/newlines).
	if strings.ContainsAny(rel, " \t\n") {
		// Prefer a quote that doesn't appear in the path.
		if strings.Contains(rel, "'") && !strings.Contains(rel, `"`) {
			return `@"` + rel + `"`
		}
		return `@'` + rel + `'`
	}
	return "@" + rel
}

func isEditorCommand(input string) bool {
	fields := strings.Fields(strings.TrimSpace(input))
	return len(fields) > 0 && fields[0] == "/editor"
}

func (m *Model) syncFilePickerFromInput() tea.Cmd {
	input := m.currentInputValue()
	q, _, _, ok := activeAtTokenAtEnd(input)
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
	_, start, end, ok := activeAtTokenAtEnd(input)
	if !ok {
		// Token was removed; just close.
		m.closeFilePicker()
		m.layout()
		return nil
	}

	repl := formatAtRef(it.rel)
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

// Hardcoded list of available slash commands for the command palette.
var availableCommands = []string{
	"/model",
	"/open",
	"/editor",
	"/cd",
	"/pwd",
	"/workdir",
}

func isExactCommand(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, cmd := range availableCommands {
		if s == cmd {
			return true
		}
	}
	return false
}

// updateCommandPalette updates the command palette state based on the current input value.
// It detects if the input starts with "/" and filters commands accordingly.
func (m *Model) updateCommandPalette() {
	prevOpen := m.commandPaletteOpen
	prevVisible := 0
	if prevOpen {
		prevVisible = min(len(m.commandPaletteMatches), 6)
	}

	var inputValue string
	if m.isMulti {
		inputValue = m.multiline.Value()
	} else {
		inputValue = m.single.Value()
	}

	// Extract the first token (command part) from the input.
	fields := strings.Fields(inputValue)
	var firstToken string
	if len(fields) > 0 {
		firstToken = fields[0]
	} else {
		// Empty input or only whitespace - use the raw value.
		firstToken = strings.TrimSpace(inputValue)
	}

	// Check if we're in command mode (starts with "/").
	if strings.HasPrefix(firstToken, "/") {
		// If the user has already completed a valid command token and is now typing
		// arguments (i.e. there is whitespace after the first token), keep the palette closed.
		if isExactCommand(firstToken) && strings.ContainsAny(inputValue, " \t\n") {
			m.commandPaletteOpen = false
			m.commandPaletteMatches = nil
			m.commandPaletteSelected = 0
			if prevOpen {
				m.layout()
			}
			return
		}

		// Filter commands that match the typed prefix.
		matches := []string{}
		for _, cmd := range availableCommands {
			if strings.HasPrefix(cmd, firstToken) {
				matches = append(matches, cmd)
			}
		}

		if len(matches) > 0 {
			m.commandPaletteOpen = true
			m.commandPaletteMatches = matches
			// Ensure selected index is valid.
			if m.commandPaletteSelected >= len(matches) {
				m.commandPaletteSelected = 0
			}
			if m.commandPaletteSelected < 0 {
				m.commandPaletteSelected = 0
			}
		} else {
			// No matches, close palette.
			m.commandPaletteOpen = false
			m.commandPaletteMatches = nil
			m.commandPaletteSelected = 0
		}
	} else {
		// Not in command mode, close palette.
		m.commandPaletteOpen = false
		m.commandPaletteMatches = nil
		m.commandPaletteSelected = 0
	}

	// If the palette visibility/height changed, recompute layout so total View height
	// stays within the terminal bounds (avoids header/transcript clipping).
	newOpen := m.commandPaletteOpen
	newVisible := 0
	if newOpen {
		newVisible = min(len(m.commandPaletteMatches), 6)
	}
	if prevOpen != newOpen || prevVisible != newVisible {
		m.layout()
	}
}

// autocompleteCommand replaces the first token in the input with the selected command,
// preserving any trailing arguments.
func (m *Model) autocompleteCommand() {
	if !m.commandPaletteOpen || len(m.commandPaletteMatches) == 0 {
		return
	}
	if m.commandPaletteSelected < 0 || m.commandPaletteSelected >= len(m.commandPaletteMatches) {
		return
	}

	selectedCmd := m.commandPaletteMatches[m.commandPaletteSelected]

	var inputValue string
	if m.isMulti {
		inputValue = m.multiline.Value()
	} else {
		inputValue = m.single.Value()
	}

	// Extract the first token and any trailing args.
	fields := strings.Fields(inputValue)
	if len(fields) == 0 {
		// Empty input, just set the command.
		if m.isMulti {
			m.multiline.SetValue(selectedCmd)
		} else {
			m.single.SetValue(selectedCmd)
		}
	} else {
		// Replace first token with selected command, preserve rest.
		rest := strings.TrimSpace(strings.TrimPrefix(inputValue, fields[0]))
		newValue := selectedCmd
		if rest != "" {
			newValue = selectedCmd + " " + rest
		}
		if m.isMulti {
			m.multiline.SetValue(newValue)
		} else {
			m.single.SetValue(newValue)
		}
	}

	// Close palette after autocomplete.
	m.commandPaletteOpen = false
	m.commandPaletteMatches = nil
	m.commandPaletteSelected = 0

	// Recompute layout so the transcript area expands again.
	m.layout()
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

func (m *Model) openHelpModal() {
	m.helpModalOpen = true
	m.helpModalText = m.helpModalContent()
	m.helpModalLines = 0
	if m.helpModalText != "" {
		m.helpModalLines = len(strings.Split(m.helpModalText, "\n"))
	}
	m.helpViewport.SetContent(m.helpModalText)
	m.helpViewport.SetYOffset(0)

	// Ensure viewport has a real size immediately so scrolling clamps correctly.
	contentW, _, vpH := m.helpModalDims()
	m.helpViewport.Width = contentW
	m.helpViewport.Height = vpH
	m.helpViewport.SetYOffset(0)
	m.layout()
}

func (m *Model) closeHelpModal() {
	m.helpModalOpen = false
	m.helpViewport.SetContent("")
	m.helpViewport.SetYOffset(0)
	m.helpModalText = ""
	m.helpModalLines = 0
	m.layout()
}

func (m *Model) helpModalDims() (contentW, contentH, vpH int) {
	outerW := 84
	if outerW > m.width-8 {
		outerW = m.width - 8
	}
	if outerW < 44 {
		outerW = 44
	}
	outerH := 22
	if outerH > m.height-8 {
		outerH = m.height - 8
	}
	if outerH < 12 {
		outerH = 12
	}

	// Keep TOTAL modal size within outerW/outerH (account for padding + border).
	// totalW = contentW + paddingLR(4) + borderLR(2) = contentW + 6
	// totalH = contentH + paddingTB(2) + borderTB(2) = contentH + 4
	contentW = max(10, outerW-6)
	contentH = max(6, outerH-4)

	// Reserve some space for title + footer inside the content area.
	vpH = max(1, contentH-3)
	return contentW, contentH, vpH
}

func (m *Model) clampHelpViewport() {
	if !m.helpModalOpen {
		return
	}
	h := m.helpViewport.Height
	if h < 1 {
		h = 1
	}
	maxY := 0
	if m.helpModalLines > h {
		maxY = m.helpModalLines - h
	}
	if m.helpViewport.YOffset < 0 {
		m.helpViewport.SetYOffset(0)
		return
	}
	if m.helpViewport.YOffset > maxY {
		m.helpViewport.SetYOffset(maxY)
	}
}

func (m *Model) helpModalContent() string {
	// Keep this plain-text (no selection/highlight), and long enough to scroll.
	lines := []string{
		"Shortcuts",
		"",
		"  ctrl+p  help (this screen)",
		"  ctrl+a  toggle activity panel",
		"  tab     cycle focus (input/activity)",
		"  pgup/pgdn  scroll transcript",
		"  ctrl+u/ctrl+f  half-page scroll transcript",
		"  ctrl+g  toggle multiline input",
		"  enter   send (single-line)",
		"  ctrl+o  send (multiline)",
		"  esc     close modal/panels",
		"",
		"Composer",
		"",
		"  /editor + Enter  open $EDITOR to compose a message (loads back into input)",
		"  ctrl+e           open $EDITOR with current input prefilled",
		"",
		"Slash commands",
		"",
		"  /model           open model picker",
		"  /model <id>      set model",
		"  /open <path>     open a file via OS",
		"  /editor <path>   edit a workdir file in $EDITOR",
		"  /cd <path>       change workdir",
		"  /pwd             show workdir",
		"  /workdir         alias for /pwd",
		"",
		"Command palette",
		"",
		"  Type '/' in input to show commands; Up/Down to select; Enter to autocomplete; Esc to close.",
		"",
		"References",
		"",
		"  Type '@' to open file picker; Enter inserts an @ref into input; Esc closes.",
		"",
		"Tips",
		"",
		"  - This modal is scrollable: use Up/Down, PgUp/PgDn, or mouse wheel.",
		"  - Press Esc to close.",
	}
	return strings.Join(lines, "\n")
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

func New(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) Model {
	main := viewport.New(0, 0)
	// Important: avoid horizontal padding on viewports.
	//
	// If viewport content becomes wider than the terminal (due to padding + borders
	// in transcript elements), the terminal will soft-wrap long lines. That increases
	// the effective number of screen lines and can make the header appear to
	// "disappear" (scrolled off the top) on resize or when toggling the sidebar.
	main.Style = lipgloss.NewStyle()
	main.MouseWheelEnabled = true

	details := viewport.New(0, 0)
	details.Style = lipgloss.NewStyle()
	details.MouseWheelEnabled = true

	helpVp := viewport.New(0, 0)
	helpVp.Style = lipgloss.NewStyle()
	helpVp.MouseWheelEnabled = true

	activity := list.New([]list.Item{}, newActivityDelegate(), 0, 0)
	activity.Title = "Activity"
	activity.SetShowHelp(false)
	activity.SetShowStatusBar(false)
	activity.SetShowPagination(false)
	activity.SetFilteringEnabled(false)
	activity.SetShowFilter(false)
	activity.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	// Textarea focus styling:
	//
	// The default bubbles/textarea focused style uses a visible cursor-line highlight.
	// For Workbench, we want focus to affect behavior (cursor + key handling) but not
	// introduce a distinct "selected" visual treatment in the input box.
	//
	// So: use identical styles for focused + blurred, and avoid background/reverse
	// effects on the cursor line.
	plainTextAreaStyle := textarea.Style{
		Base:        lipgloss.NewStyle(),
		CursorLine:  lipgloss.NewStyle(),
		EndOfBuffer: lipgloss.NewStyle().Foreground(lipgloss.Color("#404040")),
		LineNumber:  lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		Prompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		Text: lipgloss.NewStyle().Foreground(lipgloss.Color("#eaeaea")),
	}

	single := textarea.New()
	single.Placeholder = "Type a message…"
	single.Focus()
	single.Prompt = "you> "
	single.ShowLineNumbers = false
	single.SetHeight(1)
	single.CharLimit = 0
	single.KeyMap.InsertNewline.SetEnabled(false) // Enter should submit in single-line mode.
	single.FocusedStyle = plainTextAreaStyle
	single.BlurredStyle = plainTextAreaStyle

	multi := textarea.New()
	multi.Placeholder = "Multiline message (Ctrl+O to send)…"
	multi.Prompt = "…> "
	multi.ShowLineNumbers = false
	multi.CharLimit = 0
	multi.SetHeight(6)
	// Keep prompt dimmer for multiline mode, but still avoid focus highlighting.
	multiStyle := plainTextAreaStyle
	multiStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070"))
	multi.FocusedStyle = multiStyle
	multi.BlurredStyle = multiStyle

	editor := textarea.New()
	editor.Placeholder = ""
	editor.Prompt = ""
	editor.ShowLineNumbers = true
	editor.CharLimit = 0
	editor.SetHeight(1)
	editorStyle := plainTextAreaStyle
	editorStyle.Prompt = lipgloss.NewStyle()
	editor.FocusedStyle = editorStyle
	editor.BlurredStyle = editorStyle

	m := Model{
		ctx:            ctx,
		runner:         runner,
		events:         evCh,
		transcript:     main,
		activityList:   activity,
		activityDetail: details,
		helpViewport:   helpVp,
		// Default: start with the activity panel closed. Users can toggle it with
		// Ctrl+A, or enable it by default via WORKBENCH_ACTIVITY/--activity.
		showDetails:       false,
		activityIndexByID: map[string]int{},
		single:            single,
		multiline:         multi,
		editorBuf:         editor,

		styleHeaderBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleHeaderApp: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
		styleHeaderMid: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleHeaderRHS: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")),

		styleDim: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),

		styleUserLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		styleUserBox: lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleAgentBox: lipgloss.NewStyle().
			Padding(0, 1),
		styleAgent: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")),
		styleAction: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleTelemetry: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6b6b6b")),
		styleOutcome: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8a8a8a")),
		styleError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff5f5f")).
			Bold(true),

		styleInputBox: lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleComposerCardFocused: lipgloss.NewStyle().
			Margin(0, 1).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#6bbcff")),
		styleComposerCardBlurred: lipgloss.NewStyle().
			Margin(0, 1).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleComposerAccentFocus: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6bbcff")).
			Bold(true),
		styleComposerAccentBlur: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#404040")),
		styleComposerStatusKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		styleComposerStatusVal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")),
		styleHint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#707070")),

		renderer: newContentRenderer(),
		focus:    focusInput,
	}

	m.pendingActionIdx = -1
	m.lastTurnUserItemIdx = -1
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.waitEvent(),
		func() tea.Msg {
			wd, err := m.runner.RunTurn(m.ctx, "/pwd")
			if err != nil {
				return preinitStatusMsg{err: err}
			}
			// /model (or /model show) is a host-side command and works pre-init.
			out, err := m.runner.RunTurn(m.ctx, "/model")
			if err != nil {
				return preinitStatusMsg{workdir: strings.TrimSpace(wd), err: err}
			}
			modelID := ""
			// Expected: "Current model: <id>"
			if s := strings.TrimSpace(out); s != "" {
				const pfx = "Current model:"
				if strings.HasPrefix(s, pfx) {
					modelID = strings.TrimSpace(strings.TrimPrefix(s, pfx))
				}
			}
			return preinitStatusMsg{workdir: strings.TrimSpace(wd), modelID: strings.TrimSpace(modelID)}
		},
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case list.FilterMatchesMsg:
		// Critical: bubbles/list filtering runs asynchronously and sends FilterMatchesMsg
		// back through the Bubble Tea update loop. If we don't forward this message
		// into the picker list, the visible items will never update.
		if m.modelPickerOpen {
			var cmd tea.Cmd
			m.modelPickerList, cmd = m.modelPickerList.Update(msg)
			return m, cmd
		}

	case tea.KeyMsg:
		// Global quit.
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

		// Help modal captures keys globally while open.
		if m.helpModalOpen {
			if msg.Type == tea.KeyEsc {
				m.closeHelpModal()
				return m, nil
			}
			// Vim keys.
			switch msg.String() {
			case "j":
				msg = tea.KeyMsg{Type: tea.KeyDown}
			case "k":
				msg = tea.KeyMsg{Type: tea.KeyUp}
			}
			var cmd tea.Cmd
			m.helpViewport, cmd = m.helpViewport.Update(msg)
			m.clampHelpViewport()
			return m, cmd
		}

		// Ctrl+P opens help modal (only when input is focused).
		if m.focus == focusInput && (msg.Type == tea.KeyCtrlP || strings.EqualFold(msg.String(), "ctrl+p")) {
			m.openHelpModal()
			return m, nil
		}

		// Ctrl+E opens external editor for composing a message, prefilled from current input.
		// Only active when the input is focused (so we don't override Activity panel shortcuts).
		if m.focus == focusInput && strings.EqualFold(msg.String(), "ctrl+e") {
			return m, m.openComposeEditorPrefill()
		}

		// Modal: model picker (must capture keys even when input is focused).
		if m.modelPickerOpen {
			switch msg.Type {
			case tea.KeyEsc:
				m.closeModelPicker()
				m.layout()
				return m, nil
			case tea.KeyEnter:
				return m, m.selectModelFromPicker()
			case tea.KeyUp:
				m.modelPickerList.CursorUp()
				return m, nil
			case tea.KeyDown:
				m.modelPickerList.CursorDown()
				return m, nil
			case tea.KeyPgUp, tea.KeyCtrlU:
				m.modelPickerList.CursorUp()
				return m, nil
			case tea.KeyPgDown, tea.KeyCtrlF:
				m.modelPickerList.CursorDown()
				return m, nil
			default:
				var cmd tea.Cmd
				m.modelPickerList, cmd = m.modelPickerList.Update(msg)
				return m, cmd
			}
		}

		// In-TUI editor mode: capture keys and render a full-screen editor.
		if m.editorOpen {
			switch msg.Type {
			case tea.KeyEsc:
				m.editorOpen = false
				m.editorVPath = ""
				m.editorDirty = false
				m.editorErr = ""
				m.editorNotice = ""
				m.focus = focusInput
				if m.isMulti {
					m.multiline.Focus()
				} else {
					m.single.Focus()
				}
				m.layout()
				return m, nil
			}
			if msg.String() == "ctrl+o" {
				return m, m.saveEditor()
			}

			before := m.editorBuf.Value()
			var cmd tea.Cmd
			m.editorBuf, cmd = m.editorBuf.Update(msg)
			if m.editorBuf.Value() != before {
				m.editorDirty = true
				m.editorNotice = ""
			}
			return m, cmd
		}

		// Modal-ish: file picker (captures navigation/confirm/cancel keys, but lets
		// normal typing continue to flow into the input).
		if m.filePickerOpen && m.focus == focusInput {
			switch msg.Type {
			case tea.KeyEsc:
				m.closeFilePicker()
				m.layout()
				return m, nil
			case tea.KeyEnter:
				return m, m.selectFileFromPicker()
			case tea.KeyUp:
				m.filePickerList.CursorUp()
				return m, nil
			case tea.KeyDown:
				m.filePickerList.CursorDown()
				return m, nil
			case tea.KeyPgUp, tea.KeyCtrlU:
				m.filePickerList.CursorUp()
				return m, nil
			case tea.KeyPgDown, tea.KeyCtrlF:
				m.filePickerList.CursorDown()
				return m, nil
			}
		}

		// Transcript scrolling (mouse capture is off by default for native selection).
		//
		// These keys scroll the main chat transcript regardless of input focus:
		//   - PgUp/PgDn
		//   - Ctrl+U/Ctrl+F (half-page up/down)
		//
		// When the Activity panel is focused, PgUp/PgDn are routed to the details panel
		// instead (see below).
		if msg.Type == tea.KeyPgUp || msg.Type == tea.KeyPgDown || msg.Type == tea.KeyCtrlU || msg.Type == tea.KeyCtrlF {
			// If Activity is focused, let the details panel consume these keys.
			if m.showDetails && m.focus == focusActivityList {
				break
			}
			var cmd tea.Cmd
			m.transcript, cmd = m.transcript.Update(msg)
			return m, cmd
		}

		// Toggle activity panel open/closed.
		if msg.Type == tea.KeyCtrlA {
			m.showDetails = !m.showDetails
			if m.showDetails {
				m.focus = focusActivityList
				m.single.Blur()
				m.multiline.Blur()
				m.refreshActivityDetail()
			} else {
				m.focus = focusInput
				if m.isMulti {
					m.multiline.Focus()
				} else {
					m.single.Focus()
				}
			}
			m.layout()
			return m, nil
		}

		// Esc closes command palette first (if open), then file preview, then activity panel.
		if msg.Type == tea.KeyEsc {
			if m.commandPaletteOpen {
				m.commandPaletteOpen = false
				m.commandPaletteMatches = nil
				m.commandPaletteSelected = 0
				m.layout()
				return m, nil
			}
			if m.showDetails && m.fileViewOpen {
				m.fileViewOpen = false
				m.fileViewPath = ""
				m.fileViewContent = ""
				m.fileViewTruncated = false
				m.fileViewErr = ""
				m.refreshActivityDetail()
				return m, nil
			}
			if m.showDetails {
				m.showDetails = false
				m.focus = focusInput
				if m.isMulti {
					m.multiline.Focus()
				} else {
					m.single.Focus()
				}
				m.layout()
				return m, nil
			}
		}

		// Tab cycles focus between input and activity list.
		if msg.Type == tea.KeyTab {
			if !m.showDetails {
				m.focus = focusInput
				return m, nil
			}
			if m.focus == focusInput {
				m.focus = focusActivityList
				m.single.Blur()
				m.multiline.Blur()
			} else {
				m.focus = focusInput
				if m.isMulti {
					m.multiline.Focus()
				} else {
					m.single.Focus()
				}
			}
			return m, nil
		}

		// Telemetry toggle (hidden by default).
		// - When input is focused, only Ctrl+T toggles (so normal typing is never hijacked).
		// - When Activity list is focused, allow Ctrl+T and plain "t".
		if (m.focus == focusInput && msg.Type == tea.KeyCtrlT) ||
			(m.focus == focusActivityList && (msg.Type == tea.KeyCtrlT || strings.EqualFold(msg.String(), "t"))) {
			m.showTelemetry = !m.showTelemetry
			m.refreshActivityDetail()
			return m, nil
		}

		// Toggle multiline.
		//
		// Note: Ctrl+J is ASCII LF and is often indistinguishable from Enter in many
		// terminal setups. Use Ctrl+G as the reliable toggle.
		if m.focus == focusInput && msg.Type == tea.KeyCtrlG {
			m.toggleMultiline()
			return m, nil
		}

		// Activity navigation / details scrolling (when focused).
		if m.showDetails && m.focus == focusActivityList {
			switch msg.Type {
			case tea.KeyUp:
				m.activityList.CursorUp()
				m.refreshActivityDetail()
				return m, nil
			case tea.KeyDown:
				m.activityList.CursorDown()
				m.refreshActivityDetail()
				return m, nil
			case tea.KeyEnter:
				m.expandOutput = !m.expandOutput
				m.refreshActivityDetail()
				return m, nil
			}
			switch msg.String() {
			case "j":
				m.activityList.CursorDown()
				m.refreshActivityDetail()
				return m, nil
			case "k":
				m.activityList.CursorUp()
				m.refreshActivityDetail()
				return m, nil
			case "e", "ctrl+e":
				m.expandOutput = !m.expandOutput
				m.refreshActivityDetail()
				return m, nil
			case "o":
				return m, m.openSelectedActivityFile()
			case "ctrl+p":
				m.activityList.CursorUp()
				m.refreshActivityDetail()
				return m, nil
			case "ctrl+n":
				m.activityList.CursorDown()
				m.refreshActivityDetail()
				return m, nil
			}
			switch msg.Type {
			case tea.KeyPgUp, tea.KeyCtrlU, tea.KeyPgDown, tea.KeyCtrlF:
				var cmd tea.Cmd
				m.activityDetail, cmd = m.activityDetail.Update(msg)
				return m, cmd
			}
			// Do not forward keys to the input when Activity is focused.
			return m, nil
		}

		if m.turnInFlight {
			// While a turn is running, we allow scrolling but prevent submitting.
			// Mouse scroll handling is done in the global MouseMsg handler below.
		}

		// Only forward key events into the input when input is focused.
		if m.focus != focusInput {
			return m, nil
		}

		// Command palette navigation (Up/Down/Enter) when palette is open.
		// These keys must be intercepted before they reach the textarea.
		if m.commandPaletteOpen {
			switch msg.Type {
			case tea.KeyUp:
				if len(m.commandPaletteMatches) > 0 {
					m.commandPaletteSelected--
					if m.commandPaletteSelected < 0 {
						m.commandPaletteSelected = len(m.commandPaletteMatches) - 1
					}
				}
				return m, nil
			case tea.KeyDown:
				if len(m.commandPaletteMatches) > 0 {
					m.commandPaletteSelected++
					if m.commandPaletteSelected >= len(m.commandPaletteMatches) {
						m.commandPaletteSelected = 0
					}
				}
				return m, nil
			case tea.KeyEnter:
				// Autocomplete the selected command (do not submit).
				m.autocompleteCommand()
				return m, nil
			}
		}

		if m.isMulti {
			// In multiline mode, Enter inserts newline.
			//
			// Note: many terminals do not distinguish Ctrl+Enter from Enter unless an
			// "extended keys" protocol is enabled. We support:
			//   - Ctrl+Enter when it is exposed by the terminal/driver
			//   - Ctrl+O as a reliable fallback "send" key (avoids Alt+Enter fullscreen on macOS terminals)
			if msg.Type == tea.KeyCtrlO || strings.EqualFold(msg.String(), "ctrl+o") || strings.EqualFold(msg.String(), "ctrl+enter") {
				return m, m.submitMultiline()
			}
			if msg.Type == tea.KeyEnter {
				// Let textarea handle newline.
				var cmd tea.Cmd
				m.multiline, cmd = m.multiline.Update(msg)
				m.updateCommandPalette()
				cmd2 := m.syncFilePickerFromInput()
				if cmd == nil {
					return m, cmd2
				}
				if cmd2 == nil {
					return m, cmd
				}
				return m, tea.Batch(cmd, cmd2)
			}
			var cmd tea.Cmd
			m.multiline, cmd = m.multiline.Update(msg)
			m.updateCommandPalette()
			cmd2 := m.syncFilePickerFromInput()
			if cmd == nil {
				return m, cmd2
			}
			if cmd2 == nil {
				return m, cmd
			}
			return m, tea.Batch(cmd, cmd2)
		}

		// Single-line mode: Enter submits.
		if msg.Type == tea.KeyEnter {
			return m, m.submitSingle()
		}
		var cmd tea.Cmd
		m.single, cmd = m.single.Update(msg)
		// If pasted content includes newlines, switch to multiline.
		if strings.Contains(m.single.Value(), "\n") {
			m.multiline.SetValue(m.single.Value())
			m.single.SetValue("")
			m.isMulti = true
			m.multiline.Focus()
			m.layout()
		}
		m.updateCommandPalette()
		cmd2 := m.syncFilePickerFromInput()
		if cmd == nil {
			return m, cmd2
		}
		if cmd2 == nil {
			return m, cmd
		}
		return m, tea.Batch(cmd, cmd2)

	case eventMsg:
		ev := events.Event(msg)
		m.onEvent(ev)
		cmds := []tea.Cmd{m.waitEvent()}
		if ev.Type == "ui.editor.open" {
			if v := strings.TrimSpace(ev.Data["vpath"]); v != "" {
				if strings.TrimSpace(ev.Data["purpose"]) == "compose" {
					m.externalEditorComposeVPath = v
				}
				cmds = append(cmds, m.openEditor(v))
			}
		}
		return m, tea.Batch(cmds...)

	case workdirPrefetchMsg:
		if msg.err != nil {
			// Keep picker open but empty; user can still type, or try again later.
			return m, nil
		}
		if strings.TrimSpace(msg.workdir) != "" {
			m.workdir = strings.TrimSpace(msg.workdir)
		}
		// If picker is open, populate it now that we know the workdir.
		if m.filePickerOpen && strings.TrimSpace(m.workdir) != "" {
			if all, err := scanWorkdirFiles(m.workdir, 10000); err == nil {
				m.filePickerAllPaths = all
				m.filePickerWorkdir = strings.TrimSpace(m.workdir)
				m.applyFilePickerQuery(m.filePickerQuery)
				m.filePickerList.Title = "Select File"
				m.layout()
			}
		}
		return m, nil

	case preinitStatusMsg:
		// Best-effort; do not surface errors as transcript output.
		if strings.TrimSpace(msg.workdir) != "" {
			m.workdir = strings.TrimSpace(msg.workdir)
		}
		if strings.TrimSpace(msg.modelID) != "" {
			m.modelID = strings.TrimSpace(msg.modelID)
		}
		// If picker is open and workdir arrived, populate it.
		if m.filePickerOpen && strings.TrimSpace(m.workdir) != "" && m.filePickerWorkdir == "" {
			if all, err := scanWorkdirFiles(m.workdir, 10000); err == nil {
				m.filePickerAllPaths = all
				m.filePickerWorkdir = strings.TrimSpace(m.workdir)
				m.applyFilePickerQuery(m.filePickerQuery)
				if strings.Contains(m.filePickerList.Title, "loading") {
					m.filePickerList.Title = "Select File"
				}
			}
		}
		m.layout()
		return m, nil

	case editorLoadMsg:
		if strings.TrimSpace(msg.vpath) == "" || msg.vpath != m.editorVPath {
			return m, nil
		}
		if msg.err != nil {
			m.editorErr = msg.err.Error()
			m.editorBuf.SetValue("")
			m.editorDirty = false
			m.editorNotice = ""
		} else {
			m.editorErr = ""
			m.editorBuf.SetValue(msg.content)
			m.editorDirty = false
			m.editorNotice = strings.TrimSpace(msg.notice)
		}
		m.layout()
		return m, nil

	case editorSaveMsg:
		if strings.TrimSpace(msg.vpath) == "" || msg.vpath != m.editorVPath {
			return m, nil
		}
		if msg.err != nil {
			m.editorErr = msg.err.Error()
			return m, nil
		}
		m.editorErr = ""
		m.editorNotice = "saved"
		m.editorDirty = false
		return m, nil

	case editorExternalDoneMsg:
		if strings.TrimSpace(msg.vpath) == "" {
			return m, nil
		}
		if msg.err != nil {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "editor error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.rebuildTranscript()
		} else if strings.TrimSpace(m.externalEditorComposeVPath) != "" && msg.vpath == m.externalEditorComposeVPath {
			// Compose flow: load the compose buffer into the multiline input.
			cmd := m.loadComposeBuffer(msg.vpath)
			m.externalEditorComposeVPath = ""
			m.focus = focusInput
			m.isMulti = true
			m.multiline.Focus()
			return m, cmd
		}
		m.focus = focusInput
		if m.isMulti {
			m.multiline.Focus()
		} else {
			m.single.Focus()
		}
		return m, nil

	case editorComposeLoadMsg:
		if strings.TrimSpace(msg.vpath) == "" || strings.TrimSpace(msg.vpath) != strings.TrimSpace(composeVPath) {
			return m, nil
		}
		if msg.err != nil {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "compose load error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.rebuildTranscript()
			return m, nil
		}
		m.focus = focusInput
		m.isMulti = true
		m.multiline.SetValue(msg.text)
		m.multiline.Focus()
		m.layout()
		return m, nil

	case fileViewMsg:
		// Ignore stale responses.
		if strings.TrimSpace(msg.path) != "" && msg.path != m.fileViewPath {
			return m, nil
		}
		if msg.err != nil {
			m.fileViewErr = msg.err.Error()
			m.fileViewContent = ""
			m.fileViewTruncated = false
		} else {
			m.fileViewErr = ""
			m.fileViewContent = msg.content
			m.fileViewTruncated = msg.truncated
		}
		m.refreshActivityDetail()
		return m, nil

	case turnDoneMsg:
		m.turnInFlight = false
		if msg.err != nil {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "agent error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.turnTitle = ""
			return m, nil
		}
		if strings.TrimSpace(msg.final) != "" {
			m.addTranscriptItem(transcriptItem{kind: transcriptAgent, text: strings.TrimSpace(msg.final)})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
		}
		m.scrollToCurrentTurnStart()
		m.turnTitle = ""
		return m, nil
	}

	// Mouse wheel scrolling:
	// - Always allow scrolling the transcript.
	// - When the Activity pane is open, scroll the Details panel if the cursor is over it,
	//   otherwise scroll the transcript.
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if m.helpModalOpen {
			var cmd tea.Cmd
			m.helpViewport, cmd = m.helpViewport.Update(msg)
			m.clampHelpViewport()
			return m, cmd
		}
		// If details are visible and the mouse is within the right pane, scroll details.
		if m.showDetails && m.activityList.Width() > 0 {
			leftW := m.transcript.Width
			if msg.X >= leftW {
				var cmd tea.Cmd
				m.activityDetail, cmd = m.activityDetail.Update(msg)
				return m, cmd
			}
		}
		var cmd tea.Cmd
		m.transcript, cmd = m.transcript.Update(msg)
		return m, cmd
	}

	// Default fallthrough: keep viewport responsive.
	var cmd tea.Cmd
	m.transcript, cmd = m.transcript.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.editorOpen {
		return m.renderEditorView()
	}
	header := m.renderHeader()
	body := m.renderBody()
	input := m.renderInput()
	base := header + "\n" + body + "\n" + input

	// Overlay help modal if open.
	if m.helpModalOpen {
		return m.renderHelpModal(base)
	}

	// Overlay file picker modal if open.
	if m.filePickerOpen {
		return m.renderFilePicker(base)
	}

	// Overlay model picker modal if open
	if m.modelPickerOpen {
		return m.renderModelPicker(base)
	}

	return base
}

func (m Model) renderEditorView() string {
	header := m.renderHeader()

	title := m.editorTitle()
	w := max(1, m.width-2)
	bar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0c0c0")).
		Padding(0, 1).
		Render(truncateMiddle(title, w))

	bodyH := max(1, m.height-lipgloss.Height(header)-lipgloss.Height(bar)-2)
	m.editorBuf.SetHeight(bodyH)
	m.editorBuf.SetWidth(max(1, m.width-2))
	editor := lipgloss.NewStyle().Padding(0, 1).Render(m.editorBuf.View())

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Padding(0, 1).
		Render("ctrl+o save  esc cancel")

	return header + "\n" + bar + "\n" + editor + "\n" + footer
}

func (m Model) editorTitle() string {
	vp := strings.TrimSpace(m.editorVPath)
	name := vp
	switch {
	case strings.HasPrefix(vp, "/workdir/"):
		name = strings.TrimPrefix(vp, "/workdir/")
	case strings.HasPrefix(vp, "/workspace/"):
		name = strings.TrimPrefix(vp, "/workspace/")
	}
	title := "Editing: " + name
	if m.editorDirty {
		title += " *"
	}
	if strings.TrimSpace(m.editorNotice) != "" {
		title += " · " + strings.TrimSpace(m.editorNotice)
	}
	if strings.TrimSpace(m.editorErr) != "" {
		title += " · error: " + strings.TrimSpace(m.editorErr)
	}
	return title
}

func (m Model) renderHelpModal(base string) string {
	_ = base
	contentW, contentH, vpH := m.helpModalDims()
	m.helpViewport.Width = contentW
	m.helpViewport.Height = vpH

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#eaeaea")).Render("Shortcuts & commands")
	footer := lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")).Render("↑/↓ scroll  pgup/pgdn faster  esc close")
	body := m.helpViewport.View()
	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body, "", footer)

	modalStyle := lipgloss.NewStyle().
		Width(contentW).
		Height(contentH).
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6bbcff")).
		Background(lipgloss.Color("#1a1a1a")).
		Foreground(lipgloss.Color("#eaeaea"))

	modalContent := modalStyle.Render(content)
	modalLines := strings.Split(modalContent, "\n")
	modalHeightActual := len(modalLines)
	modalWidthActual := 0
	for _, line := range modalLines {
		if w := lipgloss.Width(line); w > modalWidthActual {
			modalWidthActual = w
		}
	}

	topPos := (m.height - modalHeightActual) / 2
	if topPos < 0 {
		topPos = 0
	}
	leftPos := (m.width - modalWidthActual) / 2
	if leftPos < 0 {
		leftPos = 0
	}

	// Render over a blank backdrop to avoid ANSI corruption artifacts.
	result := make([]string, m.height)
	for i := 0; i < m.height; i++ {
		result[i] = strings.Repeat(" ", max(1, m.width))
		if i >= topPos && i < topPos+modalHeightActual {
			lineIdx := i - topPos
			if lineIdx < len(modalLines) {
				result[i] = strings.Repeat(" ", leftPos) + modalLines[lineIdx]
			}
		}
	}
	return strings.Join(result, "\n")
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
		Background(lipgloss.Color("#1a1a1a")).
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

	return strings.Join(result, "\n")
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

	// Style the modal.
	modalStyle := lipgloss.NewStyle().
		Width(modalWidth).
		Height(modalHeight).
		Padding(1, 2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#6bbcff")).
		Foreground(lipgloss.Color("#eaeaea"))

	modalContent := modalStyle.Render(content)

	// Split modal content into lines.
	modalLines := strings.Split(modalContent, "\n")
	modalHeightActual := len(modalLines)
	modalWidthActual := 0
	for _, line := range modalLines {
		if w := lipgloss.Width(line); w > modalWidthActual {
			modalWidthActual = w
		}
	}

	// Calculate centering position.
	topPos := (m.height - modalHeightActual) / 2
	if topPos < 0 {
		topPos = 0
	}
	leftPos := (m.width - modalWidthActual) / 2
	if leftPos < 0 {
		leftPos = 0
	}

	// Render over a blank backdrop (avoid slicing ANSI from base).
	_ = base
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

	return strings.Join(result, "\n")
}

func (m *Model) openEditor(vpath string) tea.Cmd {
	// Prefer the user's external editor when configured.
	//
	// /editor is a host UX convenience command; it should open $VISUAL/$EDITOR
	// rather than forcing users into a bespoke in-TUI editor.
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor != "" {
		if cmd, err := m.editorExecCmd(editor, vpath); err == nil && cmd != nil {
			return tea.ExecProcess(cmd, func(err error) tea.Msg {
				return editorExternalDoneMsg{vpath: vpath, err: err}
			})
		}
	}
	return m.openInternalEditor(vpath)
}

func (m *Model) openInternalEditor(vpath string) tea.Cmd {
	m.editorOpen = true
	m.editorVPath = strings.TrimSpace(vpath)
	m.editorDirty = false
	m.editorErr = ""
	m.editorNotice = ""
	m.editorBuf.SetValue("")
	m.editorBuf.Focus()
	m.layout()

	return func() tea.Msg {
		acc, ok := m.runner.(vfsAccessor)
		if !ok {
			return editorLoadMsg{vpath: vpath, err: fmt.Errorf("vfs access not available")}
		}
		txt, _, truncated, err := acc.ReadVFS(m.ctx, vpath, 512*1024)
		if err != nil {
			// Missing file is a valid workflow: open a new file and allow saving it.
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
				return editorLoadMsg{vpath: vpath, content: "", notice: "new file"}
			}
			return editorLoadMsg{vpath: vpath, err: err}
		}
		if truncated {
			return editorLoadMsg{vpath: vpath, err: fmt.Errorf("file too large to edit in TUI (truncated)")}
		}
		return editorLoadMsg{vpath: vpath, content: txt}
	}
}

func (m *Model) editorExecCmd(editor string, vpath string) (*exec.Cmd, error) {
	editor = strings.TrimSpace(editor)
	vpath = strings.TrimSpace(vpath)
	if editor == "" || vpath == "" {
		return nil, fmt.Errorf("editor and vpath are required")
	}
	if !strings.HasPrefix(vpath, "/workdir/") {
		return nil, fmt.Errorf("external editor only supports /workdir paths")
	}
	workdir := strings.TrimSpace(m.workdir)
	if workdir == "" {
		return nil, fmt.Errorf("workdir is unknown")
	}

	sub := strings.TrimPrefix(vpath, "/workdir/")
	clean, _, err := vfsutil.NormalizeResourceSubpath(sub)
	if err != nil || clean == "" || clean == "." {
		return nil, fmt.Errorf("invalid workdir path: %s", vpath)
	}
	abs := filepath.Join(workdir, filepath.FromSlash(clean))

	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return nil, fmt.Errorf("invalid editor")
	}
	cmd := exec.CommandContext(m.ctx, fields[0], append(fields[1:], abs)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func (m *Model) loadComposeBuffer(vpath string) tea.Cmd {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		vpath = composeVPath
	}
	// Compose buffer always lives under /workdir.
	if !strings.HasPrefix(vpath, "/workdir/") {
		return func() tea.Msg {
			return editorComposeLoadMsg{vpath: composeVPath, err: fmt.Errorf("compose buffer must be under /workdir")}
		}
	}
	workdir := strings.TrimSpace(m.workdir)
	if workdir == "" {
		return func() tea.Msg {
			return editorComposeLoadMsg{vpath: composeVPath, err: fmt.Errorf("workdir is unknown")}
		}
	}
	sub := strings.TrimPrefix(vpath, "/workdir/")
	clean, _, err := vfsutil.NormalizeResourceSubpath(sub)
	if err != nil || clean == "" || clean == "." {
		return func() tea.Msg {
			return editorComposeLoadMsg{vpath: composeVPath, err: fmt.Errorf("invalid compose path: %s", vpath)}
		}
	}
	abs := filepath.Join(workdir, filepath.FromSlash(clean))
	return func() tea.Msg {
		b, err := os.ReadFile(abs)
		if err != nil {
			return editorComposeLoadMsg{vpath: composeVPath, err: err}
		}
		return editorComposeLoadMsg{vpath: composeVPath, text: string(b)}
	}
}

func (m *Model) openComposeEditorPrefill() tea.Cmd {
	// Require a known workdir so we can write the compose buffer.
	workdir := strings.TrimSpace(m.workdir)
	if workdir == "" {
		// Best-effort: try to prefetch it; otherwise no-op.
		return m.prefetchWorkdir()
	}

	// Read current input value (single or multiline).
	cur := ""
	if m.isMulti {
		cur = m.multiline.Value()
	} else {
		cur = m.single.Value()
	}

	composeRel := filepath.FromSlash(".workbench/compose.md")
	composeAbs := filepath.Join(workdir, composeRel)
	_ = os.MkdirAll(filepath.Dir(composeAbs), 0755)
	_ = os.WriteFile(composeAbs, []byte(cur), 0644)

	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		// No external editor configured; fall back to internal editor UX.
		m.externalEditorComposeVPath = composeVPath
		return m.openInternalEditor(composeVPath)
	}

	m.externalEditorComposeVPath = composeVPath
	if cmd, err := m.editorExecCmd(editor, composeVPath); err == nil && cmd != nil {
		return tea.ExecProcess(cmd, func(err error) tea.Msg {
			return editorExternalDoneMsg{vpath: composeVPath, err: err}
		})
	}
	return nil
}

func (m *Model) saveEditor() tea.Cmd {
	vpath := strings.TrimSpace(m.editorVPath)
	if vpath == "" {
		return nil
	}
	data := []byte(m.editorBuf.Value())
	return func() tea.Msg {
		acc, ok := m.runner.(vfsAccessor)
		if !ok {
			return editorSaveMsg{vpath: vpath, err: fmt.Errorf("vfs access not available")}
		}
		if err := acc.WriteVFS(m.ctx, vpath, data); err != nil {
			return editorSaveMsg{vpath: vpath, err: err}
		}
		return editorSaveMsg{vpath: vpath}
	}
}

func (m *Model) toggleMultiline() {
	m.isMulti = !m.isMulti
	if m.isMulti {
		m.multiline.SetValue(strings.TrimSpace(m.single.Value()))
		m.single.SetValue("")
		m.multiline.Focus()
	} else {
		m.single.SetValue(strings.TrimSpace(m.multiline.Value()))
		m.multiline.SetValue("")
		m.single.Focus()
	}
	m.layout()
}

func (m *Model) submitSingle() tea.Cmd {
	txt := strings.TrimSpace(m.single.Value())
	m.single.SetValue("")
	if txt == "" {
		return nil
	}
	// Intercept `/model` with no args to open picker instead of submitting
	if txt == "/model" {
		return m.openModelPicker()
	}
	return m.submit(txt)
}

func (m *Model) submitMultiline() tea.Cmd {
	txt := strings.TrimSpace(m.multiline.Value())
	m.multiline.SetValue("")
	if txt == "" {
		return nil
	}
	// Intercept `/model` with no args to open picker instead of submitting
	if txt == "/model" {
		return m.openModelPicker()
	}
	return m.submit(txt)
}

func (m *Model) submit(userMsg string) tea.Cmd {
	m.turnInFlight = true
	m.turnStarted = time.Now()
	m.turnTitle = userMsg
	m.turnN++
	m.pendingActionIdx = -1
	m.pendingActionText = ""
	m.waitingForAction = false

	if m.workflowTitle == "" {
		m.workflowTitle = firstLine(userMsg)
	}

	m.lastTurnUserItemIdx = len(m.transcriptItems)
	m.addTranscriptItem(transcriptItem{kind: transcriptUser, text: userMsg})

	return func() tea.Msg {
		final, err := m.runner.RunTurn(m.ctx, userMsg)
		_ = final
		return turnDoneMsg{final: final, err: err}
	}
}

func (m *Model) appendDetails(line string) {
	_ = line
}

func (m *Model) onEvent(ev events.Event) {
	rr := classifyEvent(ev)
	m.observeActivityEvent(ev)

	// Session title can come from run.started event.
	if ev.Type == "run.started" {
		if v := strings.TrimSpace(ev.Data["sessionTitle"]); v != "" {
			m.sessionTitle = v
		}
		if v := strings.TrimSpace(ev.Data["sessionId"]); v != "" {
			m.sessionID = v
		}
		if v := strings.TrimSpace(ev.Data["runId"]); v != "" {
			m.runID = v
		}
	}
	// Model identifier comes from agent.loop.start (host source of truth).
	if ev.Type == "agent.loop.start" {
		if v := strings.TrimSpace(ev.Data["model"]); v != "" {
			m.modelID = v
		}
	}
	// Model identifier can change at runtime via the host /model command.
	if ev.Type == "model.changed" {
		if v := strings.TrimSpace(ev.Data["to"]); v != "" {
			m.modelID = v
		}
	}
	// Workdir is discovered via host.mounted and updated via /cd at runtime.
	if ev.Type == "host.mounted" {
		if wd := strings.TrimSpace(ev.Data["/workdir"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "workdir.changed" {
		if wd := strings.TrimSpace(ev.Data["to"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "workdir.pwd" {
		if wd := strings.TrimSpace(ev.Data["workdir"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "ui.editor.open" {
		// Pre-run /editor includes the absolute workdir so the TUI can resolve
		// and open $VISUAL/$EDITOR without creating a run.
		if wd := strings.TrimSpace(ev.Data["workdir"]); wd != "" {
			m.workdir = wd
		}
	}

	// If a file picker is open and the workdir changed/arrived, refresh it.
	if m.filePickerOpen {
		wdNow := strings.TrimSpace(m.workdir)
		if wdNow != "" && wdNow != m.filePickerWorkdir {
			if all, err := scanWorkdirFiles(wdNow, 10000); err == nil {
				m.filePickerAllPaths = all
				m.filePickerWorkdir = wdNow
				m.applyFilePickerQuery(m.filePickerQuery)
				if strings.Contains(m.filePickerList.Title, "loading") {
					m.filePickerList.Title = "Select File"
				}
			}
		}
	}

	// Chrome metrics only (never rendered as transcript lines).
	switch ev.Type {
	case "llm.usage.total":
		m.lastTurnTokensIn = parseInt(ev.Data["input"])
		m.lastTurnTokensOut = parseInt(ev.Data["output"])
		m.lastTurnTokens = parseInt(ev.Data["total"])
		m.totalTokens += m.lastTurnTokens
	case "llm.cost.total":
		known := parseBool(ev.Data["known"])
		m.lastTurnCostUSD = strings.TrimSpace(ev.Data["costUsd"])
		if !known && m.lastTurnCostUSD == "" {
			m.lastTurnCostUSD = "?"
		}
		if v := strings.TrimSpace(ev.Data["costUsd"]); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				m.totalCostUSD += f
			}
		}
	case "agent.turn.complete":
		m.lastTurnDuration = strings.TrimSpace(ev.Data["duration"])
		m.lastTurnSteps = strings.TrimSpace(ev.Data["steps"])
	}

	// Chat transcript: only compact action summaries, paired request+response.
	if rr.Class != RenderAction {
		return
	}

	switch ev.Type {
	case "agent.op.request":
		txt := strings.TrimSpace(rr.Text)
		if txt == "" {
			return
		}
		m.pendingActionText = txt
		m.pendingActionIsToolRun = strings.TrimSpace(ev.Data["op"]) == "tool.run"
		m.waitingForAction = true
		m.pendingActionIdx = len(m.transcriptItems)
		m.addTranscriptItem(transcriptItem{
			kind:            transcriptAction,
			actionText:      txt,
			actionIsToolRun: m.pendingActionIsToolRun,
		})
	case "agent.op.response":
		if !m.waitingForAction || m.pendingActionIdx < 0 {
			return
		}
		comp := strings.TrimSpace(rr.Text)
		if m.pendingActionIdx < len(m.transcriptItems) {
			it := m.transcriptItems[m.pendingActionIdx]
			if it.kind == transcriptAction && !it.actionIsCompleted {
				it.actionCompletion = comp
				it.actionIsCompleted = true
				m.transcriptItems[m.pendingActionIdx] = it
			}
		}
		m.pendingActionText = ""
		m.waitingForAction = false
		m.pendingActionIdx = -1
		m.pendingActionIsToolRun = false
		m.rebuildTranscript()
	default:
		txt := strings.TrimSpace(rr.Text)
		if txt == "" {
			return
		}
		// Host-side command errors should appear as errors in the transcript.
		if ev.Type == "workdir.error" {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: txt})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			return
		}
		comp := "✓ ok"
		if ev.Type == "refs.ambiguous" || ev.Type == "refs.unresolved" {
			comp = "✗"
		}
		m.addTranscriptItem(transcriptItem{
			kind:              transcriptAction,
			actionText:        txt,
			actionCompletion:  comp,
			actionIsCompleted: true,
		})
	}
}

func (m Model) waitEvent() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-m.events
		if !ok {
			return nil
		}
		return eventMsg(ev)
	}
}

// Run starts the Workbench Bubble Tea program.
func Run(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) error {
	if runner == nil {
		return fmt.Errorf("tui runner is required")
	}
	m := New(ctx, runner, evCh)

	// Activity panel: off by default. Enable via:
	//   - env: WORKBENCH_ACTIVITY=true/false
	//   - flag: --activity (wired in cmd/workbench)
	enableActivity := strings.TrimSpace(os.Getenv("WORKBENCH_ACTIVITY"))
	if enableActivity != "" {
		m.showDetails = enableActivity == "1" || strings.EqualFold(enableActivity, "true") || strings.EqualFold(enableActivity, "yes")
	}

	// Mouse capture enables mouse wheel / trackpad scrolling in the transcript.
	//
	// Note: enabling xterm mouse tracking often disables native terminal click+drag
	// selection unless your terminal supports shift-drag selection. Workbench defaults
	// to mouse scrolling; set WORKBENCH_MOUSE=false (or --mouse=false) to restore
	// native selection behavior.
	//
	// Mouse mode is opt-in via:
	//   - env: WORKBENCH_MOUSE=true/false
	//   - flag: --mouse (wired in cmd/workbench)
	enableMouse := strings.TrimSpace(os.Getenv("WORKBENCH_MOUSE"))
	mouseOn := true
	if enableMouse != "" {
		mouseOn = enableMouse == "1" || strings.EqualFold(enableMouse, "true") || strings.EqualFold(enableMouse, "yes")
	}

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if mouseOn {
		// Cell motion is enough for wheel/trackpad scrolling and reduces event spam
		// compared to "all motion".
		opts = append(opts, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(m, opts...)
	_, err := p.Run()
	return err
}
