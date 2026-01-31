package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	ctx   context.Context
	cfg   config.Config
	runID string

	offset int64

	input textarea.Model

	activities        []Activity
	activityIndexByID map[string]int
	activityIndexByOp map[string]int
	activitySeq       int
	pendingActivityID string
	activityList      list.Model
	activityDetail    viewport.Model
	agentOutput       []string
	agentOutputVP     viewport.Model
	taskQueue         map[string]taskState
	taskQueueVP       viewport.Model
	currentTask       *taskState
	outboxResults     []outboxEntry
	outboxVP          viewport.Model
	memResults        []string
	memoryVP          viewport.Model
	stats             monitorStats
	model             string
	role              string
	focusedPanel      panelID
	width             int
	height            int
	styles            *monitorStyles
	tailCh            <-chan store.TailedEvent
	errCh             <-chan error
	cancel            context.CancelFunc
}

type monitorStats struct {
	started   time.Time
	tasksDone int
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
	panelOutput
	panelQueue
	panelOutbox
	panelMemory
	panelComposer
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

	tctx, cancel := context.WithCancel(ctx)
	evs, off, _ := store.ListEvents(cfg, runID)
	tailCh, errCh := store.TailEvents(cfg, tctx, runID, off)

	m := &monitorModel{
		ctx:               ctx,
		cfg:               cfg,
		runID:             runID,
		offset:            off,
		input:             in,
		activities:        []Activity{},
		activityIndexByID: map[string]int{},
		activityIndexByOp: map[string]int{},
		activityList:      activityList,
		activityDetail:    viewport.New(0, 0),
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
			m.focusedPanel = (m.focusedPanel + 1) % 6
			m.updateFocus()
			return m, nil
		case "shift+tab":
			m.focusedPanel = (m.focusedPanel + 5) % 6
			m.updateFocus()
			return m, nil
		}
		return m.routeKeyToFocusedPanel(msg)
	}

	return m, nil
}

func (m *monitorModel) View() string {
	headerLine := m.renderHeader()
	main := m.renderMainBody()
	outbox := m.renderOutbox()
	memory := m.renderMemory()
	composer := m.renderComposer()

	return lipgloss.JoinVertical(lipgloss.Left, headerLine, main, outbox, memory, composer)
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
			Status:    "pending",
			CreatedAt: &now,
		}
		b, _ := json.MarshalIndent(task, "", "  ")
		if err := os.WriteFile(filepath.Join(inboxDir, id+".json"), b, 0644); err != nil {
			return commandLinesMsg{lines: []string{"[queued] error: " + err.Error()}}
		}
		return commandLinesMsg{lines: []string{"[queued] " + id + " " + goal}}
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
		m.refreshActivityList()
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
			Status: "pending",
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
		m.activityList.Select(len(items) - 1)
	}
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

