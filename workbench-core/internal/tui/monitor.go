package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	taskQueue         map[string]taskState
	taskQueueVP       viewport.Model
	currentTask       *taskState
	outboxResults     []outboxEntry
	outboxVP          viewport.Model
	memResults        []string
	memoryVP          viewport.Model
	planMarkdown      string
	planDetails       string
	planLoadErr       string
	planDetailsErr    string
	stats             monitorStats
	model             string
	role              string
	focusedPanel      panelID
	compactTab        int // 0=Output, 1=Activity, 2=Plan, 3=Outbox; used when isCompactMode()
	width             int
	height            int
	styles            *monitorStyles
	tailCh            <-chan store.TailedEvent
	errCh             <-chan error
	cancel            context.CancelFunc
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
	TaskID    string
	Goal      string
	Status    string
	Summary   string
	Timestamp time.Time
}

type panelID int

const (
	panelActivity panelID = iota
	panelActivityDetail
	panelPlan
	panelOutput
	panelQueue
	panelOutbox
	panelMemory
	panelComposer
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
	in.Placeholder = "/task <goal>  |  /memory search <query>  |  /role <name>  |  /model <id>  |  /quit"
	in.SetHeight(2)
	in.CharLimit = 500
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

	runStatus := types.StatusDone
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
		taskQueue:         map[string]taskState{},
		taskQueueVP:       viewport.New(0, 0),
		outboxResults:     []outboxEntry{},
		outboxVP:          viewport.New(0, 0),
		memResults:        []string{},
		memoryVP:          viewport.New(0, 0),
		stats:             stats,
		styles:            defaultMonitorStyles(),
		focusedPanel:      panelComposer,
		tailCh:            tailCh,
		errCh:             errCh,
		cancel:            cancel,
	}

	for _, e := range evs {
		m.observeEvent(e)
	}
	m.refreshActivityList()
	m.loadPlanFiles()
	m.refreshViewports()

	return m, nil
}

func (m *monitorModel) Init() tea.Cmd {
	return tea.Batch(m.listenEvent(), m.listenErr())
}

func (m *monitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.refreshViewports()
		return m, nil

	case tailedEventMsg:
		if msg.ev.Event.EventId != "" {
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

	case tea.KeyMsg:
		if m.focusedPanel == panelComposer {
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
				cmd = strings.Join(strings.Fields(cmd), " ")
				return m, m.handleCommand(cmd)
			}
		}
		switch msg.String() {
		case "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "tab":
			if m.isCompactMode() {
				m.compactTab = (m.compactTab + 1) % 4
				return m, nil
			}
			m.focusedPanel = (m.focusedPanel + 1) % 8
			m.updateFocus()
			return m, nil
		case "shift+tab":
			if m.isCompactMode() {
				m.compactTab = (m.compactTab + 3) % 4
				return m, nil
			}
			m.focusedPanel = (m.focusedPanel + 7) % 8
			m.updateFocus()
			return m, nil
		}
		if m.isCompactMode() {
			m.focusedPanel = m.compactTabToPanel()
		}
		return m.routeKeyToFocusedPanel(msg)
	}

	return m, nil
}

func (m *monitorModel) isCompactMode() bool {
	return m.width < compactModeMinWidth || m.height < compactModeMinHeight
}

func (m *monitorModel) View() string {
	grid := m.layout()
	headerLine := m.renderHeader()

	if m.isCompactMode() {
		return m.renderCompact(grid, headerLine)
	}
	return m.renderDashboard(grid, headerLine)
}

func (m *monitorModel) renderDashboard(grid layoutmgr.GridLayout, headerLine string) string {
	main := m.renderMainBodyDashboard(grid)
	sections := []string{headerLine, "", main, "", m.renderComposer(grid.Composer), m.renderStatusBar(grid)}
	if m.runStatus != types.StatusRunning {
		warning := m.styles.header.Render(kit.StyleDim.Render("Run is not active; start the daemon first or use --run-id to attach to the running run."))
		sections = []string{headerLine, warning, "", main, "", m.renderComposer(grid.Composer), m.renderStatusBar(grid)}
	}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *monitorModel) renderStatusBar(grid layoutmgr.GridLayout) string {
	return m.styles.header.Render(kit.StyleDim.Render("Tab: cycle panels  |  Ctrl+Enter: submit  |  /quit"))
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
	default:
		return panelOutput
	}
}

