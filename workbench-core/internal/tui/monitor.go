package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	layoutmgr "github.com/tinoosan/workbench-core/internal/tui/layout"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type tailedEventMsg struct {
	ev store.TailedEvent
}

type tailErrMsg struct {
	err error
}

type commandLinesMsg struct {
	lines []string
}

type taskQueuedLocallyMsg struct {
	TaskID string
	Goal   string
}

type tickMsg struct {
	now time.Time
}

type monitorModel struct {
	ctx       context.Context
	cfg       config.Config
	runID     string
	runStatus types.RunStatus // loaded at init; used to show "run not active" warning

	offset int64

	input textarea.Model

	activities        []Activity
	activityIndexByID map[string]int
	activityIndexByOp map[string]int
	activitySeq       int
	pendingActivityID string
	activityList      list.Model
	activityDetail    viewport.Model
	planViewport      viewport.Model
	renderer          *ContentRenderer
	agentOutput       []string
	agentOutputVP     viewport.Model
	inbox             map[string]taskState
	inboxVP           viewport.Model
	currentTask       *taskState
	outboxResults     []outboxEntry
	outboxVP          viewport.Model
	memResults        []string
	memoryVP          viewport.Model
	thinkingEntries   []thinkingEntry
	thinkingVP        viewport.Model
	planMarkdown      string
	planDetails       string
	planLoadErr       string
	planDetailsErr    string
	stats             monitorStats
	model             string
	profile           string
	focusedPanel      panelID
	compactTab        int // 0=Output, 1=Activity, 2=Plan, 3=Outbox; used when isCompactMode()
	dashboardSideTab  int // 0=Activity, 1=Plan, 2=Tasks, 3=Thoughts; used when dashboard mode
	width             int
	height            int
	styles            *monitorStyles
	tailCh            <-chan store.TailedEvent
	errCh             <-chan error
	cancel            context.CancelFunc

	// Modal overlay state (only one modal open at a time)
	helpModalOpen bool

	// Model picker
	modelPickerOpen bool
	modelPickerList list.Model

	// Profile picker
	profilePickerOpen bool
	profilePickerList list.Model

	// Command palette (inline autocomplete above composer)
	commandPaletteOpen     bool
	commandPaletteMatches  []string
	commandPaletteSelected int

	// Reasoning pickers
	reasoningEffortPickerOpen      bool
	reasoningEffortPickerSelected  int
	reasoningSummaryPickerOpen     bool
	reasoningSummaryPickerSelected int

	// File picker (for @ references)
	filePickerOpen     bool
	filePickerList     list.Model
	filePickerAllPaths []string
	filePickerQuery    string
}

type monitorStats struct {
	started         time.Time
	tasksDone       int
	lastTurnCostUSD string
	totalCostUSD    float64
}

type taskState struct {
	TaskID    string
	Goal      string
	Status    string
	StartedAt time.Time
}

type outboxEntry struct {
	TaskID         string
	Goal           string
	Status         string
	Summary        string
	Error          string
	SummaryPath    string
	ArtifactsCount int
	Timestamp      time.Time
}

type thinkingEntry struct {
	Summary string
}

type panelID int

const (
	panelActivity panelID = iota
	panelActivityDetail
	panelPlan
	panelCurrentTask
	panelOutput
	panelInbox
	panelOutbox
	panelMemory
	panelComposer
	panelThinking
)

// Breakpoints for responsive layout: below these use compact mode (tabs/single column).
const (
	compactModeMinWidth  = 110
	compactModeMinHeight = 32
)

func RunMonitor(ctx context.Context, cfg config.Config, runID string) error {
	m, err := newMonitorModel(ctx, cfg, runID)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newMonitorModel(ctx context.Context, cfg config.Config, runID string) (*monitorModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}

	stats := monitorStats{started: time.Now()}

	in := textarea.New()
	in.SetHeight(6)
	in.CharLimit = 0
	in.ShowLineNumbers = false
	in.FocusedStyle.CursorLine = lipgloss.NewStyle()
	in.FocusedStyle.Placeholder = kit.StyleDim
	in.FocusedStyle.Text = kit.StyleStatusValue
	in.FocusedStyle.Prompt = kit.StyleStatusKey
	in.Prompt = "> "
	in.Focus()

	delegate := newActivityDelegate()
	activityList := list.New([]list.Item{}, delegate, 0, 0)
	activityList.SetShowTitle(false)
	activityList.SetShowStatusBar(false)
	activityList.SetShowFilter(false)
	activityList.SetShowHelp(false)
	activityList.SetShowPagination(false)

	tctx, cancel := context.WithCancel(ctx)
	evs, off, _ := store.ListEvents(cfg, runID)
	tailCh, errCh := store.TailEvents(cfg, tctx, runID, off)

	runStatus := types.StatusSucceeded
	if r, err := store.LoadRun(cfg, runID); err == nil {
		runStatus = r.Status
	}

	m := &monitorModel{
		ctx:               ctx,
		cfg:               cfg,
		runID:             runID,
		runStatus:         runStatus,
		offset:            off,
		input:             in,
		activities:        []Activity{},
		activityIndexByID: map[string]int{},
		activityIndexByOp: map[string]int{},
		activityList:      activityList,
		activityDetail:    viewport.New(0, 0),
		planViewport:      viewport.New(0, 0),
		renderer:          newContentRenderer(),
		agentOutput:       []string{},
		agentOutputVP:     viewport.New(0, 0),
		inbox:             map[string]taskState{},
		inboxVP:           viewport.New(0, 0),
		outboxResults:     []outboxEntry{},
		outboxVP:          viewport.New(0, 0),
		memResults:        []string{},
		memoryVP:          viewport.New(0, 0),
		thinkingEntries:   []thinkingEntry{},
		thinkingVP:        viewport.New(0, 0),
		stats:             stats,
		styles:            defaultMonitorStyles(),
		focusedPanel:      panelComposer,
		tailCh:            tailCh,
		errCh:             errCh,
		cancel:            cancel,
	}
	// Disable mouse handling so terminals don't enter mouse-reporting mode.
	m.activityDetail.MouseWheelEnabled = false
	m.planViewport.MouseWheelEnabled = false
	m.agentOutputVP.MouseWheelEnabled = false
	m.inboxVP.MouseWheelEnabled = false
	m.outboxVP.MouseWheelEnabled = false
	m.memoryVP.MouseWheelEnabled = false
	m.thinkingVP.MouseWheelEnabled = false

	for _, e := range evs {
		m.observeEvent(e)
	}
	pending, _ := loadPendingTasksFromInbox(cfg, runID)
	for _, ts := range pending {
		m.inbox[ts.TaskID] = ts
	}
	m.refreshActivityList()
	m.loadPlanFiles()
	m.refreshViewports()

	return m, nil
}

// loadPendingTasksFromInbox reads task-*.json from the run's inbox and returns
// taskState slices with Status pending. Used so the queue shows tasks added
// before the monitor started or via webhook.
func loadPendingTasksFromInbox(cfg config.Config, runID string) ([]taskState, error) {
	inboxDir := filepath.Join(fsutil.GetAgentDir(cfg.DataDir, runID), "inbox")
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []taskState
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if strings.HasPrefix(name, "control-") {
			continue
		}
		if strings.Contains(name, "poison") || strings.Contains(name, "archive") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(inboxDir, e.Name()))
		if err != nil {
			continue
		}
		var t types.Task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		taskID := strings.TrimSpace(t.TaskID)
		if taskID == "" {
			taskID = strings.TrimSuffix(name, ".json")
		}
		out = append(out, taskState{
			TaskID: taskID,
			Goal:   strings.TrimSpace(t.Goal),
			Status: string(types.TaskStatusPending),
		})
	}
	return out, nil
}

func (m *monitorModel) Init() tea.Cmd {
	return tea.Batch(m.listenEvent(), m.listenErr(), m.tick())
}