func (m *monitorModel) renderMainBody() string {
	layout := m.layout()

	left := lipgloss.JoinVertical(lipgloss.Left,
		m.panelStyle(panelActivity).Width(layout.leftW).Height(layout.activityH).Render(
			m.styles.sectionTitle.Render("Activity Feed")+"\n"+m.activityList.View(),
		),
		m.panelStyle(panelOutput).Width(layout.leftW).Height(layout.outputH).Render(
			m.styles.sectionTitle.Render("Agent Output")+"\n"+m.agentOutputVP.View(),
		),
	)

	right := lipgloss.JoinVertical(lipgloss.Left,
		m.renderCurrentTask(layout.rightW, layout.taskH),
		m.renderTaskQueue(layout.rightW, layout.queueH),
		m.styles.panel.Width(layout.rightW).Height(layout.statsH).Render(
			m.styles.sectionTitle.Render("Stats")+"\n"+renderStats(m.stats),
		),
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m *monitorModel) renderCurrentTask(width int, height int) string {
	body := kit.StyleDim.Render("No active task")
	if m.currentTask != nil {
		t := m.currentTask
		duration := time.Since(t.StartedAt).Round(time.Second)
		body = fmt.Sprintf(
			"%s\n%s\n%s\n%s",
			kit.StyleStatusKey.Render("Goal: ")+truncateText(t.Goal, imax(10, width-12)),
			kit.StyleStatusKey.Render("Status: ")+fallback(t.Status, "unknown"),
			kit.StyleStatusKey.Render("Started: ")+t.StartedAt.Format("15:04:05"),
			kit.StyleStatusKey.Render("Duration: ")+duration.String(),
		)
	}
	return m.styles.panel.Width(width).Height(height).Render(
		m.styles.sectionTitle.Render("Current Task") + "\n" + body,
	)
}

func (m *monitorModel) renderTaskQueue(width int, height int) string {
	return m.panelStyle(panelQueue).Width(width).Height(height).Render(
		m.styles.sectionTitle.Render("Queue") + "\n" + m.taskQueueVP.View(),
	)
}

func (m *monitorModel) renderOutbox() string {
	return m.panelStyle(panelOutbox).Render(
		m.styles.sectionTitle.Render("Outbox (Recent Results)") + "\n" + m.outboxVP.View(),
	)
}

func (m *monitorModel) renderMemory() string {
	return m.panelStyle(panelMemory).Render(
		m.styles.sectionTitle.Render("Memory (semantic search)") + "\n" + m.memoryVP.View(),
	)
}

func (m *monitorModel) renderComposer() string {
	help := fmt.Sprintf("Tab: cycle panels (%s)  |  Ctrl+Enter: submit  |  /quit", m.focusedPanelName())
	return m.commandBarStyle().Render(help + "\n" + m.input.View())
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
		m.activityList, cmd = m.activityList.Update(msg)
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

type layoutConfig struct {
	leftW     int
	rightW    int
	mainH     int
	activityH int
	outputH   int
	taskH     int
	queueH    int
	statsH    int
	outboxH   int
	memoryH   int
	composerH int
}

func (m *monitorModel) layout() layoutConfig {
	headerH := 1
	composerH := 3 + m.input.Height()
	outboxH := 6
	memoryH := 6
	mainH := m.height - headerH - composerH - outboxH - memoryH
	if mainH < 8 {
		mainH = 8
	}
	leftW := int(float64(m.width) * 0.65)
	if leftW < 40 {
		leftW = 40
	}
	rightW := m.width - leftW
	if rightW < 30 {
		rightW = 30
		leftW = m.width - rightW
	}
	activityH := mainH / 2
	outputH := mainH - activityH
	taskH := mainH / 2
	queueH := (mainH - taskH) / 2
	statsH := mainH - taskH - queueH
	return layoutConfig{
		leftW:     leftW,
		rightW:    rightW,
		mainH:     mainH,
		activityH: activityH,
		outputH:   outputH,
		taskH:     taskH,
		queueH:    queueH,
		statsH:    statsH,
		outboxH:   outboxH,
		memoryH:   memoryH,
		composerH: composerH,
	}
}

func (m *monitorModel) refreshViewports() {
	if m.width == 0 || m.height == 0 {
		return
	}
	layout := m.layout()

	m.activityList.SetSize(imax(10, layout.leftW-4), imax(2, layout.activityH-2))
	m.agentOutputVP.Width = imax(10, layout.leftW-4)
	m.agentOutputVP.Height = imax(2, layout.outputH-2)
	m.agentOutputVP.SetContent(strings.Join(m.agentOutput, "\n"))
	m.agentOutputVP.GotoBottom()

	m.taskQueueVP.Width = imax(10, layout.rightW-4)
	m.taskQueueVP.Height = imax(2, layout.queueH-2)
	m.taskQueueVP.SetContent(renderTaskQueue(m.taskQueue))

	m.outboxVP.Width = imax(10, m.width-4)
	m.outboxVP.Height = imax(2, layout.outboxH-2)
	m.outboxVP.SetContent(renderOutboxLines(m.outboxResults))

	m.memoryVP.Width = imax(10, m.width-4)
	m.memoryVP.Height = imax(2, layout.memoryH-2)
	m.memoryVP.SetContent(renderMemResults(m.memResults))

	m.input.SetWidth(imax(10, m.width-6))
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
	return fmt.Sprintf("Tasks done: %d\nUptime: %s", s.tasksDone, fallback(uptime, "unknown"))
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