// renderCompact builds the view for compact mode: header + tab bar + main content + composer.
func (m *monitorModel) renderCompact(grid layoutmgr.GridLayout, headerLine string) string {
	tabBar := m.renderCompactTabBar()
	contentHeight := grid.AgentOutput.InnerHeight() - 1
	if contentHeight < 1 {
		contentHeight = 1
	}
	content := m.renderCompactTabContent(grid, contentHeight)
	main := m.panelStyle(panelOutput).
		Width(grid.AgentOutput.InnerWidth()).Height(contentHeight).
		Render(content)
	sections := []string{headerLine, tabBar, main, m.renderComposer(grid.Composer)}
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *monitorModel) renderCompactTabBar() string {
	parts := make([]string, len(compactTabNames))
	for i, name := range compactTabNames {
		if i == m.compactTab {
			parts[i] = m.styles.sectionTitle.Render(name)
		} else {
			parts[i] = kit.StyleDim.Render(name)
		}
	}
	return m.styles.header.Render(strings.Join(parts, "  |  "))
}

func (m *monitorModel) renderCompactTabContent(grid layoutmgr.GridLayout, contentHeight int) string {
	switch m.compactTab {
	case 0:
		return m.styles.sectionTitle.Render("Agent Output") + "\n" + m.agentOutputVP.View()
	case 1:
		return m.styles.sectionTitle.Render("Activity") + "\n" + m.activityList.View() + "\n" + m.activityDetail.View()
	case 2:
		return m.styles.sectionTitle.Render("Plan") + "\n" + m.planViewport.View()
	case 3:
		return m.styles.sectionTitle.Render("Outbox") + "\n" + m.outboxVP.View()
	default:
		return m.styles.sectionTitle.Render("Output") + "\n" + m.agentOutputVP.View()
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
	if raw == "/quit" {
		if m.cancel != nil {
			m.cancel()
		}
		return tea.Quit
	}

	if strings.HasPrefix(raw, "/task ") {
		goal := strings.TrimSpace(strings.TrimPrefix(raw, "/task "))
		goal = strings.Trim(goal, "\"")
		return m.enqueueTask(goal, 0)
	}
	if strings.HasPrefix(raw, "/role ") {
		roleName := strings.TrimSpace(strings.TrimPrefix(raw, "/role "))
		return m.writeControl(map[string]any{"role": roleName})
	}
	if strings.HasPrefix(raw, "/model ") {
		model := strings.TrimSpace(strings.TrimPrefix(raw, "/model "))
		return m.writeControl(map[string]any{"model": model})
	}
	if strings.HasPrefix(raw, "/memory search ") {
		query := strings.TrimSpace(strings.TrimPrefix(raw, "/memory search "))
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
		runDir := fsutil.GetRunDir(m.cfg.DataDir, m.runID)
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
		return commandLinesMsg{lines: []string{"[queued] " + id + " " + goal + " — task queued to run " + m.runID}}
	}
}

func (m *monitorModel) writeControl(payload map[string]any) tea.Cmd {
	return func() tea.Msg {
		runDir := fsutil.GetRunDir(m.cfg.DataDir, m.runID)
		inboxDir := filepath.Join(runDir, "inbox")
		_ = os.MkdirAll(inboxDir, 0755)
		payload["processed"] = false
		b, _ := json.MarshalIndent(payload, "", "  ")
		if err := os.WriteFile(filepath.Join(inboxDir, "control.json"), b, 0644); err != nil {
			return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
		}
		return commandLinesMsg{lines: []string{"[control] updated"}}
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
			lines = append(lines, fmt.Sprintf("  - %.3f %s (%s)", r.Score, r.Title, r.Filename))
		}
		return commandLinesMsg{lines: lines}
	}
}