func (m *monitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshViewports()
		return m, nil

	case tickMsg:
		// Re-render time-based UI (uptime, elapsed timers) even when no new events arrive.
		m.refreshViewports()
		return m, m.tick()

	case tailedEventMsg:
		if msg.ev.Event.EventID != "" {
			m.offset = msg.ev.NextOffset
			m.observeEvent(msg.ev.Event)
		}
		m.refreshViewports()
		return m, m.listenEvent()

	case tailErrMsg:
		if msg.err != nil {
			m.appendAgentOutput("[error] " + msg.err.Error())
		}
		m.refreshViewports()
		return m, m.listenErr()

	case commandLinesMsg:
		if len(msg.lines) != 0 {
			for _, line := range msg.lines {
				m.appendAgentOutput(line)
			}
			if strings.HasPrefix(strings.TrimSpace(msg.lines[0]), "[memory] search:") {
				m.memResults = msg.lines[1:]
			}
		}
		m.refreshViewports()
		return m, nil

	case monitorEditorDoneMsg:
		m.handleEditorDone(msg)
		m.refreshViewports()
		return m, nil

	case taskQueuedLocallyMsg:
		m.inbox[msg.TaskID] = taskState{TaskID: msg.TaskID, Goal: msg.Goal, Status: string(types.TaskStatusPending)}
		m.refreshViewports()
		return m, nil

	case monitorFilePickerPathsMsg:
		m.handleFilePickerPaths(msg.paths)
		return m, nil

	case tea.KeyMsg:
		// Modal overlay handling - if any modal is open, handle it first
		if m.helpModalOpen {
			switch msg.String() {
			case "esc", "escape", "?":
				m.closeHelpModal()
				return m, nil
			}
			return m, nil // Consume all other keys when help is open
		}
		if m.profilePickerOpen {
			return m.updateProfilePicker(msg)
		}
		if m.modelPickerOpen {
			return m.updateModelPicker(msg)
		}
		if m.reasoningEffortPickerOpen {
			return m.updateReasoningEffortPicker(msg)
		}
		if m.reasoningSummaryPickerOpen {
			return m.updateReasoningSummaryPicker(msg)
		}
		if m.filePickerOpen {
			return m.updateFilePicker(msg)
		}

		// Help modal hotkey
		if msg.String() == "?" && m.focusedPanel != panelComposer {
			m.openHelpModal()
			return m, nil
		}

		if m.focusedPanel == panelComposer {
			// Handle command palette key events first
			if cmd, ok := m.handleCommandPaletteKey(msg); ok {
				return m, cmd
			}

			if strings.EqualFold(msg.String(), "ctrl+e") {
				seed := strings.TrimSpace(m.input.Value())
				m.input.SetValue("")
				return m, m.openComposeEditor(seed)
			}

			key := strings.ToLower(msg.String())
			if key == "ctrl+enter" ||
				key == "ctrl+j" ||
				key == "ctrl+m" ||
				key == "ctrl+o" ||
				msg.Type == tea.KeyCtrlJ ||
				msg.Type == tea.KeyCtrlM ||
				msg.Type == tea.KeyCtrlO ||
				(msg.Type == tea.KeyEnter && msg.Alt) {
				cmd := strings.TrimSpace(m.input.Value())
				m.input.SetValue("")
				if cmd == "" {
					return m, nil
				}
				return m, m.handleCommand(cmd)
			}
		}
		// Compact mode: allow switching tabs and focusing Activity subpanels so
		// long details can be scrolled.
		if m.isCompactMode() {
			switch msg.String() {
			case "ctrl+]":
				m.compactTab = (m.compactTab + 1) % len(compactTabNames)
				if m.focusedPanel != panelComposer {
					m.focusedPanel = m.compactTabToPanel()
				}
				m.updateFocus()
				m.refreshViewports()
				return m, nil
			case "ctrl+[":
				m.compactTab = (m.compactTab + len(compactTabNames) - 1) % len(compactTabNames)
				if m.focusedPanel != panelComposer {
					m.focusedPanel = m.compactTabToPanel()
				}
				m.updateFocus()
				m.refreshViewports()
				return m, nil
			case "ctrl+down", "ctrl+j":
				if m.compactTab == 1 && m.focusedPanel != panelComposer { // Activity tab
					m.focusedPanel = panelActivityDetail
					m.updateFocus()
					return m, nil
				}
			case "ctrl+up", "ctrl+k":
				if m.compactTab == 1 && m.focusedPanel != panelComposer { // Activity tab
					m.focusedPanel = panelActivity
					m.updateFocus()
					return m, nil
				}
			}
		}
		// Dashboard mode: quick focus toggle between Activity Feed and Details.
		if !m.isCompactMode() && m.dashboardSideTab == 0 && m.focusedPanel != panelComposer {
			switch msg.String() {
			case "ctrl+down":
				m.focusedPanel = panelActivityDetail
				m.updateFocus()
				return m, nil
			case "ctrl+up":
				m.focusedPanel = panelActivity
				m.updateFocus()
				return m, nil
			}
		}
		switch msg.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "ctrl+y":
			if !m.isCompactMode() {
				m.dashboardSideTab = 3
				m.focusedPanel = panelThinking
				m.updateFocus()
				m.refreshViewports()
				return m, nil
			}
		case "tab", "shift+tab":
			if m.isCompactMode() {
				// Toggle focus between the Composer and the current tab's panel.
				if m.focusedPanel == panelComposer {
					m.focusedPanel = m.compactTabToPanel()
				} else {
					m.focusedPanel = panelComposer
				}
				m.updateFocus()
				return m, nil
			}
			cycle := m.dashboardTabFocusCycle()
			if len(cycle) == 0 {
				cycle = []panelID{panelComposer}
			}
			idx := slices.Index(cycle, m.focusedPanel)
			if idx < 0 {
				idx = 0
			}
			switch msg.String() {
			case "tab":
				idx = (idx + 1) % len(cycle)
			case "shift+tab":
				idx = (idx + len(cycle) - 1) % len(cycle)
			}
			m.focusedPanel = cycle[idx]
			m.syncDashboardSideTabFromFocus()
			m.updateFocus()
			return m, nil
		case "ctrl+]":
			if !m.isCompactMode() {
				m.dashboardSideTab = (m.dashboardSideTab + 1) % len(dashboardSideTabNames)
				m.focusedPanel = m.dashboardSideTabToPanel()
				m.updateFocus()
				return m, nil
			}
		case "ctrl+[":
			if !m.isCompactMode() {
				m.dashboardSideTab = (m.dashboardSideTab + len(dashboardSideTabNames) - 1) % len(dashboardSideTabNames)
				m.focusedPanel = m.dashboardSideTabToPanel()
				m.updateFocus()
				return m, nil
			}
		}
		if m.isCompactMode() {
			// Keep explicit focus on Activity Details if the user selected it.
			if !(m.compactTab == 1 && m.focusedPanel == panelActivityDetail) {
				m.focusedPanel = m.compactTabToPanel()
			}
			m.updateFocus()
		}
		return m.routeKeyToFocusedPanel(msg)
	}

	return m, nil
}

func (m *monitorModel) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg{now: t} })
}

func (m *monitorModel) isCompactMode() bool {
	return m.width < compactModeMinWidth || m.height < compactModeMinHeight
}

func (m *monitorModel) View() string {
	grid := m.layout()
	headerLine := m.renderHeader()

	var base string
	if m.isCompactMode() {
		base = m.renderCompact(grid, headerLine)
	} else {
		base = m.renderDashboard(grid, headerLine)
	}

	// Render modal overlays on top
	if m.helpModalOpen {
		return m.renderHelpModal(base)
	}
	if m.profilePickerOpen {
		return m.renderProfilePicker(base)
	}
	if m.modelPickerOpen {
		return m.renderModelPicker(base)
	}
	if m.reasoningEffortPickerOpen {
		return m.renderReasoningEffortPicker(base)
	}
	if m.reasoningSummaryPickerOpen {
		return m.renderReasoningSummaryPicker(base)
	}
	if m.filePickerOpen {
		return m.renderFilePicker(base)
	}

	return base
}

func (m *monitorModel) renderDashboard(grid layoutmgr.GridLayout, headerLine string) string {
	main := m.renderMainBodyDashboard(grid)
	statusBar := m.renderStatusBar(grid.ScreenWidth)
	composer := m.renderComposer(grid.Composer)
	stats := m.renderStatsInline(grid.Stats.Width)
	sections := []string{headerLine, "", main, "", composer, stats, statusBar}
	if m.runStatus != types.StatusRunning {
		w := m.width
		if w <= 0 {
			w = 80
		}
		warning := m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render("Agent is not active; start the daemon first or use --agent-id to attach to the running agent."))
		sections = []string{headerLine, warning, "", main, "", composer, stats, statusBar}
	}
	final := lipgloss.JoinVertical(lipgloss.Left, sections...)
	// Guarantee view never exceeds terminal width (handles m.width==0 or any section overflow).
	effectiveWidth := m.width
	if effectiveWidth <= 0 {
		effectiveWidth = 80
	}
	return lipgloss.NewStyle().MaxWidth(effectiveWidth).Render(final)
}