func (m *monitorModel) observeEvent(ev types.Event) {
	if v := strings.TrimSpace(ev.Data["model"]); v != "" {
		m.model = v
	}
	if v := strings.TrimSpace(ev.Data["role"]); v != "" {
		m.role = v
	}
	if strings.HasPrefix(ev.Type, "agent.op.") {
		m.observeActivityEvent(ev)
	}
	m.observeTaskEvent(ev)
	m.observeAgentOutput(ev)
	switch ev.Type {
	case "llm.cost.total":
		m.stats.lastTurnCostUSD = strings.TrimSpace(ev.Data["costUsd"])
		if v := strings.TrimSpace(ev.Data["costUsd"]); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				m.stats.totalCostUSD += f
			}
		}
	}
}

func (m *monitorModel) observeActivityEvent(ev types.Event) {
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

func (m *monitorModel) observeTaskEvent(ev types.Event) {
	switch ev.Type {
	case "task.queued", "task.generated":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.taskQueue[taskID] = taskState{
			TaskID: taskID,
			Goal:   strings.TrimSpace(ev.Data["goal"]),
			Status: string(types.TaskStatusPending),
		}
	case "task.start":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		ts := m.taskQueue[taskID]
		ts.TaskID = taskID
		ts.Goal = strings.TrimSpace(ev.Data["goal"])
		ts.Status = "active"
		ts.StartedAt = ev.Timestamp
		m.currentTask = &ts
		delete(m.taskQueue, taskID)
	case "task.done":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.currentTask = nil
		m.stats.tasksDone++
		m.outboxResults = append(m.outboxResults, outboxEntry{
			TaskID:    taskID,
			Goal:      strings.TrimSpace(ev.Data["goal"]),
			Status:    strings.TrimSpace(ev.Data["status"]),
			Summary:   strings.TrimSpace(ev.Data["summary"]),
			Timestamp: ev.Timestamp,
		})
		if len(m.outboxResults) > 10 {
			m.outboxResults = m.outboxResults[len(m.outboxResults)-10:]
		}
	}
}

func (m *monitorModel) observeAgentOutput(ev types.Event) {
	switch ev.Type {
	case "daemon.start", "daemon.stop", "daemon.control", "daemon.warning":
		m.appendAgentOutput(formatEventLine(ev))
	case "task.queued", "task.generated", "task.start", "task.done":
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
	items := make([]list.Item, 0, len(m.activities))
	for _, a := range m.activities {
		items = append(items, activityItem{act: a})
	}
	m.activityList.SetItems(items)
	if len(items) > 0 {
		lastIdx := len(items) - 1
		m.activityList.Select(lastIdx)

		// Manually drive pagination to the end so the newest entry is visible.
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
	m.activityDetail.SetContent(strings.TrimRight(m.renderer.RenderMarkdown(header+help+md, w), "\n"))
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
	m.planViewport.SetContent(strings.TrimRight(m.renderer.RenderMarkdown(content, w), "\n"))
	m.planViewport.GotoTop()
}

func (m *monitorModel) renderHeader() string {
	return m.styles.header.Render(
		lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.headerTitle.Render("Workbench - Always On"),
			kit.RenderTag(kit.TagOptions{Key: "Model", Value: fallback(m.model, "(unknown)")}),
			kit.RenderTag(kit.TagOptions{Key: "Role", Value: fallback(m.role, "(none)")}),
			kit.RenderTag(kit.TagOptions{Key: "Run", Value: m.runID}),
		))
}

func (m *monitorModel) panelStyle(panel panelID) lipgloss.Style {
	if m.focusedPanel == panel {
		return m.styles.panelFocused
	}
	return m.styles.panel
}

// renderMainBodyDashboard builds the two-column dashboard: left = AgentOutput + Outbox,
// right = Activity, ActivityDetail, CurrentTask, Queue, Plan, Stats.
func (m *monitorModel) renderMainBodyDashboard(grid layoutmgr.GridLayout) string {
	leftParts := []string{
		m.panelStyle(panelOutput).Width(grid.AgentOutput.InnerWidth()).Height(grid.AgentOutput.InnerHeight()).Render(
			m.styles.sectionTitle.Render("Agent Output") + "\n" + m.agentOutputVP.View(),
		),
	}
	if grid.Outbox.Height > 0 {
		leftParts = append(leftParts, m.renderOutbox(grid.Outbox))
	}
	left := lipgloss.JoinVertical(lipgloss.Left, leftParts...)

	right := lipgloss.JoinVertical(lipgloss.Left,
		m.panelStyle(panelActivity).Width(grid.ActivityFeed.InnerWidth()).Height(grid.ActivityFeed.InnerHeight()).Render(
			m.styles.sectionTitle.Render("Activity Feed")+"\n"+m.activityList.View(),
		),
		m.panelStyle(panelActivityDetail).Width(grid.ActivityDetail.InnerWidth()).Height(grid.ActivityDetail.InnerHeight()).Render(
			m.styles.sectionTitle.Render("Activity Details")+"\n"+m.activityDetail.View(),
		),
		m.renderCurrentTask(grid.CurrentTask),
		m.renderTaskQueue(grid.TaskQueue),
		m.panelStyle(panelPlan).Width(grid.Plan.InnerWidth()).Height(grid.Plan.InnerHeight()).Render(
			m.styles.sectionTitle.Render("Plan")+"\n"+m.planViewport.View(),
		),
		m.styles.panel.Width(grid.Stats.InnerWidth()).Height(grid.Stats.InnerHeight()).Render(
			m.styles.sectionTitle.Render("Stats")+"\n"+renderStats(m.stats),
		),
	)

	const gapCols = 1
	gap := strings.Repeat(" ", gapCols)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
}

func (m *monitorModel) renderCurrentTask(spec layoutmgr.PanelSpec) string {
	body := kit.StyleDim.Render("No active task")
	if m.currentTask != nil {
		t := m.currentTask
		duration := time.Since(t.StartedAt).Round(time.Second)
		body = fmt.Sprintf(
			"%s\n%s\n%s\n%s",
			kit.StyleStatusKey.Render("Goal: ")+truncateText(t.Goal, imax(10, spec.ContentWidth-12)),
			kit.StyleStatusKey.Render("Status: ")+fallback(t.Status, "unknown"),
			kit.StyleStatusKey.Render("Started: ")+t.StartedAt.Format("15:04:05"),
			kit.StyleStatusKey.Render("Duration: ")+duration.String(),
		)
	}
	return m.styles.panel.Width(spec.InnerWidth()).Height(spec.InnerHeight()).Render(
		m.styles.sectionTitle.Render("Current Task") + "\n" + body,
	)
}

func (m *monitorModel) renderTaskQueue(spec layoutmgr.PanelSpec) string {
	return m.panelStyle(panelQueue).Width(spec.InnerWidth()).Height(spec.InnerHeight()).Render(
		m.styles.sectionTitle.Render("Queue") + "\n" + m.taskQueueVP.View(),
	)
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
	help := fmt.Sprintf("Tab: cycle panels (%s)  |  Ctrl+Enter: submit  |  /quit", m.focusedPanelName())
	return m.commandBarStyle().
		Width(spec.InnerWidth()).
		Height(spec.InnerHeight()).
		Render(help + "\n" + m.input.View())
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
	case panelOutput:
		return "Output"
	case panelQueue:
		return "Queue"
	case panelOutbox:
		return "Outbox"
	case panelMemory:
		return "Memory"
	case panelComposer:
		return "Composer"
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
	case panelOutput:
		var cmd tea.Cmd
		m.agentOutputVP, cmd = m.agentOutputVP.Update(msg)
		return m, cmd
	case panelQueue:
		var cmd tea.Cmd
		m.taskQueueVP, cmd = m.taskQueueVP.Update(msg)
		return m, cmd
	case panelOutbox:
		var cmd tea.Cmd
		m.outboxVP, cmd = m.outboxVP.Update(msg)
		return m, cmd
	case panelMemory:
		var cmd tea.Cmd
		m.memoryVP, cmd = m.memoryVP.Update(msg)
		return m, cmd
	case panelComposer:
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m *monitorModel) layout() layoutmgr.GridLayout {
	manager := layoutmgr.NewManager(m.styles.panel, true)
	composerHeight := 3 + m.input.Height()
	if m.isCompactMode() {
		return manager.CalculateCompact(m.width, m.height, composerHeight)
	}
	outboxRows := len(m.outboxResults)
	outboxHeight := m.calculatePanelHeight(outboxRows, outboxRows == 0, m.focusedPanel == panelOutbox)
	return manager.CalculateDashboard(m.width, m.height, composerHeight, outboxHeight)
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
		w := imax(10, grid.AgentOutput.ContentWidth)
		h := imax(1, grid.AgentOutput.ContentHeight-1)
		m.activityList.SetSize(w, h/2)
		m.agentOutputVP.Width = w
		m.agentOutputVP.Height = h
		m.agentOutputVP.SetContent(strings.Join(m.agentOutput, "\n"))
		m.agentOutputVP.GotoBottom()
		m.activityDetail.Width = w
		m.activityDetail.Height = h / 2
		m.refreshActivityDetail()
		m.planViewport.Width = w
		m.planViewport.Height = h
		m.refreshPlanView()
		m.outboxVP.Width = w
		m.outboxVP.Height = h
		m.outboxVP.SetContent(renderOutboxLines(m.outboxResults))
		m.taskQueueVP.Width = w
		m.taskQueueVP.Height = imax(1, grid.TaskQueue.ContentHeight)
		m.taskQueueVP.SetContent(renderTaskQueue(m.taskQueue))
		m.memoryVP.Width = w
		m.memoryVP.Height = h
		m.memoryVP.SetContent(renderMemResults(m.memResults))
		m.input.SetWidth(imax(10, grid.Composer.ContentWidth))
		return
	}

	m.activityList.SetSize(
		imax(10, grid.ActivityFeed.ContentWidth),
		imax(1, grid.ActivityFeed.ContentHeight),
	)

	m.agentOutputVP.Width = imax(10, grid.AgentOutput.ContentWidth)
	m.agentOutputVP.Height = imax(1, grid.AgentOutput.ContentHeight)
	m.agentOutputVP.SetContent(strings.Join(m.agentOutput, "\n"))
	m.agentOutputVP.GotoBottom()

	m.activityDetail.Width = imax(10, grid.ActivityDetail.ContentWidth)
	m.activityDetail.Height = imax(1, grid.ActivityDetail.ContentHeight)
	m.refreshActivityDetail()

	m.planViewport.Width = imax(10, grid.Plan.ContentWidth)
	m.planViewport.Height = imax(1, grid.Plan.ContentHeight)
	m.refreshPlanView()

	m.taskQueueVP.Width = imax(10, grid.TaskQueue.ContentWidth)
	m.taskQueueVP.Height = imax(1, grid.TaskQueue.ContentHeight)
	m.taskQueueVP.SetContent(renderTaskQueue(m.taskQueue))

	m.outboxVP.Width = imax(10, grid.Outbox.ContentWidth)
	m.outboxVP.Height = imax(1, grid.Outbox.ContentHeight)
	m.outboxVP.SetContent(renderOutboxLines(m.outboxResults))

	m.memoryVP.Width = imax(10, grid.Memory.ContentWidth)
	m.memoryVP.Height = imax(1, grid.Memory.ContentHeight)
	m.memoryVP.SetContent(renderMemResults(m.memResults))

	m.input.SetWidth(imax(10, grid.Composer.ContentWidth))
}

func (m *monitorModel) loadPlanFiles() {
	runDir := fsutil.GetRunDir(m.cfg.DataDir, m.runID)
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

func formatEventLine(e types.Event) string {
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
	if v := strings.TrimSpace(e.Data["op"]); v != "" {
		line += " op=" + v
	}
	if v := strings.TrimSpace(e.Data["path"]); v != "" {
		line += " path=" + truncateText(v, 40)
	}
	return line
}

func renderTaskQueue(tasks map[string]taskState) string {
	if len(tasks) == 0 {
		return "No pending tasks."
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
		if goal != "" {
			lines = append(lines, fmt.Sprintf("%s - %s", shortID(id), goal))
		} else {
			lines = append(lines, shortID(id))
		}
	}
	return strings.Join(lines, "\n")
}

func renderOutboxLines(results []outboxEntry) string {
	if len(results) == 0 {
		return "No completed tasks yet."
	}
	lines := make([]string, 0, len(results))
	for _, r := range results {
		goal := truncateText(r.Goal, 30)
		summary := truncateText(r.Summary, 60)
		status := r.Status
		if status == "" {
			status = "unknown"
		}
		line := fmt.Sprintf("%s %q -> %s: %s", shortID(r.TaskID), goal, status, summary)
		lines = append(lines, line)
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