func (m *monitorModel) renderStatusBar(width int) string {
	line := "Tab: focus  |  Ctrl+Enter: submit  |  /quit"
	if m.isCompactMode() {
		line += "  |  Ctrl+]/Ctrl+[ switch tab (Output | Activity | Plan | Outbox)  |  Ctrl+Up/Down focus Activity Feed/Details"
	} else {
		line += "  |  Ctrl+]/Ctrl+[ cycle side panel (Activity | Plan | Tasks | Thoughts)  |  Ctrl+Y Thoughts tab  |  Ctrl+Up/Down focus Activity Feed/Details"
	}
	w := width
	if w <= 0 {
		w = 80
	}
	// Keep the status bar single-line; wrapping changes height and breaks layout budgets.
	line = kit.TruncateRight(line, max(1, w-2))
	return m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render(line))
}

// compactTabNames for the tab bar in compact mode.
var compactTabNames = []string{"Output", "Activity", "Plan", "Outbox"}

func (m *monitorModel) compactTabToPanel() panelID {
	switch m.compactTab {
	case 0:
		return panelOutput
	case 1:
		return panelActivity
	case 2:
		return panelPlan
	case 3:
		return panelOutbox
	case 4:
		return panelComposer
	default:
		return panelOutput
	}
}

// dashboardSideTabNames for the side-panel tab bar in dashboard mode.
var dashboardSideTabNames = []string{"Activity", "Plan", "Tasks", "Thoughts"}

func (m *monitorModel) dashboardSideTabToPanel() panelID {
	switch m.dashboardSideTab {
	case 0:
		return panelActivity
	case 1:
		return panelPlan
	case 2:
		return panelCurrentTask
	case 3:
		return panelThinking
	default:
		return panelActivity
	}
}

func (m *monitorModel) syncDashboardSideTabFromFocus() {
	switch m.focusedPanel {
	case panelActivity, panelActivityDetail:
		m.dashboardSideTab = 0
	case panelPlan:
		m.dashboardSideTab = 1
	case panelInbox, panelOutbox, panelCurrentTask:
		m.dashboardSideTab = 2
	case panelThinking:
		m.dashboardSideTab = 3
	}
}

func (m *monitorModel) dashboardTabFocusCycle() []panelID {
	switch m.dashboardSideTab {
	case 0:
		return []panelID{
			panelComposer,
			panelOutput,
			panelActivity,
			panelActivityDetail,
		}
	case 1:
		return []panelID{
			panelComposer,
			panelOutput,
			panelPlan,
		}
	case 2:
		return []panelID{
			panelComposer,
			panelOutput,
			panelCurrentTask,
			panelInbox,
			panelOutbox,
		}
	case 3:
		return []panelID{
			panelComposer,
			panelOutput,
			panelThinking,
		}
	default:
		return []panelID{
			panelComposer,
			panelOutput,
			panelActivity,
			panelActivityDetail,
		}
	}
}

// renderCompact builds the view for compact mode: header + tab bar + main content + composer.
func (m *monitorModel) renderCompact(grid layoutmgr.GridLayout, headerLine string) string {
	tabBar := m.renderCompactTabBar()
	// Each tab renders its own panel(s) so subpanels can be focused and scrolled.
	content := m.renderCompactTabContent(grid)
	sections := []string{headerLine, tabBar, content, m.renderComposer(grid.Composer)}
	final := lipgloss.JoinVertical(lipgloss.Left, sections...)
	effectiveWidth := m.width
	if effectiveWidth <= 0 {
		effectiveWidth = 80
	}
	return lipgloss.NewStyle().MaxWidth(effectiveWidth).Render(final)
}

func (m *monitorModel) renderCompactTabBar() string {
	parts := make([]string, len(compactTabNames))
	for i, name := range compactTabNames {
		// Only highlight a tab if it's selected. If m.compactTab == 4 (Composer), all tabs are dim.
		if i == m.compactTab {
			parts[i] = m.styles.sectionTitle.Render(name)
		} else {
			parts[i] = kit.StyleDim.Render(name)
		}
	}
	w := m.width
	if w <= 0 {
		w = 80
	}
	return m.styles.header.Copy().MaxWidth(w).Render(strings.Join(parts, "  |  "))
}

func (m *monitorModel) renderCompactTabContent(grid layoutmgr.GridLayout) string {
	switch m.compactTab {
	case 0:
		return m.panelStyle(panelOutput).
			Width(grid.AgentOutput.InnerWidth()).
			Height(grid.AgentOutput.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Agent Output") + "\n" + m.agentOutputVP.View())
	case 1:
		feed := m.panelStyle(panelActivity).
			Width(grid.ActivityFeed.InnerWidth()).
			Height(grid.ActivityFeed.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Feed") + "\n" + m.activityList.View())
		detail := m.panelStyle(panelActivityDetail).
			Width(grid.ActivityDetail.InnerWidth()).
			Height(grid.ActivityDetail.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Details") + "\n" + m.activityDetail.View())
		return lipgloss.JoinVertical(lipgloss.Left, feed, detail)
	case 2:
		return m.panelStyle(panelPlan).
			Width(grid.Plan.InnerWidth()).
			Height(grid.Plan.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Plan") + "\n" + m.planViewport.View())
	case 3:
		return m.panelStyle(panelOutbox).
			Width(grid.Outbox.InnerWidth()).
			Height(grid.Outbox.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Outbox (Recent Results)") + "\n" + m.outboxVP.View())
	default:
		return m.panelStyle(panelOutput).
			Width(grid.AgentOutput.InnerWidth()).
			Height(grid.AgentOutput.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Agent Output") + "\n" + m.agentOutputVP.View())
	}
}

func (m *monitorModel) listenEvent() tea.Cmd {
	return func() tea.Msg {
		if m.tailCh == nil {
			time.Sleep(250 * time.Millisecond)
			return tailedEventMsg{}
		}
		ev, ok := <-m.tailCh
		if !ok {
			time.Sleep(250 * time.Millisecond)
			return tailedEventMsg{}
		}
		return tailedEventMsg{ev: ev}
	}
}

func (m *monitorModel) listenErr() tea.Cmd {
	return func() tea.Msg {
		if m.errCh == nil {
			time.Sleep(250 * time.Millisecond)
			return tailErrMsg{}
		}
		err, ok := <-m.errCh
		if !ok {
			time.Sleep(250 * time.Millisecond)
			return tailErrMsg{}
		}
		return tailErrMsg{err: err}
	}
}

func (m *monitorModel) handleCommand(raw string) tea.Cmd {
	raw = strings.TrimSpace(raw)
	cmd, rest := splitMonitorCommand(raw)
	if cmd == "" {
		return nil
	}

	if cmd == "/quit" {
		if m.cancel != nil {
			m.cancel()
		}
		return tea.Quit
	}

	if cmd == "/help" {
		m.openHelpModal()
		return nil
	}

	if cmd == "/editor" {
		return m.openComposeEditor("")
	}

	if cmd == "/task" {
		goal := strings.TrimSpace(rest)
		goal = strings.Trim(goal, "\"")
		return m.enqueueTask(goal, 0)
	}
	if cmd == "/profile" && strings.TrimSpace(rest) == "" {
		return m.openProfilePicker()
	}
	if cmd == "/profile" && strings.TrimSpace(rest) != "" {
		ref := strings.TrimSpace(rest)
		return m.writeControl("switch_profile", map[string]any{"profile": ref})
	}

	// /model with no arg opens picker, with arg sets directly
	if cmd == "/model" && strings.TrimSpace(rest) == "" {
		return m.openModelPicker()
	}
	if cmd == "/model" && strings.TrimSpace(rest) != "" {
		model := strings.TrimSpace(rest)
		return m.writeControl("set_model", map[string]any{"model": model})
	}

	// Reasoning commands
	if cmd == "/reasoning-effort" {
		m.openReasoningEffortPicker()
		return nil
	}
	if cmd == "/reasoning-summary" {
		m.openReasoningSummaryPicker()
		return nil
	}

	if cmd == "/memory search" {
		query := strings.TrimSpace(rest)
		query = strings.Trim(query, "\"")
		return m.searchMemory(query)
	}

	// Unknown commands are logged to the view.
	return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] " + raw}} }
}

func (m *monitorModel) enqueueTask(goal string, priority int) tea.Cmd {
	return func() tea.Msg {
		goal = strings.TrimSpace(goal)
		if goal == "" {
			return commandLinesMsg{lines: []string{"[queued] error: goal is empty"}}
		}
		runDir := fsutil.GetAgentDir(m.cfg.DataDir, m.runID)
		inboxDir := filepath.Join(runDir, "inbox")
		_ = os.MkdirAll(inboxDir, 0755)
		now := time.Now()
		id := "task-" + uuid.NewString()
		task := types.Task{
			TaskID:    id,
			Goal:      goal,
			Priority:  priority,
			Status:    types.TaskStatusPending,
			CreatedAt: &now,
		}
		b, _ := json.MarshalIndent(task, "", "  ")
		if err := os.WriteFile(filepath.Join(inboxDir, id+".json"), b, 0644); err != nil {
			return commandLinesMsg{lines: []string{"[queued] error: " + err.Error()}}
		}
		return tea.Batch(
			func() tea.Msg {
				return commandLinesMsg{lines: []string{"[queued] " + id + " " + goal + " — task queued to run " + m.runID}}
			},
			func() tea.Msg { return taskQueuedLocallyMsg{TaskID: id, Goal: goal} },
		)
	}
}

func (m *monitorModel) writeControl(command string, args map[string]any) tea.Cmd {
	return func() tea.Msg {
		runDir := fsutil.GetAgentDir(m.cfg.DataDir, m.runID)
		inboxDir := filepath.Join(runDir, "inbox")
		_ = os.MkdirAll(inboxDir, 0755)
		payload := map[string]any{
			"type":    "control",
			"command": strings.TrimSpace(command),
		}
		if len(args) != 0 {
			payload["args"] = args
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		id := "control-" + uuid.NewString()
		if err := os.WriteFile(filepath.Join(inboxDir, id+".json"), b, 0644); err != nil {
			return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
		}
		return commandLinesMsg{lines: []string{"[control] queued " + id}}
	}
}

func (m *monitorModel) searchMemory(query string) tea.Cmd {
	return func() tea.Msg {
		query = strings.TrimSpace(query)
		if query == "" {
			return commandLinesMsg{lines: []string{"[memory] error: query is empty"}}
		}
		vm, err := store.NewVectorMemoryStore(m.cfg)
		if err != nil {
			return commandLinesMsg{lines: []string{"[memory] error: " + err.Error()}}
		}
		results, err := vm.Search(m.ctx, query, 5)
		if err != nil {
			return commandLinesMsg{lines: []string{"[memory] error: " + err.Error()}}
		}
		lines := []string{"[memory] search: " + query}
		for _, r := range results {
			lines = append(lines, fmt.Sprintf("  - %.3f %s (%s)", r.Score, r.Title, r.Path))
		}
		return commandLinesMsg{lines: lines}
	}
}

func (m *monitorModel) observeEvent(ev types.EventRecord) {
	if v := strings.TrimSpace(ev.Data["model"]); v != "" {
		m.model = v
	}
	if v := strings.TrimSpace(ev.Data["profile"]); v != "" {
		m.profile = v
	}
	if strings.HasPrefix(ev.Type, "agent.op.") {
		m.observeActivityEvent(ev)
	}
	m.observeTaskEvent(ev)
	m.observeAgentOutput(ev)
	switch ev.Type {
	case "agent.step":
		summary := strings.TrimSpace(ev.Data["reasoningSummary"])
		if summary != "" {
			m.thinkingEntries = append(m.thinkingEntries, thinkingEntry{Summary: summary})
			m.refreshThinkingViewport()
		}
	case "llm.cost.total":
		m.stats.lastTurnCostUSD = strings.TrimSpace(ev.Data["costUsd"])
		if v := strings.TrimSpace(ev.Data["costUsd"]); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				m.stats.totalCostUSD += f
			}
		}
	}
}

func (m *monitorModel) observeActivityEvent(ev types.EventRecord) {
	switch ev.Type {
	case "agent.op.request":
		op := strings.TrimSpace(ev.Data["op"])
		if op == "" {
			return
		}
		opID := strings.TrimSpace(ev.Data["opId"])
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()

		act := Activity{
			ID:            id,
			Kind:          op,
			Status:        ActivityPending,
			StartedAt:     now,
			Path:          strings.TrimSpace(ev.Data["path"]),
			MaxBytes:      strings.TrimSpace(ev.Data["maxBytes"]),
			ToolID:        strings.TrimSpace(ev.Data["toolId"]),
			ActionID:      strings.TrimSpace(ev.Data["actionId"]),
			InputJSON:     strings.TrimSpace(ev.Data["input"]),
			TextPreview:   strings.TrimSpace(ev.Data["textPreview"]),
			TextTruncated: strings.TrimSpace(ev.Data["textTruncated"]) == "true",
			TextRedacted:  strings.TrimSpace(ev.Data["textRedacted"]) == "true",
			TextIsJSON:    strings.TrimSpace(ev.Data["textIsJSON"]) == "true",
			TextBytes:     strings.TrimSpace(ev.Data["textBytes"]),
			Data:          ev.Data,
		}
		if op == "tool.run" {
			act.Command = strings.TrimSpace(renderToolRunTranscript(act.ToolID, act.ActionID, act.InputJSON))
			if act.Command != "" {
				act.Title = "Run " + act.Command
			} else {
				act.Title = "Run tool"
			}
		} else {
			act.Title = renderOpRequest(ev.Data)
		}

		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		if opID != "" {
			m.activityIndexByOp[opID] = len(m.activities) - 1
		} else {
			m.pendingActivityID = id
		}
		m.refreshActivityList()
		m.refreshActivityDetail()

	case "agent.op.response":
		if strings.TrimSpace(ev.Data["op"]) == "" {
			return
		}
		opID := strings.TrimSpace(ev.Data["opId"])
		idx := -1
		ok := false
		if opID != "" {
			idx, ok = m.activityIndexByOp[opID]
		}
		if !ok {
			idx, ok = m.activityIndexByID[m.pendingActivityID]
		}
		if !ok || idx < 0 || idx >= len(m.activities) {
			return
		}
		act := m.activities[idx]
		now := time.Now()

		act.Ok = strings.TrimSpace(ev.Data["ok"])
		act.Error = strings.TrimSpace(ev.Data["err"])
		act.CallID = strings.TrimSpace(ev.Data["callId"])
		act.OutputPreview = strings.TrimSpace(ev.Data["outputPreview"])
		act.BytesLen = strings.TrimSpace(ev.Data["bytesLen"])
		act.Truncated = strings.TrimSpace(ev.Data["truncated"]) == "true"

		fin := now
		act.FinishedAt = &fin
		act.Duration = fin.Sub(act.StartedAt)
		if act.Ok == "true" {
			act.Status = ActivityOK
		} else {
			act.Status = ActivityError
		}
		if act.Data == nil {
			act.Data = make(map[string]string)
		}
		for k, v := range ev.Data {
			act.Data[k] = v
		}

		m.activities[idx] = act
		if opID != "" {
			delete(m.activityIndexByOp, opID)
		} else {
			m.pendingActivityID = ""
		}
		if isPlanEvent(act.Kind, act.Path) && strings.TrimSpace(act.Ok) == "true" {
			m.loadPlanFiles()
		}
		m.refreshActivityList()
		m.refreshActivityDetail()
	}
}

func (m *monitorModel) observeTaskEvent(ev types.EventRecord) {
	switch ev.Type {
	case "task.queued", "task.generated":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.inbox[taskID] = taskState{
			TaskID: taskID,
			Goal:   strings.TrimSpace(ev.Data["goal"]),
			Status: string(types.TaskStatusPending),
		}
	case "webhook.task.queued":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.inbox[taskID] = taskState{
			TaskID: taskID,
			Goal:   strings.TrimSpace(ev.Data["goal"]),
			Status: string(types.TaskStatusPending),
		}
	case "task.start":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		ts := m.inbox[taskID]
		ts.TaskID = taskID
		ts.Goal = strings.TrimSpace(ev.Data["goal"])
		ts.Status = "active"
		ts.StartedAt = ev.Timestamp
		m.currentTask = &ts
		delete(m.inbox, taskID)
	case "task.done":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.currentTask = nil
		m.stats.tasksDone++
		artifact0 := strings.TrimSpace(ev.Data["artifact0"])
		artCount := 0
		if v := strings.TrimSpace(ev.Data["artifacts"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				artCount = n
			}
		}
		m.outboxResults = append(m.outboxResults, outboxEntry{
			TaskID:         taskID,
			Goal:           strings.TrimSpace(ev.Data["goal"]),
			Status:         strings.TrimSpace(ev.Data["status"]),
			Summary:        strings.TrimSpace(ev.Data["summary"]),
			Error:          strings.TrimSpace(ev.Data["error"]),
			SummaryPath:    artifact0,
			ArtifactsCount: artCount,
			Timestamp:      ev.Timestamp,
		})
		if len(m.outboxResults) > 10 {
			m.outboxResults = m.outboxResults[len(m.outboxResults)-10:]
		}
	case "task.quarantined":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.outboxResults = append(m.outboxResults, outboxEntry{
			TaskID:    taskID,
			Goal:      strings.TrimSpace(ev.Data["goal"]),
			Status:    "quarantined",
			Error:     strings.TrimSpace(ev.Data["error"]),
			Timestamp: ev.Timestamp,
		})
		if len(m.outboxResults) > 10 {
			m.outboxResults = m.outboxResults[len(m.outboxResults)-10:]
		}
	}
}

func (m *monitorModel) observeAgentOutput(ev types.EventRecord) {
	switch ev.Type {
	case "daemon.start", "daemon.stop", "daemon.control", "daemon.warning", "daemon.error", "daemon.runner.error":
		m.appendAgentOutput(formatEventLine(ev))
	case "task.queued", "task.generated", "task.start", "task.done", "task.quarantined", "task.delivered", "task.heartbeat.enqueued", "task.heartbeat.skipped":
		m.appendAgentOutput(formatEventLine(ev))
	case "control.check", "control.success", "control.error":
		m.appendAgentOutput(formatEventLine(ev))
	}
}

func (m *monitorModel) appendAgentOutput(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	m.agentOutput = append(m.agentOutput, line)
	if len(m.agentOutput) > 200 {
		m.agentOutput = m.agentOutput[len(m.agentOutput)-200:]
	}
}

func (m *monitorModel) refreshActivityList() {
	// Preserve selection unless the user was following the tail.
	prevIdx := m.activityList.Index()
	prevID := ""
	oldItems := m.activityList.Items()
	wasFollowingTail := false
	if len(oldItems) > 0 && prevIdx == len(oldItems)-1 {
		wasFollowingTail = true
	}
	if prevIdx >= 0 && prevIdx < len(m.activities) {
		prevID = m.activities[prevIdx].ID
	}

	items := make([]list.Item, 0, len(m.activities))
	for _, a := range m.activities {
		items = append(items, activityItem{act: a})
	}
	m.activityList.SetItems(items)
	if len(items) == 0 {
		return
	}

	selectIdx := 0
	if wasFollowingTail {
		selectIdx = len(items) - 1
	} else if strings.TrimSpace(prevID) != "" {
		selectIdx = -1
		for i := range m.activities {
			if m.activities[i].ID == prevID {
				selectIdx = i
				break
			}
		}
		if selectIdx < 0 {
			selectIdx = min(max(prevIdx, 0), len(items)-1)
		}
	} else {
		selectIdx = min(max(prevIdx, 0), len(items)-1)
	}
	m.activityList.Select(selectIdx)

	// Only force pagination when following the tail.
	if wasFollowingTail {
		if pages := m.activityList.Paginator.TotalPages; pages > 0 {
			m.activityList.Paginator.Page = pages - 1
			itemsOnPage := m.activityList.Paginator.ItemsOnPage(len(m.activityList.VisibleItems()))
			for itemsOnPage > 0 && m.activityList.Cursor() < itemsOnPage-1 {
				m.activityList.CursorDown()
			}
		}
	}
}

func (m *monitorModel) refreshActivityDetail() {
	if m.renderer == nil {
		return
	}
	if len(m.activities) == 0 || m.activityList.Index() < 0 || m.activityList.Index() >= len(m.activities) {
		m.activityDetail.SetContent("")
		m.activityDetail.GotoTop()
		return
	}
	w := imax(24, m.activityDetail.Width-4)
	header := "### Details\n\n"
	help := "_PgUp/PgDn scroll · use Activity to change selection_\n\n"
	act := m.activities[m.activityList.Index()]
	md := renderActivityDetailMarkdown(act, false, false)
	rendered := strings.TrimRight(m.renderer.RenderMarkdown(header+help+md, w), "\n")
	rendered = wrapViewportText(rendered, imax(10, m.activityDetail.Width))
	m.activityDetail.SetContent(rendered)
	m.activityDetail.GotoTop()
}

func (m *monitorModel) refreshPlanView() {
	if m.renderer == nil {
		return
	}
	w := imax(24, m.planViewport.Width-4)
	detailsBody := ""
	detailsText := strings.TrimSpace(m.planDetails)
	if detailsText == "" {
		if strings.TrimSpace(m.planDetailsErr) != "" {
			detailsBody = fmt.Sprintf("_Failed to load plan details: %s_", m.planDetailsErr)
		} else {
			detailsBody = "_No plan details have been created yet._"
		}
	} else {
		detailsBody = detailsText
	}

	currentStep := ""
	progress := ""
	checklistBody := ""
	planText := strings.TrimSpace(m.planMarkdown)
	if planText == "" {
		if strings.TrimSpace(m.planLoadErr) != "" {
			checklistBody = fmt.Sprintf("_Failed to load checklist: %s_", m.planLoadErr)
		} else {
			checklistBody = "_No checklist has been created yet._"
		}
	} else {
		highlighted, active, done, total := highlightPlanChecklist(m.planMarkdown)
		if active != "" {
			currentStep = fmt.Sprintf("_Current step: %s_\n\n", active)
		}
		if total > 0 {
			progress = fmt.Sprintf("_Progress: %d/%d complete._\n\n", done, total)
		}
		if strings.TrimSpace(m.planLoadErr) != "" {
			checklistBody = fmt.Sprintf("_Failed to load checklist: %s_\n\n%s", m.planLoadErr, highlighted)
		} else {
			checklistBody = highlighted
		}
	}

	detailsSection := "### Plan Details\n\n" + detailsBody
	checklistSection := "### Checklist\n\n" + currentStep + progress + checklistBody
	content := detailsSection + "\n\n---\n\n" + checklistSection
	if strings.TrimSpace(content) == "" {
		content = "_Plan view is preparing…_"
	}
	rendered := strings.TrimRight(m.renderer.RenderMarkdown(content, w), "\n")
	rendered = wrapViewportText(rendered, imax(10, m.planViewport.Width))
	m.planViewport.SetContent(rendered)
	m.planViewport.GotoTop()
}

func (m *monitorModel) refreshThinkingViewport() {
	if len(m.thinkingEntries) == 0 {
		m.thinkingVP.SetContent(kit.StyleDim.Render("No thoughts captured yet."))
		m.thinkingVP.GotoTop()
		return
	}

	// Timeline view: colored nodes with a dimmed vertical spine.
	w := imax(10, m.thinkingVP.Width)
	const prefixW = 4 // "● " or "│ "
	contentW := imax(1, w-prefixW)

	// Styles for the timeline
	nodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a371f7")) // Purple node
	spineStyle := kit.StyleDim
	titleStyle := kit.StyleBold

	out := make([]string, 0, len(m.thinkingEntries)*3)
	last := len(m.thinkingEntries) - 1

	for i, e := range m.thinkingEntries {
		summary := strings.TrimSpace(e.Summary)
		if summary == "" {
			continue
		}

		// Render content with markdown
		body := summary
		if m.renderer != nil {
			body = strings.TrimRight(m.renderer.RenderMarkdown(summary, contentW), "\n")
		}
		body = wrapViewportText(body, contentW)
		lines := strings.Split(body, "\n")
		if len(lines) == 0 {
			continue
		}

		// First line gets a colored bullet, rest get the spine
		// Use consistent prefix width: glyph (1 char) + space = 2 columns
		for lineIdx, line := range lines {
			line = strings.TrimRight(line, " ")
			if lineIdx == 0 {
				// Node with colored bullet and bold first line (● + space = 2 cols)
				out = append(out, nodeStyle.Render("●")+" "+titleStyle.Render(line))
			} else if i == last {
				// Last entry: blank prefix instead of spine (2 spaces = 2 cols)
				out = append(out, "  "+line)
			} else {
				// Continuation line with spine (│ + space = 2 cols)
				out = append(out, spineStyle.Render("│")+" "+line)
			}
		}

		// Spacer between entries (if not last)
		if i < last {
			out = append(out, spineStyle.Render("│"))
		}
	}

	if len(out) == 0 {
		m.thinkingVP.SetContent(kit.StyleDim.Render("No thoughts captured yet."))
		m.thinkingVP.GotoTop()
		return
	}

	m.thinkingVP.SetContent(strings.Join(out, "\n"))
	m.thinkingVP.GotoBottom()
}

func (m *monitorModel) renderHeader() string {
	content := lipgloss.JoinHorizontal(lipgloss.Left,
		m.styles.headerTitle.Render("Workbench - Always On "),
		kit.RenderTag(kit.TagOptions{Key: "Agent", Value: m.runID}),
	)
	w := m.width
	if w <= 0 {
		w = 80
	}
	return m.styles.header.Copy().MaxWidth(w).Render(content)
}

func (m *monitorModel) panelStyle(panel panelID) lipgloss.Style {
	if m.focusedPanel == panel {
		return m.styles.panelFocused
	}
	return m.styles.panel
}

// renderMainBodyDashboard builds the two-column dashboard: left = AgentOutput + Outbox,
// right = tabbed side panels (Activity | Plan | Tasks | Thoughts) rendered as their own boxes so
// each subpanel can be focused and scrolled.
func (m *monitorModel) renderMainBodyDashboard(grid layoutmgr.GridLayout) string {
	leftParts := []string{
		m.panelStyle(panelOutput).Width(grid.AgentOutput.InnerWidth()).Height(grid.AgentOutput.InnerHeight()).Render(
			m.styles.sectionTitle.Render("Agent Output") + "\n" + m.agentOutputVP.View(),
		),
	}
	left := lipgloss.JoinVertical(lipgloss.Left, leftParts...)

	tabBar := m.renderDashboardSidePanelTabBar(grid)
	rightContent := m.renderDashboardSidePanels(grid)
	right := lipgloss.JoinVertical(lipgloss.Left, tabBar, rightContent)
	right = lipgloss.NewStyle().Width(grid.ActivityFeed.Width).MaxWidth(grid.ActivityFeed.Width).Render(right)

	const gapCols = 1
	gap := strings.Repeat(" ", gapCols)
	row := lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
	// Strict overflow protection: ensure the row doesn't exceed screen width.
	// logic in manager.go guarantees left + gap + right <= width, but
	// we add a hard clamp here to catch any rendering artifacts.
	return lipgloss.NewStyle().MaxWidth(grid.ScreenWidth).Render(row)
}

func (m *monitorModel) renderDashboardSidePanelTabBar(grid layoutmgr.GridLayout) string {
	parts := make([]string, len(dashboardSideTabNames))
	for i, name := range dashboardSideTabNames {
		if i == m.dashboardSideTab {
			parts[i] = m.styles.sectionTitle.Render(name)
		} else {
			parts[i] = kit.StyleDim.Render(name)
		}
	}
	line := strings.Join(parts, "  |  ")
	w := grid.ActivityFeed.Width
	if w <= 0 {
		w = m.width
	}
	if w <= 0 {
		w = 80
	}
	return m.styles.header.Copy().MaxWidth(w).Render(line)
}

func (m *monitorModel) renderDashboardSidePanels(grid layoutmgr.GridLayout) string {
	switch m.dashboardSideTab {
	case 0:
		feed := m.panelStyle(panelActivity).
			Width(grid.ActivityFeed.InnerWidth()).
			Height(grid.ActivityFeed.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Feed") + "\n" + m.activityList.View())
		detail := m.panelStyle(panelActivityDetail).
			Width(grid.ActivityDetail.InnerWidth()).
			Height(grid.ActivityDetail.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Details") + "\n" + m.activityDetail.View())
		return lipgloss.JoinVertical(lipgloss.Left, feed, detail)
	case 1:
		return m.panelStyle(panelPlan).
			Width(grid.Plan.InnerWidth()).
			Height(grid.Plan.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Plan") + "\n" + m.planViewport.View())
	case 2:
		w := imax(10, grid.CurrentTask.ContentWidth)
		currentTaskBody := kit.StyleDim.Render("No active task")
		if m.currentTask != nil {
			t := m.currentTask
			duration := time.Since(t.StartedAt).Round(time.Second)
			currentTaskBody = strings.Join([]string{
				kit.StyleStatusKey.Render("Goal:    ") + kit.StyleStatusValue.Render(truncateText(t.Goal, imax(10, w-12))),
				kit.StyleStatusKey.Render("Status:  ") + kit.StyleStatusValue.Render(fallback(t.Status, "unknown")),
				kit.StyleStatusKey.Render("Started: ") + t.StartedAt.Format("15:04:05"),
				kit.StyleStatusKey.Render("Elapsed: ") + duration.String(),
			}, "\n")
		}
		current := m.panelStyle(panelCurrentTask).
			Width(grid.CurrentTask.InnerWidth()).
			Height(grid.CurrentTask.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Current Task") + "\n" + currentTaskBody)
		parts := []string{current}
		if grid.Inbox.Height > 0 {
			inbox := m.panelStyle(panelInbox).
				Width(grid.Inbox.InnerWidth()).
				Height(grid.Inbox.InnerHeight()).
				Render(m.styles.sectionTitle.Render("Inbox") + "\n" + m.inboxVP.View())
			parts = append(parts, inbox)
		}
		if grid.Outbox.Height > 0 {
			parts = append(parts, m.renderOutbox(grid.Outbox))
		}
		// Constrain total height to sideContentH (= grid.Plan.Height) to prevent overflow.
		joined := lipgloss.JoinVertical(lipgloss.Left, parts...)
		return lipgloss.NewStyle().MaxHeight(grid.Plan.Height).Render(joined)
	case 3:
		return m.panelStyle(panelThinking).
			Width(grid.Plan.InnerWidth()).
			Height(grid.Plan.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Thoughts") + "\n" + m.thinkingVP.View())
	default:
		feed := m.panelStyle(panelActivity).
			Width(grid.ActivityFeed.InnerWidth()).
			Height(grid.ActivityFeed.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Feed") + "\n" + m.activityList.View())
		detail := m.panelStyle(panelActivityDetail).
			Width(grid.ActivityDetail.InnerWidth()).
			Height(grid.ActivityDetail.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Details") + "\n" + m.activityDetail.View())
		return lipgloss.JoinVertical(lipgloss.Left, feed, detail)
	}
}

func (m *monitorModel) renderOutbox(spec layoutmgr.PanelSpec) string {
	return m.panelStyle(panelOutbox).Width(spec.InnerWidth()).Height(spec.InnerHeight()).Render(
		m.styles.sectionTitle.Render("Outbox (Recent Results)") + "\n" + m.outboxVP.View(),
	)
}

func (m *monitorModel) renderMemory(spec layoutmgr.PanelSpec) string {
	return m.panelStyle(panelMemory).Width(spec.InnerWidth()).Height(spec.InnerHeight()).Render(
		m.styles.sectionTitle.Render("Memory (semantic search)") + "\n" + m.memoryVP.View(),
	)
}

func (m *monitorModel) renderComposer(spec layoutmgr.PanelSpec) string {
	contentW := max(20, spec.ContentWidth)

	// Status row: model | profile | help hints
	modelID := strings.TrimSpace(m.model)
	if modelID == "" {
		modelID = "default"
	}
	profileRef := strings.TrimSpace(m.profile)
	if profileRef == "" {
		profileRef = "default"
	}

	tagKeyStyle := kit.CloneStyle(kit.StyleStatusKey)
	tagValueStyle := kit.CloneStyle(kit.StyleStatusValue)

	modelLabel := kit.RenderTag(kit.TagOptions{
		Key:   "model",
		Value: kit.TruncateMiddle(modelID, 24),
		Styles: kit.TagStyles{
			KeyStyle:   tagKeyStyle,
			ValueStyle: tagValueStyle,
		},
	})
	profileLabel := kit.RenderTag(kit.TagOptions{
		Key:   "profile",
		Value: kit.TruncateMiddle(profileRef, 12),
		Styles: kit.TagStyles{
			KeyStyle:   tagKeyStyle,
			ValueStyle: tagValueStyle,
		},
	})

	statusLeft := modelLabel + "  " + profileLabel
	statusLeft = kit.TruncateRight(statusLeft, contentW)

	status := lipgloss.NewStyle().Width(contentW).Render(statusLeft)

	// Build content parts: status, palette (if open), input
	contentParts := []string{status}

	// Render command palette if open
	if palette := m.renderCommandPalette(); palette != "" {
		contentParts = append(contentParts, "", palette)
	}

	contentParts = append(contentParts, m.input.View())

	content := lipgloss.JoinVertical(lipgloss.Left, contentParts...)

	return m.commandBarStyle().
		Width(spec.InnerWidth()).
		Height(spec.InnerHeight()).
		Render(content)
}

func (m *monitorModel) renderStatsPanel(spec layoutmgr.PanelSpec) string {
	if spec.Height <= 0 || spec.Width <= 0 {
		return ""
	}
	return m.styles.panel.
		Width(spec.InnerWidth()).
		Height(spec.InnerHeight()).
		Render(m.styles.sectionTitle.Render("Stats") + "\n" + renderStats(m.stats))
}

func (m *monitorModel) renderStatsInline(width int) string {
	w := width
	if w <= 0 {
		w = m.width
	}
	if w <= 0 {
		w = 80
	}
	lines := strings.Split(renderStats(m.stats), "\n")
	for i := range lines {
		lines[i] = kit.TruncateRight(strings.TrimRight(lines[i], " \t"), imax(1, w-2))
		lines[i] = m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render(lines[i]))
	}
	return strings.Join(lines, "\n")
}

func (m *monitorModel) updateFocus() {
	if m.focusedPanel == panelComposer {
		m.input.Focus()
	} else {
		m.input.Blur()
	}
}

func (m *monitorModel) commandBarStyle() lipgloss.Style {
	if m.focusedPanel == panelComposer {
		return m.styles.panelFocused
	}
	return m.styles.commandBar
}

func (m *monitorModel) focusedPanelName() string {
	switch m.focusedPanel {
	case panelActivity:
		return "Activity"
	case panelActivityDetail:
		return "Details"
	case panelPlan:
		return "Plan"
	case panelCurrentTask:
		return "Current Task"
	case panelOutput:
		return "Output"
	case panelInbox:
		return "Inbox"
	case panelOutbox:
		return "Outbox"
	case panelMemory:
		return "Memory"
	case panelComposer:
		return "Composer"
	case panelThinking:
		return "Thoughts"
	default:
		return "Unknown"
	}
}

func (m *monitorModel) routeKeyToFocusedPanel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		var cmd tea.Cmd
		prev := m.activityList.Index()
		m.activityList, cmd = m.activityList.Update(msg)
		if m.activityList.Index() != prev {
			m.refreshActivityDetail()
		}
		return m, cmd
	case panelActivityDetail:
		var cmd tea.Cmd
		m.activityDetail, cmd = m.activityDetail.Update(msg)
		return m, cmd
	case panelPlan:
		var cmd tea.Cmd
		m.planViewport, cmd = m.planViewport.Update(msg)
		return m, cmd
	case panelCurrentTask:
		// Current task panel is static, no interactive model.
		return m, nil
	case panelOutput:
		var cmd tea.Cmd
		m.agentOutputVP, cmd = m.agentOutputVP.Update(msg)
		return m, cmd
	case panelInbox:
		var cmd tea.Cmd
		m.inboxVP, cmd = m.inboxVP.Update(msg)
		return m, cmd
	case panelOutbox:
		var cmd tea.Cmd
		m.outboxVP, cmd = m.outboxVP.Update(msg)
		return m, cmd
	case panelMemory:
		var cmd tea.Cmd
		m.memoryVP, cmd = m.memoryVP.Update(msg)
		return m, cmd
	case panelThinking:
		var cmd tea.Cmd
		m.thinkingVP, cmd = m.thinkingVP.Update(msg)
		return m, cmd
	case panelComposer:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.updateCommandPalette()
		return m, cmd
	default:
		return m, nil
	}
}

func (m *monitorModel) layout() layoutmgr.GridLayout {
	manager := layoutmgr.NewManager(m.styles.panel, true)
	// textarea.View() renders an extra row beyond Height() in some configurations
	// (prompt/cursor line). The composer content also includes a status row + help row.
	// Budget enough lines so the composer panel never expands beyond its spec.
	composerHeight := 5 + m.input.Height()
	if m.isCompactMode() {
		return manager.CalculateCompact(m.width, m.height, composerHeight)
	}
	// Stats panel is rendered below the composer (full width).
	statsHeight := lipgloss.Height(renderStats(m.stats))
	if statsHeight < 1 {
		statsHeight = 1
	}
	statusBarH := lipgloss.Height(m.renderStatusBar(m.width))
	return manager.CalculateDashboard(m.width, m.height, composerHeight, statsHeight, statusBarH)
}

func (m *monitorModel) calculatePanelHeight(contentRows int, isEmpty bool, isFocused bool) int {
	const maxPanelHeight = 6

	if isEmpty && !isFocused {
		return 0
	}

	desired := contentRows + 3 // border(2) + title(1)
	if desired > maxPanelHeight {
		return maxPanelHeight
	}
	if desired < 0 {
		return 0
	}
	return desired
}

func (m *monitorModel) refreshViewports() {
	if m.width == 0 || m.height == 0 {
		return
	}
	grid := m.layout()

	if m.isCompactMode() {
		outW := imax(10, grid.AgentOutput.ContentWidth)
		outH := imax(1, grid.AgentOutput.ContentHeight)
		m.agentOutputVP.Width = outW
		m.agentOutputVP.Height = outH
		m.refreshAgentOutputViewport()

		feedW := imax(10, grid.ActivityFeed.ContentWidth)
		feedH := imax(1, grid.ActivityFeed.ContentHeight)
		m.activityList.SetSize(feedW, feedH)

		detailW := imax(10, grid.ActivityDetail.ContentWidth)
		detailH := imax(1, grid.ActivityDetail.ContentHeight)
		m.activityDetail.Width = detailW
		m.activityDetail.Height = detailH
		m.refreshActivityDetail()

		planW := imax(10, grid.Plan.ContentWidth)
		planH := imax(1, grid.Plan.ContentHeight)
		m.planViewport.Width = planW
		m.planViewport.Height = planH
		m.refreshPlanView()

		outboxW := imax(10, grid.Outbox.ContentWidth)
		outboxH := imax(1, grid.Outbox.ContentHeight)
		m.outboxVP.Width = outboxW
		m.outboxVP.Height = outboxH
		m.outboxVP.SetContent(wrapViewportText(renderOutboxLines(m.outboxResults, m.renderer, outboxW), outboxW))

		// Keep auxiliary viewports sized to something sane even if not visible in compact mode.
		m.inboxVP.Width = outW
		m.inboxVP.Height = imax(1, outH)
		m.inboxVP.SetContent(wrapViewportText(renderInbox(m.inbox), outW))
		m.memoryVP.Width = outW
		m.memoryVP.Height = outH
		m.memoryVP.SetContent(wrapViewportText(renderMemResults(m.memResults), outW))
		m.thinkingVP.Width = outW
		m.thinkingVP.Height = outH
		m.refreshThinkingViewport()

		m.input.SetWidth(imax(10, grid.Composer.ContentWidth))
		return
	}

	w := imax(10, grid.AgentOutput.ContentWidth)
	m.agentOutputVP.Width = w
	m.agentOutputVP.Height = imax(1, grid.AgentOutput.ContentHeight)
	m.refreshAgentOutputViewport()

	m.outboxVP.Width = imax(10, grid.Outbox.ContentWidth)
	m.outboxVP.Height = imax(1, grid.Outbox.ContentHeight)
	m.outboxVP.SetContent(wrapViewportText(renderOutboxLines(m.outboxResults, m.renderer, m.outboxVP.Width), m.outboxVP.Width))

	feedW := imax(10, grid.ActivityFeed.ContentWidth)
	feedH := imax(1, grid.ActivityFeed.ContentHeight)
	m.activityList.SetSize(feedW, feedH)

	detailW := imax(10, grid.ActivityDetail.ContentWidth)
	detailH := imax(1, grid.ActivityDetail.ContentHeight)
	m.activityDetail.Width = detailW
	m.activityDetail.Height = detailH
	m.refreshActivityDetail()

	planW := imax(10, grid.Plan.ContentWidth)
	planH := imax(1, grid.Plan.ContentHeight)
	m.planViewport.Width = planW
	m.planViewport.Height = planH
	m.refreshPlanView()
	m.thinkingVP.Width = planW
	m.thinkingVP.Height = planH
	m.refreshThinkingViewport()
	inboxW := imax(10, grid.Inbox.ContentWidth)
	inboxH := imax(1, grid.Inbox.ContentHeight)
	m.inboxVP.Width = inboxW
	m.inboxVP.Height = inboxH
	m.inboxVP.SetContent(wrapViewportText(renderInbox(m.inbox), inboxW))
	m.memoryVP.Width = imax(10, grid.Memory.ContentWidth)
	m.memoryVP.Height = imax(1, grid.Memory.ContentHeight)
	m.memoryVP.SetContent(wrapViewportText(renderMemResults(m.memResults), m.memoryVP.Width))
	m.input.SetWidth(imax(10, grid.Composer.ContentWidth))
}

func (m *monitorModel) loadPlanFiles() {
	runDir := fsutil.GetAgentDir(m.cfg.DataDir, m.runID)
	planDir := filepath.Join(runDir, "plan")

	load := func(name string) (string, string) {
		b, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				return "", ""
			}
			return "", err.Error()
		}
		return string(b), ""
	}

	m.planDetails, m.planDetailsErr = load("HEAD.md")
	m.planMarkdown, m.planLoadErr = load("CHECKLIST.md")
	m.refreshPlanView()
}

func isPlanEvent(kind string, path string) bool {
	if kind != "fs.write" && kind != "fs.append" && kind != "fs.edit" && kind != "fs.patch" {
		return false
	}
	p := strings.TrimSpace(path)
	return strings.EqualFold(p, "/plan/HEAD.md") || strings.EqualFold(p, "/plan/CHECKLIST.md")
}

func formatEventLine(e types.EventRecord) string {
	ts := e.Timestamp.Local().Format("15:04:05")
	line := fmt.Sprintf("[%s] %s: %s", ts, e.Type, e.Message)
	if v := strings.TrimSpace(e.Data["taskId"]); v != "" {
		line += " task=" + shortID(v)
	}
	if v := strings.TrimSpace(e.Data["goal"]); v != "" {
		line += " goal=" + truncateText(v, 40)
	}
	if v := strings.TrimSpace(e.Data["status"]); v != "" {
		line += " status=" + v
	}
	if v := strings.TrimSpace(e.Data["summary"]); v != "" {
		line += " summary=" + truncateText(v, 60)
	}
	if v := strings.TrimSpace(e.Data["error"]); v != "" {
		line += " error=" + truncateText(v, 80)
	}
	if v := strings.TrimSpace(e.Data["op"]); v != "" {
		line += " op=" + v
	}
	if v := strings.TrimSpace(e.Data["path"]); v != "" {
		line += " path=" + truncateText(v, 40)
	}
	if v := strings.TrimSpace(e.Data["poisonPath"]); v != "" {
		line += " poison=" + truncateText(v, 40)
	}
	return line
}

func outputLineStyle(line string) lipgloss.Style {
	line = strings.TrimSpace(line)
	if line == "" {
		return lipgloss.NewStyle()
	}
	// formatEventLine: "[15:04:05] <type>: <message>"
	eventType := ""
	if strings.HasPrefix(line, "[") {
		if end := strings.Index(line, "]"); end != -1 {
			inside := strings.TrimSpace(line[1:end])
			rest := strings.TrimSpace(line[end+1:])
			// If the bracket looks like a timestamp, parse the event type after it.
			if strings.Count(inside, ":") >= 2 {
				if colon := strings.Index(rest, ":"); colon != -1 {
					eventType = strings.TrimSpace(rest[:colon])
				}
			} else {
				// Monitor-local status lines like "[error]" or "[control] ..."
				eventType = inside
				if eventType == "queued" {
					eventType = "task.queued"
				}
			}
		}
	}
	switch eventType {
	case "error", "daemon.error", "daemon.runner.error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	case "task.done", "task.delivered":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950"))
	case "task.start", "task.queued", "task.generated":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6bbcff"))
	case "control", "control.check", "control.success", "control.error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922"))
	case "daemon.start", "daemon.stop", "daemon.control", "daemon.warning":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#a371f7"))
	case "task.quarantined":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	default:
		return kit.StyleDim
	}
}

func (m *monitorModel) refreshAgentOutputViewport() {
	w := m.agentOutputVP.Width
	if w <= 0 {
		w = 80
	}
	lines := make([]string, 0, len(m.agentOutput))
	for _, rawLine := range m.agentOutput {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			lines = append(lines, "")
			continue
		}
		style := outputLineStyle(rawLine)
		wrapped := wordwrap.String(rawLine, w)
		for _, sub := range strings.Split(wrapped, "\n") {
			sub = strings.TrimRight(sub, " ")
			if sub == "" {
				lines = append(lines, "")
				continue
			}
			lines = append(lines, style.Render(sub))
		}
	}
	m.agentOutputVP.SetContent(strings.Join(lines, "\n"))
	m.agentOutputVP.GotoBottom()
}

func renderInbox(tasks map[string]taskState) string {
	if len(tasks) == 0 {
		return kit.StyleDim.Render("No pending inbox tasks.")
	}
	ids := make([]string, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		task := tasks[id]
		goal := truncateText(task.Goal, 48)
		// Use bullet + bold ID for better visual hierarchy
		line := "• " + kit.StyleBold.Render(shortID(id))
		if goal != "" {
			line += " — " + goal
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderOutboxLines(results []outboxEntry, renderer *ContentRenderer, width int) string {
	if len(results) == 0 {
		return kit.StyleDim.Render("No completed tasks yet.")
	}
	lines := make([]string, 0, len(results))
	for _, r := range results {
		goal := truncateText(r.Goal, 50)
		summary := strings.TrimSpace(r.Summary)
		status := r.Status
		if status == "" {
			status = "unknown"
		}

		// Color-code status for quick visual scanning
		statusStr := status
		switch status {
		case "succeeded":
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#98c379")).Render(status)
		case "failed", "quarantined", "canceled":
			statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Render(status)
		}

		// Header: bullet + bold ID + dim goal -> colored status
		header := "• " + kit.StyleBold.Render(shortID(r.TaskID)) + " " +
			kit.StyleDim.Render("\""+goal+"\"") + " → " + statusStr
		lines = append(lines, header)

		// Summary with markdown rendering
		if summary != "" {
			summaryRendered := summary
			if renderer != nil && width > 0 {
				// Render markdown and indent each line
				rendered := strings.TrimRight(renderer.RenderMarkdown(summary, width-4), "\n")
				renderedLines := strings.Split(rendered, "\n")
				for i, line := range renderedLines {
					if i == 0 {
						summaryRendered = "  └ " + line
					} else {
						summaryRendered += "\n    " + line
					}
				}
			} else {
				summaryRendered = "  └ " + summary
			}
			lines = append(lines, summaryRendered)
		}
		if strings.TrimSpace(r.Error) != "" && (status == "failed" || status == "canceled" || status == "quarantined") {
			lines = append(lines, "  └ "+lipgloss.NewStyle().Foreground(lipgloss.Color("#e06c75")).Render("error: "+strings.TrimSpace(r.Error)))
		}
		if r.ArtifactsCount > 0 || strings.TrimSpace(r.SummaryPath) != "" {
			info := fmt.Sprintf("  └ deliverables: %d", r.ArtifactsCount)
			if strings.TrimSpace(r.SummaryPath) != "" {
				info += " (summary: " + r.SummaryPath + ")"
			}
			lines = append(lines, info)
		}
	}
	return strings.Join(lines, "\n")
}

func renderStats(s monitorStats) string {
	uptime := ""
	if !s.started.IsZero() {
		uptime = time.Since(s.started).Round(time.Second).String()
	}
	costLine := ""
	if strings.TrimSpace(s.lastTurnCostUSD) != "" {
		costLine = fmt.Sprintf("\nLast cost: $%s", s.lastTurnCostUSD)
	}
	totalLine := ""
	if s.totalCostUSD > 0 {
		totalLine = fmt.Sprintf("\nTotal cost: $%.4f", s.totalCostUSD)
	}
	return fmt.Sprintf("Tasks done: %d\nUptime: %s%s%s", s.tasksDone, fallback(uptime, "unknown"), costLine, totalLine)
}

func renderMemResults(results []string) string {
	if len(results) == 0 {
		return "No memory results."
	}
	return strings.Join(results, "\n")
}

// wrapViewportText hard-wraps long lines so the viewport never renders lines wider
// than its configured width (which would otherwise get clipped by the terminal).
func wrapViewportText(s string, width int) string {
	if width <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, "")
			continue
		}
		wrapped := wordwrap.String(line, width)
		out = append(out, strings.Split(wrapped, "\n")...)
	}
	return strings.Join(out, "\n")
}

func shortID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func truncateText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func fallback(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type monitorStyles struct {
	header       lipgloss.Style
	headerTitle  lipgloss.Style
	sectionTitle lipgloss.Style
	panel        lipgloss.Style
	panelFocused lipgloss.Style
	commandBar   lipgloss.Style
}

func defaultMonitorStyles() *monitorStyles {
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(kit.BorderColorDefault).
		Padding(0, 1)
	panelFocused := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(kit.BorderColorAccent).
		Padding(0, 1)

	return &monitorStyles{
		header:       lipgloss.NewStyle().Padding(0, 1),
		headerTitle:  lipgloss.NewStyle().Bold(true),
		sectionTitle: lipgloss.NewStyle().Bold(true),
		panel:        panel,
		panelFocused: panelFocused,
		commandBar:   lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(kit.BorderColorDefault).Padding(0, 1),
	}
}
