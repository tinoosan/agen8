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
	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/resources"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/timeutil"
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

type uiRefreshMsg struct{}

type planReloadMsg struct{}

type sessionTotalsReloadMsg struct{}

type planFilesLoadedMsg struct {
	checklist    string
	checklistErr string
	details      string
	detailsErr   string
}

type sessionTotalsLoadedMsg struct {
	session types.Session
	err     error
}

type inboxLoadedMsg struct {
	tasks      []taskState
	totalCount int
	page       int
}

type outboxLoadedMsg struct {
	entries    []outboxEntry
	totalCount int
	page       int
}

type teamStatusLoadedMsg struct {
	pending      int
	active       int
	done         int
	roles        []teamRoleState
	runIDs       []string
	roleByRunID  map[string]string
	totalTokens  int
	totalCostUSD float64
	pricingKnown bool
}

type teamManifestLoadedMsg struct {
	manifest *teamManifestFile
	err      error
}

type teamEventsLoadedMsg struct {
	events  []types.EventRecord
	cursors map[string]int64
}

type activityLoadedMsg struct {
	activities []Activity
	totalCount int
	page       int
}

type MonitorResult struct {
	SwitchToRunID string
}

type monitorModel struct {
	ctx       context.Context
	cfg       config.Config
	runID     string
	teamID    string
	runStatus string // loaded at init; used to show "run not active" warning
	result    *MonitorResult
	session   pkgstore.SessionQuery
	sessionID string

	offset int64

	input textarea.Model

	activityPageItems          []Activity
	activityPage               int
	activityPageSize           int
	activityTotalCount         int
	activityList               list.Model
	activityDetail             viewport.Model
	activityDetailAct          string
	activityFollowingTail      bool
	planViewport               viewport.Model
	planFollowingTop           bool
	renderer                   *ContentRenderer
	agentOutput                []string
	agentOutputRunID           []string
	agentOutputFilteredCache   []string
	agentOutputVP              viewport.Model
	agentOutputFollow          bool
	agentOutputPending         map[string]agentOutputPendingEntry
	agentOutputPendingFallback *agentOutputPendingEntry
	agentOutputLogicalYOffset  int
	agentOutputTotalLines      int
	agentOutputLayoutWidth     int
	agentOutputLineStarts      []int
	agentOutputLineHeights     []int
	agentOutputWindowStartLine int

	taskStore agentstate.TaskStore

	inbox                        map[string]taskState
	inboxVP                      viewport.Model
	currentTask                  *taskState
	inboxList                    []taskState
	inboxPage                    int
	inboxPageSize                int
	inboxTotalCount              int
	outboxResults                []outboxEntry
	outboxVP                     viewport.Model
	outboxPage                   int
	outboxPageSize               int
	outboxTotalCount             int
	memResults                   []string
	memoryVP                     viewport.Model
	thinkingEntries              []thinkingEntry
	thinkingVP                   viewport.Model
	thinkingAutoScroll           bool
	planMarkdown                 string
	planDetails                  string
	planLoadErr                  string
	planDetailsErr               string
	planReloadScheduled          bool
	planReloadDebounce           time.Duration
	sessionTotalsReloadScheduled bool
	sessionTotalsReloadDebounce  time.Duration
	stats                        monitorStats
	model                        string
	profile                      string
	focusedPanel                 panelID
	compactTab                   int // 0=Output, 1=Activity, 2=Plan, 3=Outbox; used when isCompactMode()
	dashboardSideTab             int // 0=Activity, 1=Plan, 2=Tasks, 3=Thoughts; used when dashboard mode
	width                        int
	height                       int
	styles                       *monitorStyles
	tailCh                       <-chan store.TailedEvent
	errCh                        <-chan error
	cancel                       context.CancelFunc

	// Modal overlay state (only one modal open at a time)
	helpModalOpen bool

	// Session picker
	sessionPickerOpen     bool
	sessionPickerList     list.Model
	sessionPickerErr      string
	sessionPickerPage     int
	sessionPickerPageSize int
	sessionPickerTotal    int
	sessionPickerFilter   string

	// Model picker
	modelPickerOpen         bool
	modelPickerList         list.Model
	modelPickerProvider     string
	modelPickerQuery        string
	modelPickerProviderView bool

	// Profile picker
	profilePickerOpen bool
	profilePickerList list.Model

	// Team picker
	teamPickerOpen bool
	teamPickerList list.Model

	// Command palette (inline autocomplete above composer)
	commandPaletteOpen     bool
	commandPaletteMatches  []string
	commandPaletteSelected int

	// Artifact viewer (full-screen takeover)
	artifactViewerOpen      bool
	artifactTasks           []types.Task
	artifactTree            []artifactTreeNode
	artifactSelected        int
	artifactContent         string
	artifactContentRaw      string
	artifactContentVP       viewport.Model
	artifactRenderWidth     int
	artifactRenderRawLen    int
	artifactRenderedVPath   string
	artifactSearchMode      bool
	artifactSearchQuery     string
	artifactNavFocused      bool
	artifactSelectedVPath   string
	artifactWorkspaceFiles  []artifactTreeNode
	artifactRoleExpanded    map[string]bool
	artifactTaskExpanded    map[string]bool
	artifactWorkspaceExpand map[string]bool

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

	// Incremental rendering (avoid rebuilding all viewports on every event).
	dirtyLayout      bool
	dirtyAgentOutput bool
	dirtyInbox       bool
	dirtyOutbox      bool
	dirtyActivity    bool
	dirtyPlan        bool
	dirtyThinking    bool
	dirtyMemory      bool

	uiRefreshScheduled bool
	uiRefreshDebounce  time.Duration

	teamPendingCount     int
	teamActiveCount      int
	teamDoneCount        int
	focusedRunID         string
	focusedRunRole       string
	teamRoles            []teamRoleState
	seenOutboxByTask     map[string]struct{}
	teamRunIDs           []string
	teamRoleByRunID      map[string]string
	teamCoordinatorRunID string
	teamCoordinatorRole  string
	teamEventCursor      map[string]int64
	teamModelChange      *teamModelChangeFile
}

type monitorStats struct {
	started time.Time

	tasksDone int

	lastTurnTokensIn  int
	lastTurnTokensOut int
	lastTurnTokens    int

	totalTokensIn  int
	totalTokensOut int
	totalTokens    int

	lastTurnCostUSD string
	totalCostUSD    float64
	pricingKnown    bool
}

func pricingKnownForRunID(cfg config.Config, runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return false
	}
	run, err := store.LoadRun(cfg, runID)
	if err != nil {
		return false
	}
	if run.Runtime == nil {
		return false
	}
	if run.Runtime.PriceInPerMTokensUSD != 0 || run.Runtime.PriceOutPerMTokensUSD != 0 {
		return true
	}
	modelID := strings.TrimSpace(run.Runtime.Model)
	if modelID == "" {
		return false
	}
	_, _, ok := cost.DefaultPricing().Lookup(modelID)
	return ok
}

type taskState struct {
	TaskID       string
	AssignedRole string
	Goal         string
	Status       string
	StartedAt    time.Time
}

type outboxEntry struct {
	TaskID         string
	RunID          string
	AssignedRole   string
	Goal           string
	Status         string
	Summary        string
	Error          string
	SummaryPath    string
	ArtifactsCount int
	InputTokens    int
	OutputTokens   int
	TotalTokens    int
	CostUSD        float64
	Timestamp      time.Time
}

type thinkingEntry struct {
	RunID   string
	Role    string
	Summary string
}

type teamRoleState struct {
	Role string
	Info string
}

type teamManifestFile struct {
	TeamID          string               `json:"teamId"`
	ProfileID       string               `json:"profileId"`
	TeamModel       string               `json:"teamModel,omitempty"`
	ModelChange     *teamModelChangeFile `json:"modelChange,omitempty"`
	CoordinatorRole string               `json:"coordinatorRole"`
	CoordinatorRun  string               `json:"coordinatorRunId"`
	Roles           []teamManifestRole   `json:"roles"`
	CreatedAt       string               `json:"createdAt"`
}

type teamModelChangeFile struct {
	RequestedModel string `json:"requestedModel,omitempty"`
	Status         string `json:"status,omitempty"`
	RequestedAt    string `json:"requestedAt,omitempty"`
	AppliedAt      string `json:"appliedAt,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Error          string `json:"error,omitempty"`
}

type teamManifestRole struct {
	RoleName  string `json:"roleName"`
	RunID     string `json:"runId"`
	SessionID string `json:"sessionId"`
}

type agentOutputPendingEntry struct {
	index     int
	timestamp string
	desc      string
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

const (
	// Keep a large, bounded agent output history to avoid unbounded RAM growth.
	// This replaces the old 1000-line hard limit with a much larger buffer.
	agentOutputMaxLines  = 50_000
	agentOutputDropChunk = 5_000

	// Keep a small, bounded thoughts history to avoid unbounded RAM growth.
	maxThinkingEntries = 50
)

// Breakpoints for responsive layout: below these use compact mode (tabs/single column).
const (
	compactModeMinWidth  = 110
	compactModeMinHeight = 32
)

func RunMonitor(ctx context.Context, cfg config.Config, runID string) error {
	var result MonitorResult
	m, err := newMonitorModel(ctx, cfg, runID, &result)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err == nil && strings.TrimSpace(result.SwitchToRunID) != "" {
		return &MonitorSwitchRunError{RunID: strings.TrimSpace(result.SwitchToRunID)}
	}
	return err
}

func RunTeamMonitor(ctx context.Context, cfg config.Config, teamID string) error {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return fmt.Errorf("teamID is required")
	}
	var result MonitorResult
	m, err := newTeamMonitorModel(ctx, cfg, teamID, &result)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func newMonitorModel(ctx context.Context, cfg config.Config, runID string, result *MonitorResult) (*monitorModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, fmt.Errorf("runID is required")
	}

	stats := monitorStats{started: time.Now()}

	taskStore, err := agentstate.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		return nil, err
	}
	if rs, err := taskStore.GetRunStats(ctx, runID); err == nil {
		// Best-effort: show tasks already completed before monitor attached.
		stats.tasksDone = rs.Succeeded + rs.Failed
	}

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
	// Best-effort: load a small recent window for initial display without scanning the full log.
	var evs []types.EventRecord
	{
		filter := store.EventFilter{
			RunID:    runID,
			Limit:    200,
			SortDesc: true, // newest first
		}
		if recent, _, err := store.ListEventsPaginated(cfg, filter); err == nil {
			// Observe in chronological order (oldest -> newest) to preserve ordering semantics.
			slices.Reverse(recent)
			evs = recent
		}
	}

	off := int64(0)
	if latest, err := store.GetLatestEventSeq(cfg, runID); err == nil {
		off = latest
	}
	tailCh, errCh := store.TailEvents(cfg, tctx, runID, off)

	runStatus := types.RunStatusSucceeded
	runSessionID := ""
	if r, err := store.LoadRun(cfg, runID); err == nil {
		runStatus = r.Status
		runSessionID = strings.TrimSpace(r.SessionID)
	}

	sessionStore, err := store.NewSQLiteSessionStore(cfg)
	if err != nil {
		return nil, err
	}

	if runSessionID != "" {
		if sess, err := sessionStore.LoadSession(ctx, runSessionID); err == nil {
			stats.totalTokensIn = sess.InputTokens
			stats.totalTokensOut = sess.OutputTokens
			stats.totalTokens = sess.TotalTokens
			stats.totalCostUSD = sess.CostUSD
			stats.pricingKnown = sess.TotalTokens == 0 || sess.CostUSD > 0 || pricingKnownForRunID(cfg, runID)
		}
	}

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		runID:                       runID,
		runStatus:                   runStatus,
		result:                      result,
		session:                     sessionStore,
		sessionID:                   runSessionID,
		offset:                      off,
		input:                       in,
		activityPageItems:           []Activity{},
		activityPage:                0,
		activityPageSize:            200,
		activityTotalCount:          0,
		activityList:                activityList,
		activityDetail:              viewport.New(0, 0),
		activityFollowingTail:       true,
		planViewport:                viewport.New(0, 0),
		planFollowingTop:            true,
		renderer:                    newContentRenderer(),
		agentOutput:                 []string{},
		agentOutputRunID:            []string{},
		agentOutputVP:               viewport.New(0, 0),
		agentOutputFollow:           true,
		inbox:                       map[string]taskState{},
		inboxVP:                     viewport.New(0, 0),
		inboxList:                   []taskState{},
		inboxPage:                   0,
		inboxPageSize:               50,
		outboxResults:               []outboxEntry{},
		outboxVP:                    viewport.New(0, 0),
		outboxPage:                  0,
		outboxPageSize:              50,
		memResults:                  []string{},
		memoryVP:                    viewport.New(0, 0),
		thinkingEntries:             []thinkingEntry{},
		thinkingVP:                  viewport.New(0, 0),
		thinkingAutoScroll:          true,
		artifactContentVP:           viewport.New(0, 0),
		taskStore:                   taskStore,
		stats:                       stats,
		styles:                      defaultMonitorStyles(),
		focusedPanel:                panelComposer,
		tailCh:                      tailCh,
		errCh:                       errCh,
		cancel:                      cancel,
		uiRefreshDebounce:           33 * time.Millisecond,
		planReloadDebounce:          100 * time.Millisecond,
		sessionTotalsReloadDebounce: 150 * time.Millisecond,
		seenOutboxByTask:            map[string]struct{}{},
		teamRoleByRunID:             map[string]string{},
		teamEventCursor:             map[string]int64{},
	}
	// Disable mouse handling so terminals don't enter mouse-reporting mode.
	m.activityDetail.MouseWheelEnabled = false
	m.planViewport.MouseWheelEnabled = false
	m.agentOutputVP.MouseWheelEnabled = false
	m.inboxVP.MouseWheelEnabled = false
	m.outboxVP.MouseWheelEnabled = false
	m.memoryVP.MouseWheelEnabled = false
	m.thinkingVP.MouseWheelEnabled = false
	m.artifactContentVP.MouseWheelEnabled = false

	for _, e := range evs {
		m.observeEvent(e)
	}
	// Activity feed is loaded from SQLite (paginated) via loadActivityPage.
	m.loadPlanFiles()
	m.refreshViewports()

	return m, nil
}

func newTeamMonitorModel(ctx context.Context, cfg config.Config, teamID string, result *MonitorResult) (*monitorModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, fmt.Errorf("teamID is required")
	}
	taskStore, err := agentstate.NewSQLiteTaskStore(fsutil.GetSQLitePath(cfg.DataDir))
	if err != nil {
		return nil, err
	}
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
	manifest, _ := loadTeamManifest(cfg.DataDir, teamID)
	teamRoleByRun := map[string]string{}
	teamRunIDs := []string{}
	teamCoordinatorRunID := ""
	teamCoordinatorRole := ""
	if manifest != nil {
		teamCoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
		teamCoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
		for _, role := range manifest.Roles {
			runID := strings.TrimSpace(role.RunID)
			roleName := strings.TrimSpace(role.RoleName)
			if runID == "" {
				continue
			}
			teamRoleByRun[runID] = roleName
			teamRunIDs = append(teamRunIDs, runID)
		}
	}
	teamEventCursor := map[string]int64{}
	for _, runID := range teamRunIDs {
		if latest, err := store.GetLatestEventSeq(cfg, runID); err == nil {
			start := latest - 150
			if start < 0 {
				start = 0
			}
			teamEventCursor[runID] = start
		} else {
			teamEventCursor[runID] = 0
		}
	}

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		runID:                       "team:" + teamID,
		teamID:                      teamID,
		runStatus:                   types.RunStatusRunning,
		result:                      result,
		session:                     nil,
		sessionID:                   "",
		offset:                      0,
		input:                       in,
		activityPageItems:           []Activity{},
		activityPage:                0,
		activityPageSize:            200,
		activityTotalCount:          0,
		activityList:                activityList,
		activityDetail:              viewport.New(0, 0),
		activityFollowingTail:       true,
		planViewport:                viewport.New(0, 0),
		planFollowingTop:            true,
		renderer:                    newContentRenderer(),
		agentOutput:                 []string{},
		agentOutputRunID:            []string{},
		agentOutputVP:               viewport.New(0, 0),
		agentOutputFollow:           true,
		inbox:                       map[string]taskState{},
		inboxVP:                     viewport.New(0, 0),
		inboxList:                   []taskState{},
		inboxPage:                   0,
		inboxPageSize:               50,
		outboxResults:               []outboxEntry{},
		outboxVP:                    viewport.New(0, 0),
		outboxPage:                  0,
		outboxPageSize:              50,
		memResults:                  []string{},
		memoryVP:                    viewport.New(0, 0),
		thinkingEntries:             []thinkingEntry{},
		thinkingVP:                  viewport.New(0, 0),
		thinkingAutoScroll:          true,
		artifactContentVP:           viewport.New(0, 0),
		taskStore:                   taskStore,
		stats:                       monitorStats{started: time.Now()},
		styles:                      defaultMonitorStyles(),
		focusedPanel:                panelComposer,
		uiRefreshDebounce:           33 * time.Millisecond,
		planReloadDebounce:          100 * time.Millisecond,
		sessionTotalsReloadDebounce: 150 * time.Millisecond,
		seenOutboxByTask:            map[string]struct{}{},
		teamRunIDs:                  teamRunIDs,
		teamRoleByRunID:             teamRoleByRun,
		teamCoordinatorRunID:        teamCoordinatorRunID,
		teamCoordinatorRole:         teamCoordinatorRole,
		teamEventCursor:             teamEventCursor,
	}
	m.activityDetail.MouseWheelEnabled = false
	m.planViewport.MouseWheelEnabled = false
	m.agentOutputVP.MouseWheelEnabled = false
	m.inboxVP.MouseWheelEnabled = false
	m.outboxVP.MouseWheelEnabled = false
	m.memoryVP.MouseWheelEnabled = false
	m.thinkingVP.MouseWheelEnabled = false
	m.artifactContentVP.MouseWheelEnabled = false
	if manifest != nil {
		m.profile = strings.TrimSpace(manifest.ProfileID)
		m.model = strings.TrimSpace(manifest.TeamModel)
		m.teamModelChange = manifest.ModelChange
		m.teamCoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
		m.teamCoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
	}
	if m.profile == "" {
		m.profile = "team"
	}
	if m.model == "" {
		m.model = "team"
	}
	m.refreshViewports()
	return m, nil
}

// loadPendingTasksFromSQLite queries pending tasks for the run. Used so the queue
// shows tasks added before the monitor started or via webhook, without scanning
// inbox files.
func (m *monitorModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.listenEvent(), m.listenErr(), m.tick(), m.loadInboxPage(), m.loadOutboxPage(), m.loadActivityPage()}
	if strings.TrimSpace(m.teamID) != "" {
		cmds = append(cmds, m.loadTeamStatus(), m.loadTeamEvents(), m.loadPlanFilesCmd(), m.loadTeamManifestCmd())
	}
	return tea.Batch(cmds...)
}

func (m *monitorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.artifactViewerOpen {
			m.refreshArtifactViewport()
			return m, nil
		}
		m.dirtyLayout = true
		m.refreshViewports()
		return m, nil

	case tickMsg:
		// Re-render time-based UI (uptime, elapsed timers) even when no new events arrive.
		// The View() computes elapsed durations on demand; no need to rebuild viewports.
		if strings.TrimSpace(m.teamID) != "" {
			return m, tea.Batch(m.tick(), m.loadInboxPage(), m.loadOutboxPage(), m.loadActivityPage(), m.loadTeamStatus(), m.loadTeamEvents(), m.loadPlanFilesCmd(), m.loadTeamManifestCmd())
		}
		return m, m.tick()

	case tailedEventMsg:
		if msg.ev.Event.EventID != "" {
			m.offset = msg.ev.NextOffset
			m.observeEvent(msg.ev.Event)
		}
		cmds := []tea.Cmd{m.listenEvent()}
		if shouldReloadPlanOnEvent(msg.ev.Event) {
			cmds = append(cmds, m.schedulePlanReload())
		}
		switch msg.ev.Event.Type {
		case "llm.cost.total", "llm.usage.total":
			cmds = append(cmds, m.scheduleSessionTotalsReload())
		}
		// Keep paginated lists in sync without loading everything into memory.
		switch msg.ev.Event.Type {
		case "task.queued", "webhook.task.queued", "task.start":
			cmds = append(cmds, m.loadInboxPage())
		case "task.done", "task.quarantined":
			cmds = append(cmds, m.loadInboxPage(), m.loadOutboxPage())
		case "task.delivered":
			cmds = append(cmds, m.loadOutboxPage())
		case "agent.op.request", "agent.op.response":
			if m.activityFollowingTail {
				// If a new page is created, overshoot so loadActivityPage clamps to the new last page.
				m.activityPage = max(0, (m.activityTotalCount+m.activityPageSize-1)/max(1, m.activityPageSize))
				cmds = append(cmds, m.loadActivityPage())
			} else if msg.ev.Event.Type == "agent.op.response" {
				opID := strings.TrimSpace(msg.ev.Event.Data["opId"])
				if opID != "" {
					for _, a := range m.activityPageItems {
						if strings.TrimSpace(a.ID) == opID {
							cmds = append(cmds, m.loadActivityPage())
							break
						}
					}
				}
			}
		}
		cmds = append(cmds, m.scheduleUIRefresh())
		return m, tea.Batch(cmds...)

	case tailErrMsg:
		if msg.err != nil {
			m.appendAgentOutput("[error] " + msg.err.Error())
		}
		return m, tea.Batch(m.listenErr(), m.scheduleUIRefresh())

	case commandLinesMsg:
		if len(msg.lines) != 0 {
			for _, line := range msg.lines {
				m.appendAgentOutput(line)
			}
			if strings.HasPrefix(strings.TrimSpace(msg.lines[0]), "[memory] search:") {
				m.memResults = msg.lines[1:]
				m.dirtyMemory = true
			}
		}
		return m, m.scheduleUIRefresh()

	case monitorEditorDoneMsg:
		m.handleEditorDone(msg)
		return m, m.scheduleUIRefresh()

	case taskQueuedLocallyMsg:
		m.inbox[msg.TaskID] = taskState{TaskID: msg.TaskID, Goal: msg.Goal, Status: string(types.TaskStatusPending)}
		m.dirtyInbox = true
		return m, tea.Batch(m.loadInboxPage(), m.scheduleUIRefresh())

	case inboxLoadedMsg:
		m.inboxList = msg.tasks
		m.inboxTotalCount = msg.totalCount
		m.inboxPage = msg.page
		m.dirtyInbox = true
		return m, m.scheduleUIRefresh()

	case outboxLoadedMsg:
		m.outboxResults = msg.entries
		m.outboxTotalCount = msg.totalCount
		m.outboxPage = msg.page
		if strings.TrimSpace(m.teamID) != "" {
			for _, entry := range msg.entries {
				if strings.TrimSpace(entry.TaskID) == "" {
					continue
				}
				if _, ok := m.seenOutboxByTask[entry.TaskID]; ok {
					continue
				}
				m.seenOutboxByTask[entry.TaskID] = struct{}{}
				rolePrefix := strings.TrimSpace(entry.AssignedRole)
				if rolePrefix == "" {
					rolePrefix = "team"
				}
				ts := entry.Timestamp.Local().Format("15:04:05")
				line := fmt.Sprintf("[%s] [%s] task.done %s %s", ts, rolePrefix, shortID(entry.TaskID), strings.TrimSpace(entry.Status))
				if summary := strings.TrimSpace(entry.Summary); summary != "" {
					line += " - " + truncateText(summary, 120)
				}
				m.appendAgentOutputForRun(line, strings.TrimSpace(entry.RunID))
			}
		}
		m.dirtyOutbox = true
		return m, m.scheduleUIRefresh()

	case teamStatusLoadedMsg:
		m.teamPendingCount = msg.pending
		m.teamActiveCount = msg.active
		m.teamDoneCount = msg.done
		m.teamRoles = msg.roles
		if len(msg.runIDs) != 0 {
			m.teamRunIDs = msg.runIDs
		}
		if len(msg.roleByRunID) != 0 {
			if m.teamRoleByRunID == nil {
				m.teamRoleByRunID = map[string]string{}
			}
			for runID, role := range msg.roleByRunID {
				m.teamRoleByRunID[runID] = role
			}
		}
		m.stats.totalTokens = msg.totalTokens
		m.stats.totalCostUSD = msg.totalCostUSD
		m.stats.pricingKnown = msg.pricingKnown
		return m, tea.Batch(m.ensureFocusedRunStillValid(), m.scheduleUIRefresh())

	case teamManifestLoadedMsg:
		if msg.err != nil || msg.manifest == nil {
			return m, nil
		}
		manifest := msg.manifest
		if profileID := strings.TrimSpace(manifest.ProfileID); profileID != "" {
			m.profile = profileID
		}
		if teamModel := strings.TrimSpace(manifest.TeamModel); teamModel != "" {
			m.model = teamModel
		}
		m.teamModelChange = manifest.ModelChange
		m.teamCoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
		m.teamCoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
		if len(manifest.Roles) != 0 {
			roleByRun := map[string]string{}
			runIDs := make([]string, 0, len(manifest.Roles))
			for _, role := range manifest.Roles {
				runID := strings.TrimSpace(role.RunID)
				if runID == "" {
					continue
				}
				runIDs = append(runIDs, runID)
				roleByRun[runID] = strings.TrimSpace(role.RoleName)
			}
			if len(runIDs) != 0 {
				m.teamRunIDs = runIDs
				m.teamRoleByRunID = roleByRun
			}
		}
		return m, tea.Batch(m.ensureFocusedRunStillValid(), m.scheduleUIRefresh())

	case teamEventsLoadedMsg:
		if len(msg.cursors) != 0 {
			if m.teamEventCursor == nil {
				m.teamEventCursor = map[string]int64{}
			}
			for runID, cursor := range msg.cursors {
				m.teamEventCursor[runID] = cursor
			}
		}
		if len(msg.events) != 0 {
			reloadPlan := false
			for _, ev := range msg.events {
				m.observeEvent(ev)
				if shouldReloadPlanOnEvent(ev) {
					reloadPlan = true
				}
			}
			if reloadPlan {
				return m, tea.Batch(m.loadPlanFilesCmd(), m.scheduleUIRefresh())
			}
		}
		return m, m.scheduleUIRefresh()

	case activityLoadedMsg:
		m.activityPageItems = msg.activities
		m.activityTotalCount = msg.totalCount
		m.activityPage = msg.page
		m.dirtyActivity = true
		return m, m.scheduleUIRefresh()

	case sessionsListMsg:
		if msg.err != nil {
			m.sessionPickerErr = msg.err.Error()
			m.sessionPickerList.SetItems(nil)
			return m, m.scheduleUIRefresh()
		}
		m.sessionPickerErr = ""
		m.sessionPickerTotal = msg.total
		m.sessionPickerPage = msg.page
		items := sessionsToPickerItems(msg.sessions)
		m.sessionPickerList.SetItems(items)
		if len(items) > 0 {
			m.sessionPickerList.Select(0)
		}
		return m, m.scheduleUIRefresh()

	case uiRefreshMsg:
		m.uiRefreshScheduled = false
		m.refreshViewports()
		return m, nil

	case planReloadMsg:
		m.planReloadScheduled = false
		return m, m.loadPlanFilesCmd()

	case sessionTotalsReloadMsg:
		m.sessionTotalsReloadScheduled = false
		return m, m.loadSessionTotalsCmd()

	case planFilesLoadedMsg:
		m.planMarkdown = msg.checklist
		m.planLoadErr = msg.checklistErr
		m.planDetails = msg.details
		m.planDetailsErr = msg.detailsErr
		m.dirtyPlan = true
		return m, m.scheduleUIRefresh()

	case sessionTotalsLoadedMsg:
		if msg.err == nil {
			m.stats.totalTokensIn = msg.session.InputTokens
			m.stats.totalTokensOut = msg.session.OutputTokens
			m.stats.totalTokens = msg.session.TotalTokens
			m.stats.totalCostUSD = msg.session.CostUSD
			m.stats.pricingKnown = msg.session.TotalTokens == 0 || msg.session.CostUSD > 0 || pricingKnownForRunID(m.cfg, m.runID)
		}
		return m, m.scheduleUIRefresh()

	case artifactTreeLoadedMsg:
		return m, tea.Batch(m.handleArtifactTreeLoaded(msg), m.scheduleUIRefresh())

	case artifactContentLoadedMsg:
		m.handleArtifactContentLoaded(msg)
		return m, m.scheduleUIRefresh()

	case monitorFilePickerPathsMsg:
		m.handleFilePickerPaths(msg.paths)
		return m, nil

	case tea.KeyMsg:
		if m.artifactViewerOpen {
			return m.updateArtifactViewer(msg)
		}
		// Modal overlay handling - if any modal is open, handle it first
		if m.helpModalOpen {
			switch msg.String() {
			case "esc", "escape", "?":
				m.closeHelpModal()
				return m, nil
			}
			return m, nil // Consume all other keys when help is open
		}
		if m.sessionPickerOpen {
			return m.updateSessionPicker(msg)
		}
		if m.profilePickerOpen {
			return m.updateProfilePicker(msg)
		}
		if m.teamPickerOpen {
			return m.updateTeamPicker(msg)
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

		// Help modal hotkey:
		// - When not composing, "?" should always open help.
		// - When composing, allow "?" to open help only if the composer is empty
		//   (otherwise treat it as a literal character the user wants to type).
		if msg.String() == "?" {
			if m.focusedPanel != panelComposer || strings.TrimSpace(m.input.Value()) == "" {
				m.openHelpModal()
				return m, nil
			}
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
		case "ctrl+g":
			if strings.TrimSpace(m.teamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
				m.focusedRunID = ""
				m.focusedRunRole = ""
				return m, m.applyFocusLens()
			}
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
			m.refreshViewports()
			return m, nil
		case "ctrl+]":
			if !m.isCompactMode() {
				m.dashboardSideTab = (m.dashboardSideTab + 1) % len(dashboardSideTabNames)
				m.focusedPanel = m.dashboardSideTabToPanel()
				m.updateFocus()
				m.refreshViewports()
				return m, nil
			}
		case "ctrl+[":
			if !m.isCompactMode() {
				m.dashboardSideTab = (m.dashboardSideTab + len(dashboardSideTabNames) - 1) % len(dashboardSideTabNames)
				m.focusedPanel = m.dashboardSideTabToPanel()
				m.updateFocus()
				m.refreshViewports()
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

		// Pagination controls for paginated panels when focused.
		if m.focusedPanel == panelActivity || m.focusedPanel == panelInbox || m.focusedPanel == panelOutbox {
			switch msg.String() {
			case "n", "right":
				return m.handleNextPage()
			case "p", "left":
				return m.handlePrevPage()
			case "g":
				return m.handleFirstPage()
			case "G":
				return m.handleLastPage()
			}
		}
		return m.routeKeyToFocusedPanel(msg)
	}

	return m, nil
}

func (m *monitorModel) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg{now: t} })
}

func (m *monitorModel) scheduleUIRefresh() tea.Cmd {
	if m == nil {
		return nil
	}
	if m.uiRefreshScheduled {
		return nil
	}
	m.uiRefreshScheduled = true
	d := m.uiRefreshDebounce
	if d <= 0 {
		d = 0
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return uiRefreshMsg{} })
}

func (m *monitorModel) schedulePlanReload() tea.Cmd {
	if m == nil {
		return nil
	}
	if m.planReloadScheduled {
		return nil
	}
	m.planReloadScheduled = true
	d := m.planReloadDebounce
	if d <= 0 {
		d = 0
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return planReloadMsg{} })
}

func (m *monitorModel) scheduleSessionTotalsReload() tea.Cmd {
	if m == nil {
		return nil
	}
	if m.sessionTotalsReloadScheduled {
		return nil
	}
	m.sessionTotalsReloadScheduled = true
	d := m.sessionTotalsReloadDebounce
	if d <= 0 {
		d = 0
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return sessionTotalsReloadMsg{} })
}

func (m *monitorModel) loadSessionTotalsCmd() tea.Cmd {
	if m == nil || m.session == nil || strings.TrimSpace(m.sessionID) == "" {
		return nil
	}
	sessionID := strings.TrimSpace(m.sessionID)
	return func() tea.Msg {
		sess, err := m.session.LoadSession(m.ctx, sessionID)
		return sessionTotalsLoadedMsg{session: sess, err: err}
	}
}

func (m *monitorModel) loadInboxPage() tea.Cmd {
	if m == nil || m.taskStore == nil {
		return nil
	}
	pageSize := m.inboxPageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	page := m.inboxPage
	if page < 0 {
		page = 0
	}
	prevTasks := append([]taskState(nil), m.inboxList...)
	prevTotal := m.inboxTotalCount
	prevPage := m.inboxPage

	return func() tea.Msg {
		totalFilter := m.scopedTaskFilter(agentstate.TaskFilter{
			Status:   []types.TaskStatus{types.TaskStatusPending, types.TaskStatusActive},
			Limit:    0,
			Offset:   0,
			SortBy:   "",
			SortDesc: false,
		})
		total, err := m.taskStore.CountTasks(m.ctx, totalFilter)
		if err != nil {
			return inboxLoadedMsg{tasks: prevTasks, totalCount: prevTotal, page: prevPage}
		}
		if total <= 0 {
			return inboxLoadedMsg{tasks: []taskState{}, totalCount: 0, page: 0}
		}
		maxPage := (total + pageSize - 1) / pageSize
		if maxPage > 0 {
			maxPage--
		}
		if page > maxPage {
			page = maxPage
		}
		if page < 0 {
			page = 0
		}

		listFilter := m.scopedTaskFilter(agentstate.TaskFilter{
			Status:   []types.TaskStatus{types.TaskStatusPending, types.TaskStatusActive},
			SortBy:   "created_at",
			SortDesc: false,
			Limit:    pageSize,
			Offset:   page * pageSize,
		})
		tasks, err := m.taskStore.ListTasks(m.ctx, listFilter)
		if err != nil {
			return inboxLoadedMsg{tasks: prevTasks, totalCount: prevTotal, page: prevPage}
		}
		out := make([]taskState, 0, len(tasks))
		for _, t := range tasks {
			ts := taskState{
				TaskID:       strings.TrimSpace(t.TaskID),
				AssignedRole: strings.TrimSpace(t.AssignedRole),
				Goal:         strings.TrimSpace(t.Goal),
				Status:       strings.TrimSpace(string(t.Status)),
			}
			if timeutil.IsSet(t.StartedAt) {
				ts.StartedAt = *t.StartedAt
			}
			if ts.TaskID != "" {
				out = append(out, ts)
			}
		}
		return inboxLoadedMsg{tasks: out, totalCount: total, page: page}
	}
}

func (m *monitorModel) loadOutboxPage() tea.Cmd {
	if m == nil || m.taskStore == nil {
		return nil
	}
	pageSize := m.outboxPageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	page := m.outboxPage
	if page < 0 {
		page = 0
	}
	prevEntries := append([]outboxEntry(nil), m.outboxResults...)
	prevTotal := m.outboxTotalCount
	prevPage := m.outboxPage

	return func() tea.Msg {
		totalFilter := m.scopedTaskFilter(agentstate.TaskFilter{
			Status:   []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled},
			Limit:    0,
			Offset:   0,
			SortBy:   "",
			SortDesc: false,
		})
		total, err := m.taskStore.CountTasks(m.ctx, totalFilter)
		if err != nil {
			return outboxLoadedMsg{entries: prevEntries, totalCount: prevTotal, page: prevPage}
		}
		if total <= 0 {
			return outboxLoadedMsg{entries: []outboxEntry{}, totalCount: 0, page: 0}
		}
		maxPage := (total + pageSize - 1) / pageSize
		if maxPage > 0 {
			maxPage--
		}
		if page > maxPage {
			page = maxPage
		}
		if page < 0 {
			page = 0
		}

		listFilter := m.scopedTaskFilter(agentstate.TaskFilter{
			Status:   []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled},
			SortBy:   "finished_at",
			SortDesc: true,
			Limit:    pageSize,
			Offset:   page * pageSize,
		})
		tasks, err := m.taskStore.ListTasks(m.ctx, listFilter)
		if err != nil {
			return outboxLoadedMsg{entries: prevEntries, totalCount: prevTotal, page: prevPage}
		}
		out := make([]outboxEntry, 0, len(tasks))
		for _, t := range tasks {
			ts := time.Time{}
			if timeutil.IsSet(t.CompletedAt) {
				ts = *t.CompletedAt
			}
			out = append(out, outboxEntry{
				TaskID:       strings.TrimSpace(t.TaskID),
				RunID:        strings.TrimSpace(t.RunID),
				AssignedRole: strings.TrimSpace(t.AssignedRole),
				Goal:         strings.TrimSpace(t.Goal),
				Status:       strings.TrimSpace(string(t.Status)),
				Summary:      strings.TrimSpace(t.Summary),
				Error:        strings.TrimSpace(t.Error),
				InputTokens:  t.InputTokens,
				OutputTokens: t.OutputTokens,
				TotalTokens:  t.TotalTokens,
				CostUSD:      t.CostUSD,
				Timestamp:    ts,
			})
		}
		return outboxLoadedMsg{entries: out, totalCount: total, page: page}
	}
}

func (m *monitorModel) scopedTaskFilter(filter agentstate.TaskFilter) agentstate.TaskFilter {
	if strings.TrimSpace(m.teamID) != "" && strings.TrimSpace(m.focusedRunID) != "" {
		filter.TeamID = strings.TrimSpace(m.teamID)
		filter.RunID = strings.TrimSpace(m.focusedRunID)
		filter.AssignedRole = ""
		return filter
	}
	if strings.TrimSpace(m.teamID) != "" {
		filter.TeamID = strings.TrimSpace(m.teamID)
		filter.RunID = ""
	}
	return filter
}

func (m *monitorModel) loadTeamStatus() tea.Cmd {
	if m == nil || m.taskStore == nil || strings.TrimSpace(m.teamID) == "" {
		return nil
	}
	return func() tea.Msg {
		pending, _ := m.taskStore.CountTasks(m.ctx, agentstate.TaskFilter{
			TeamID: m.teamID,
			Status: []types.TaskStatus{types.TaskStatusPending},
		})
		active, _ := m.taskStore.CountTasks(m.ctx, agentstate.TaskFilter{
			TeamID: m.teamID,
			Status: []types.TaskStatus{types.TaskStatusActive},
		})
		done, _ := m.taskStore.CountTasks(m.ctx, agentstate.TaskFilter{
			TeamID: m.teamID,
			Status: []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled},
		})
		activeTasks, _ := m.taskStore.ListTasks(m.ctx, agentstate.TaskFilter{
			TeamID:   m.teamID,
			Status:   []types.TaskStatus{types.TaskStatusActive},
			SortBy:   "updated_at",
			SortDesc: true,
			Limit:    200,
		})
		pendingTasks, _ := m.taskStore.ListTasks(m.ctx, agentstate.TaskFilter{
			TeamID:   m.teamID,
			Status:   []types.TaskStatus{types.TaskStatusPending},
			SortBy:   "created_at",
			SortDesc: false,
			Limit:    200,
		})
		roleInfo := map[string]string{}
		roleByRunID := map[string]string{}
		for runID, role := range m.teamRoleByRunID {
			if strings.TrimSpace(runID) == "" {
				continue
			}
			roleByRunID[strings.TrimSpace(runID)] = strings.TrimSpace(role)
		}
		runIDSet := map[string]struct{}{}
		for runID := range roleByRunID {
			runIDSet[runID] = struct{}{}
		}
		for _, task := range pendingTasks {
			role := strings.TrimSpace(task.AssignedRole)
			if role == "" {
				role = "(coordinator)"
			}
			if strings.TrimSpace(task.RunID) != "" {
				if _, ok := roleByRunID[strings.TrimSpace(task.RunID)]; !ok {
					roleByRunID[strings.TrimSpace(task.RunID)] = role
				}
				runIDSet[strings.TrimSpace(task.RunID)] = struct{}{}
			}
			if _, exists := roleInfo[role]; exists {
				continue
			}
			roleInfo[role] = "pending: " + truncateText(strings.TrimSpace(task.Goal), 52)
		}
		for _, task := range activeTasks {
			role := strings.TrimSpace(task.AssignedRole)
			if role == "" {
				role = "(coordinator)"
			}
			if strings.TrimSpace(task.RunID) != "" {
				if _, ok := roleByRunID[strings.TrimSpace(task.RunID)]; !ok {
					roleByRunID[strings.TrimSpace(task.RunID)] = role
				}
				runIDSet[strings.TrimSpace(task.RunID)] = struct{}{}
			}
			roleInfo[role] = "active: " + truncateText(strings.TrimSpace(task.Goal), 52)
		}
		completedTasks, _ := m.taskStore.ListTasks(m.ctx, agentstate.TaskFilter{
			TeamID:   m.teamID,
			Status:   []types.TaskStatus{types.TaskStatusSucceeded, types.TaskStatusFailed, types.TaskStatusCanceled},
			SortBy:   "finished_at",
			SortDesc: true,
			Limit:    500,
		})
		for _, task := range completedTasks {
			role := strings.TrimSpace(task.AssignedRole)
			if role == "" {
				role = "(coordinator)"
			}
			if strings.TrimSpace(task.RunID) != "" {
				if _, ok := roleByRunID[strings.TrimSpace(task.RunID)]; !ok {
					roleByRunID[strings.TrimSpace(task.RunID)] = role
				}
				runIDSet[strings.TrimSpace(task.RunID)] = struct{}{}
			}
		}
		roles := make([]teamRoleState, 0, len(roleInfo))
		keys := make([]string, 0, len(roleInfo))
		for role := range roleInfo {
			keys = append(keys, role)
		}
		sort.Strings(keys)
		for _, role := range keys {
			roles = append(roles, teamRoleState{Role: role, Info: roleInfo[role]})
		}
		runIDs := make([]string, 0, len(runIDSet))
		for runID := range runIDSet {
			runIDs = append(runIDs, runID)
		}
		sort.Strings(runIDs)
		totalTokens := 0
		totalCostUSD := 0.0
		pricingKnown := true
		for _, runID := range runIDs {
			stats, err := m.taskStore.GetRunStats(m.ctx, runID)
			if err != nil {
				continue
			}
			totalTokens += stats.TotalTokens
			totalCostUSD += stats.TotalCost
			if stats.TotalTokens > 0 && stats.TotalCost <= 0 && !pricingKnownForRunID(m.cfg, runID) {
				pricingKnown = false
			}
		}
		if totalTokens == 0 {
			pricingKnown = true
		}
		return teamStatusLoadedMsg{
			pending:      pending,
			active:       active,
			done:         done,
			roles:        roles,
			runIDs:       runIDs,
			roleByRunID:  roleByRunID,
			totalTokens:  totalTokens,
			totalCostUSD: totalCostUSD,
			pricingKnown: pricingKnown,
		}
	}
}

func (m *monitorModel) loadTeamEvents() tea.Cmd {
	if m == nil || strings.TrimSpace(m.teamID) == "" {
		return nil
	}
	runIDs := append([]string(nil), m.teamRunIDs...)
	if len(runIDs) == 0 {
		return nil
	}
	roleByRun := map[string]string{}
	for k, v := range m.teamRoleByRunID {
		roleByRun[k] = v
	}
	cursors := map[string]int64{}
	for runID, cursor := range m.teamEventCursor {
		cursors[runID] = cursor
	}
	cfg := m.cfg
	return func() tea.Msg {
		all := make([]types.EventRecord, 0, 256)
		next := map[string]int64{}
		for _, runID := range runIDs {
			after := cursors[runID]
			batch, cursor, err := store.ListEventsPaginated(cfg, store.EventFilter{
				RunID:    runID,
				AfterSeq: after,
				Limit:    200,
				SortDesc: false,
			})
			if err != nil {
				continue
			}
			if cursor > 0 {
				next[runID] = cursor
			} else {
				next[runID] = after
			}
			role := strings.TrimSpace(roleByRun[runID])
			for _, ev := range batch {
				if ev.Data == nil {
					ev.Data = map[string]string{}
				}
				if strings.TrimSpace(ev.Data["role"]) == "" && role != "" {
					ev.Data["role"] = role
				}
				if strings.TrimSpace(ev.Data["teamId"]) == "" {
					ev.Data["teamId"] = strings.TrimSpace(m.teamID)
				}
				all = append(all, ev)
			}
		}
		sort.SliceStable(all, func(i, j int) bool {
			return all[i].Timestamp.Before(all[j].Timestamp)
		})
		return teamEventsLoadedMsg{
			events:  all,
			cursors: next,
		}
	}
}

func (m *monitorModel) loadTeamManifestCmd() tea.Cmd {
	if m == nil || strings.TrimSpace(m.teamID) == "" {
		return nil
	}
	dataDir := m.cfg.DataDir
	teamID := strings.TrimSpace(m.teamID)
	return func() tea.Msg {
		manifest, err := loadTeamManifest(dataDir, teamID)
		return teamManifestLoadedMsg{manifest: manifest, err: err}
	}
}

func (m *monitorModel) ensureFocusedRunStillValid() tea.Cmd {
	if m == nil || strings.TrimSpace(m.teamID) == "" || strings.TrimSpace(m.focusedRunID) == "" {
		return nil
	}
	target := strings.TrimSpace(m.focusedRunID)
	valid := false
	for _, runID := range m.teamRunIDs {
		if strings.TrimSpace(runID) == target {
			valid = true
			break
		}
	}
	if !valid {
		if _, ok := m.teamRoleByRunID[target]; ok {
			valid = true
		}
	}
	if valid {
		if role := strings.TrimSpace(m.teamRoleByRunID[target]); role != "" {
			m.focusedRunRole = role
		}
		return nil
	}
	m.focusedRunID = ""
	m.focusedRunRole = ""
	return m.applyFocusLens()
}

func (m *monitorModel) applyFocusLens() tea.Cmd {
	if m == nil {
		return nil
	}
	m.agentOutputFilteredCache = nil
	m.agentOutputLayoutWidth = 0
	m.agentOutputLineStarts = nil
	m.agentOutputLineHeights = nil
	m.agentOutputTotalLines = 0
	m.agentOutputWindowStartLine = 0
	m.dirtyAgentOutput = true
	m.dirtyActivity = true
	m.dirtyPlan = true
	m.dirtyThinking = true
	m.dirtyInbox = true
	m.dirtyOutbox = true

	return tea.Batch(
		m.loadActivityPage(),
		m.loadInboxPage(),
		m.loadOutboxPage(),
		m.loadPlanFilesCmd(),
		m.scheduleUIRefresh(),
	)
}

func (m *monitorModel) loadActivityPage() tea.Cmd {
	if m == nil {
		return nil
	}
	if strings.TrimSpace(m.teamID) != "" {
		pageSize := m.activityPageSize
		if pageSize <= 0 {
			pageSize = 200
		}
		page := m.activityPage
		if page < 0 {
			page = 0
		}
		prevActivities := append([]Activity(nil), m.activityPageItems...)
		prevTotal := m.activityTotalCount
		prevPage := m.activityPage
		runIDs := append([]string(nil), m.teamRunIDs...)
		if strings.TrimSpace(m.focusedRunID) != "" {
			runIDs = []string{strings.TrimSpace(m.focusedRunID)}
		}
		roleByRun := map[string]string{}
		for k, v := range m.teamRoleByRunID {
			roleByRun[k] = v
		}
		focusedRunID := strings.TrimSpace(m.focusedRunID)
		focusedRunRole := strings.TrimSpace(m.focusedRunRole)
		dataDir := m.cfg.DataDir
		return func() tea.Msg {
			if len(runIDs) == 0 {
				return activityLoadedMsg{activities: []Activity{}, totalCount: 0, page: 0}
			}
			merged := make([]Activity, 0, 512)
			for _, runID := range runIDs {
				acts, err := store.ListActivities(context.Background(), config.Config{DataDir: dataDir}, runID, 300, 0)
				if err != nil {
					continue
				}
				role := strings.TrimSpace(roleByRun[runID])
				if role == "" && focusedRunID != "" && runID == focusedRunID {
					role = focusedRunRole
				}
				for i := range acts {
					act := acts[i]
					if role != "" {
						act.Title = "[" + role + "] " + strings.TrimSpace(act.Title)
					}
					act.ID = runID + ":" + act.ID
					merged = append(merged, act)
				}
			}
			sort.SliceStable(merged, func(i, j int) bool {
				return merged[i].StartedAt.Before(merged[j].StartedAt)
			})
			total := len(merged)
			if total <= 0 {
				return activityLoadedMsg{activities: []Activity{}, totalCount: 0, page: 0}
			}
			maxPage := (total + pageSize - 1) / pageSize
			if maxPage > 0 {
				maxPage--
			}
			if page > maxPage {
				page = maxPage
			}
			if page < 0 {
				page = 0
			}
			start := page * pageSize
			end := start + pageSize
			if end > total {
				end = total
			}
			if start < 0 || start >= total || start > end {
				return activityLoadedMsg{activities: prevActivities, totalCount: prevTotal, page: prevPage}
			}
			out := make([]Activity, 0, end-start)
			out = append(out, merged[start:end]...)
			return activityLoadedMsg{activities: out, totalCount: total, page: page}
		}
	}
	pageSize := m.activityPageSize
	if pageSize <= 0 {
		pageSize = 200
	}
	page := m.activityPage
	if page < 0 {
		page = 0
	}
	prevActivities := append([]Activity(nil), m.activityPageItems...)
	prevTotal := m.activityTotalCount
	prevPage := m.activityPage

	return func() tea.Msg {
		total, err := store.CountActivities(m.ctx, m.cfg, m.runID)
		if err != nil {
			return activityLoadedMsg{activities: prevActivities, totalCount: prevTotal, page: prevPage}
		}
		if total <= 0 {
			return activityLoadedMsg{activities: []Activity{}, totalCount: 0, page: 0}
		}
		maxPage := (total + pageSize - 1) / pageSize
		if maxPage > 0 {
			maxPage--
		}
		if page > maxPage {
			page = maxPage
		}
		if page < 0 {
			page = 0
		}

		acts, err := store.ListActivities(m.ctx, m.cfg, m.runID, pageSize, page*pageSize)
		if err != nil {
			return activityLoadedMsg{activities: prevActivities, totalCount: prevTotal, page: prevPage}
		}
		out := make([]Activity, 0, len(acts))
		for _, a := range acts {
			out = append(out, a)
		}
		return activityLoadedMsg{activities: out, totalCount: total, page: page}
	}
}

func (m *monitorModel) handleNextPage() (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		maxPage := max(0, (m.activityTotalCount+m.activityPageSize-1)/max(1, m.activityPageSize)-1)
		if m.activityPage < maxPage {
			m.activityPage++
			m.activityFollowingTail = m.activityPage == maxPage
			return m, m.loadActivityPage()
		}
	case panelInbox:
		maxPage := max(0, (m.inboxTotalCount+m.inboxPageSize-1)/max(1, m.inboxPageSize)-1)
		if m.inboxPage < maxPage {
			m.inboxPage++
			return m, m.loadInboxPage()
		}
	case panelOutbox:
		maxPage := max(0, (m.outboxTotalCount+m.outboxPageSize-1)/max(1, m.outboxPageSize)-1)
		if m.outboxPage < maxPage {
			m.outboxPage++
			return m, m.loadOutboxPage()
		}
	}
	return m, nil
}

func (m *monitorModel) handlePrevPage() (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		if m.activityPage > 0 {
			m.activityPage--
			m.activityFollowingTail = false
			return m, m.loadActivityPage()
		}
	case panelInbox:
		if m.inboxPage > 0 {
			m.inboxPage--
			return m, m.loadInboxPage()
		}
	case panelOutbox:
		if m.outboxPage > 0 {
			m.outboxPage--
			return m, m.loadOutboxPage()
		}
	}
	return m, nil
}

func (m *monitorModel) handleFirstPage() (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		if m.activityPage != 0 {
			m.activityPage = 0
			m.activityFollowingTail = false
			return m, m.loadActivityPage()
		}
	case panelInbox:
		if m.inboxPage != 0 {
			m.inboxPage = 0
			return m, m.loadInboxPage()
		}
	case panelOutbox:
		if m.outboxPage != 0 {
			m.outboxPage = 0
			return m, m.loadOutboxPage()
		}
	}
	return m, nil
}

func (m *monitorModel) handleLastPage() (tea.Model, tea.Cmd) {
	switch m.focusedPanel {
	case panelActivity:
		maxPage := max(0, (m.activityTotalCount+m.activityPageSize-1)/max(1, m.activityPageSize)-1)
		if m.activityPage != maxPage {
			m.activityPage = maxPage
			m.activityFollowingTail = true
			return m, m.loadActivityPage()
		}
	case panelInbox:
		maxPage := max(0, (m.inboxTotalCount+m.inboxPageSize-1)/max(1, m.inboxPageSize)-1)
		if m.inboxPage != maxPage {
			m.inboxPage = maxPage
			return m, m.loadInboxPage()
		}
	case panelOutbox:
		maxPage := max(0, (m.outboxTotalCount+m.outboxPageSize-1)/max(1, m.outboxPageSize)-1)
		if m.outboxPage != maxPage {
			m.outboxPage = maxPage
			return m, m.loadOutboxPage()
		}
	}
	return m, nil
}

func (m *monitorModel) isCompactMode() bool {
	return m.width < compactModeMinWidth || m.height < compactModeMinHeight
}

func (m *monitorModel) View() string {
	if m.artifactViewerOpen {
		return m.renderArtifactViewer()
	}
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
	if m.sessionPickerOpen {
		return m.renderSessionPicker(base)
	}
	if m.profilePickerOpen {
		return m.renderProfilePicker(base)
	}
	if m.teamPickerOpen {
		return m.renderTeamPicker(base)
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
	if m.runStatus != types.RunStatusRunning {
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
	return lipgloss.NewStyle().MaxWidth(effectiveWidth).MaxHeight(m.height).Render(final)
}

func (m *monitorModel) renderStatusBar(width int) string {
	line := "Tab: focus  |  Ctrl+Enter: submit  |  /quit"
	if m.isCompactMode() {
		line += "  |  Ctrl+]/Ctrl+[ switch tab (Output | Activity | Plan | Outbox)  |  Ctrl+Up/Down focus Activity Feed/Details"
	} else {
		line += "  |  Ctrl+]/Ctrl+[ cycle side panel (Activity | Plan | Tasks | Thoughts)  |  Ctrl+Y Thoughts tab  |  Ctrl+Up/Down focus Activity Feed/Details"
	}
	if strings.TrimSpace(m.teamID) != "" {
		line += "  |  /team focus run  |  Ctrl+G clear focus"
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
	return lipgloss.NewStyle().MaxWidth(effectiveWidth).MaxHeight(m.height).Render(final)
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
		feedBody := m.activityList.View()
		if footer := m.renderPaginationFooter(m.activityPage, m.activityPageSize, m.activityTotalCount); footer != "" {
			feedBody = strings.TrimRight(feedBody, "\n") + "\n" + footer
		}
		feed := m.panelStyle(panelActivity).
			Width(grid.ActivityFeed.InnerWidth()).
			Height(grid.ActivityFeed.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Feed") + "\n" + feedBody)
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
			Render(m.styles.sectionTitle.Render("Outbox") + "\n" + m.outboxVP.View())
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
	if raw == "" {
		return nil
	}
	cmd, rest := splitMonitorCommand(raw)
	// Treat any non-command submission as a task goal.
	// A "command" is a known slash-command token (e.g. "/help", "/model").
	if cmd == "" || !strings.HasPrefix(cmd, "/") || !isExactMonitorCommand(cmd) {
		return m.enqueueTask(strings.TrimSpace(raw), 0)
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
	if cmd == "/artifact" {
		if strings.TrimSpace(rest) != "" {
			return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /artifact"}} }
		}
		return m.openArtifactViewer()
	}

	if cmd == "/team" {
		if strings.TrimSpace(m.teamID) == "" {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] /team is only available in team monitor"}}
			}
		}
		if strings.TrimSpace(rest) != "" {
			return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /team"}} }
		}
		return m.openTeamPicker()
	}

	if cmd == "/sessions" {
		if strings.TrimSpace(rest) != "" {
			return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /sessions"}} }
		}
		return m.openSessionPicker()
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
		if strings.TrimSpace(m.teamID) != "" {
			return m.writeTeamControl("set_team_model", model)
		}
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
		if strings.TrimSpace(m.teamID) != "" {
			if m.taskStore == nil {
				return commandLinesMsg{lines: []string{"[queued] error: task store not available"}}
			}
			now := time.Now().UTC()
			id := "task-" + uuid.NewString()
			teamSessionID := "team-" + strings.TrimSpace(m.teamID)
			teamRunID := "team-" + strings.TrimSpace(m.teamID) + "-monitor"
			task := types.Task{
				TaskID:    id,
				SessionID: teamSessionID,
				RunID:     teamRunID,
				Goal:      goal,
				Priority:  priority,
				Status:    types.TaskStatusPending,
				CreatedAt: &now,
				TeamID:    m.teamID,
				Inputs:    map[string]any{},
				Metadata: map[string]any{
					"source": "monitor",
				},
			}
			if err := m.taskStore.CreateTask(m.ctx, task); err != nil {
				return commandLinesMsg{lines: []string{"[queued] error: " + err.Error()}}
			}
			return tea.Batch(
				func() tea.Msg {
					return commandLinesMsg{lines: []string{"[queued] " + id + " " + goal + " — task queued to team " + m.teamID}}
				},
				func() tea.Msg { return taskQueuedLocallyMsg{TaskID: id, Goal: goal} },
				m.loadTeamStatus(),
			)
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
		if strings.TrimSpace(m.teamID) != "" {
			return commandLinesMsg{lines: []string{"[control] not supported in team mode"}}
		}
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

func (m *monitorModel) writeTeamControl(command, model string) tea.Cmd {
	return func() tea.Msg {
		teamID := strings.TrimSpace(m.teamID)
		if teamID == "" {
			return commandLinesMsg{lines: []string{"[control] error: team id is required"}}
		}
		command = strings.TrimSpace(command)
		model = strings.TrimSpace(model)
		if command == "" || model == "" {
			return commandLinesMsg{lines: []string{"[control] error: command and model are required"}}
		}
		controlDir := filepath.Join(fsutil.GetTeamDir(m.cfg.DataDir, teamID), "control")
		if err := os.MkdirAll(controlDir, 0o755); err != nil {
			return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
		}
		payload := map[string]any{
			"type":        "team_control",
			"command":     command,
			"model":       model,
			"requestedAt": time.Now().UTC().Format(time.RFC3339Nano),
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		controlPath := filepath.Join(controlDir, "set-model.json")
		if err := os.WriteFile(controlPath, b, 0o644); err != nil {
			return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
		}
		return tea.Batch(
			func() tea.Msg {
				return commandLinesMsg{lines: []string{"[control] queued team model change -> " + model}}
			},
			m.loadTeamManifestCmd(),
		)
	}
}

func (m *monitorModel) searchMemory(query string) tea.Cmd {
	return func() tea.Msg {
		query = strings.TrimSpace(query)
		if query == "" {
			return commandLinesMsg{lines: []string{"[memory] error: query is empty"}}
		}
		memDir := fsutil.GetMemoryDir(m.cfg.DataDir)
		res, err := resources.NewDailyMemoryResource(memDir)
		if err != nil {
			return commandLinesMsg{lines: []string{"[memory] error: " + err.Error()}}
		}
		results, err := res.Search(m.ctx, "", query, 5)
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
		if strings.TrimSpace(m.teamID) == "" || strings.TrimSpace(m.model) == "" || strings.EqualFold(strings.TrimSpace(m.model), "team") {
			m.model = v
		}
	}
	if v := strings.TrimSpace(ev.Data["profile"]); v != "" {
		if strings.TrimSpace(m.teamID) == "" || strings.TrimSpace(m.profile) == "" || strings.EqualFold(strings.TrimSpace(m.profile), "team") {
			m.profile = v
		}
	}
	m.observeTaskEvent(ev)
	m.observeAgentOutput(ev)
	switch ev.Type {
	case "agent.step":
		summary := strings.TrimSpace(ev.Data["reasoningSummary"])
		if summary != "" {
			m.appendThinkingEntry(strings.TrimSpace(ev.RunID), strings.TrimSpace(ev.Data["role"]), summary)
		}
	case "llm.usage.total":
		m.stats.lastTurnTokensIn = parseInt(ev.Data["input"])
		m.stats.lastTurnTokensOut = parseInt(ev.Data["output"])
		m.stats.lastTurnTokens = parseInt(ev.Data["total"])
	case "llm.cost.total":
		known := parseBool(ev.Data["known"])
		m.stats.lastTurnCostUSD = strings.TrimSpace(ev.Data["costUsd"])
		if !known && m.stats.lastTurnCostUSD == "" {
			m.stats.lastTurnCostUSD = "?"
		}
		m.stats.pricingKnown = known
	}
}

func (m *monitorModel) observeTaskEvent(ev types.EventRecord) {
	switch ev.Type {
	case "task.queued":
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
		if v := strings.TrimSpace(ev.Data["costUsd"]); v != "" {
			m.stats.lastTurnCostUSD = v
		}
	case "task.quarantined":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		// Best-effort: clear any active task view; outbox panel is loaded via pagination.
		m.currentTask = nil
	}
}

func (m *monitorModel) observeAgentOutput(ev types.EventRecord) {
	runID := strings.TrimSpace(ev.RunID)
	role := strings.TrimSpace(ev.Data["role"])
	rolePrefix := ""
	if role != "" {
		rolePrefix = "[" + role + "] "
	}
	switch ev.Type {
	case "daemon.start", "daemon.stop", "daemon.control", "daemon.warning", "daemon.error", "daemon.runner.error":
		m.appendAgentOutputForRun(formatEventLine(ev), runID)
	case "task.queued", "task.start", "task.done", "task.quarantined", "task.delivered", "task.heartbeat.enqueued", "task.heartbeat.skipped":
		for _, line := range formatTaskEventLines(ev) {
			m.appendAgentOutputForRun(line, runID)
		}
	case "control.check", "control.success", "control.error":
		m.appendAgentOutputForRun(formatEventLine(ev), runID)
	case "agent.error", "agent.turn.complete":
		m.appendAgentOutputForRun(formatEventLine(ev), runID)
	case "agent.op.request":
		if shouldHideInboxOp(ev.Data["op"], ev.Data["path"]) {
			return
		}
		txt := strings.TrimSpace(renderOpRequest(ev.Data))
		if txt == "" {
			txt = strings.TrimSpace(ev.Data["op"])
		}
		ts := ev.Timestamp.Local().Format("15:04:05")
		line := fmt.Sprintf("[%s] %sop: %s", ts, rolePrefix, txt)
		idx := m.appendAgentOutputLine(line, runID)
		if idx < 0 {
			return
		}
		entry := agentOutputPendingEntry{
			index:     idx,
			timestamp: ts,
			desc:      txt,
		}
		if opID := strings.TrimSpace(ev.Data["opId"]); opID != "" {
			if m.agentOutputPending == nil {
				m.agentOutputPending = map[string]agentOutputPendingEntry{}
			}
			m.agentOutputPending[opID] = entry
		} else {
			m.agentOutputPendingFallback = &agentOutputPendingEntry{
				index:     idx,
				timestamp: ts,
				desc:      txt,
			}
		}
	case "agent.op.response":
		if shouldHideInboxOp(ev.Data["op"], ev.Data["path"]) {
			return
		}
		opID := strings.TrimSpace(ev.Data["opId"])
		entry, ok := m.takeAgentOutputPending(opID)
		status := formatAgentOutputStatus(ev)
		if ok {
			line := fmt.Sprintf("[%s] %sop: %s — %s", entry.timestamp, rolePrefix, entry.desc, status)
			if entry.index >= 0 && entry.index < len(m.agentOutput) {
				m.agentOutput[entry.index] = line
				// The updated line may re-wrap, so invalidate cached layout metadata.
				m.agentOutputLayoutWidth = 0
				m.dirtyAgentOutput = true
				return
			}
		}
		// If the pending entry was dropped from the output buffer, fall back to appending a new line.
		ts := ev.Timestamp.Local().Format("15:04:05")
		txt := strings.TrimSpace(renderOpRequest(ev.Data))
		if txt == "" {
			txt = strings.TrimSpace(ev.Data["op"])
		}
		line := fmt.Sprintf("[%s] %sop: %s — %s", ts, rolePrefix, txt, status)
		m.appendAgentOutputForRun(line, runID)
	}
}

func (m *monitorModel) appendAgentOutput(line string) {
	m.appendAgentOutputForRun(line, "")
}

func (m *monitorModel) appendAgentOutputForRun(line, runID string) {
	_ = m.appendAgentOutputLine(line, runID)
}

func (m *monitorModel) appendAgentOutputLine(line, runID string) int {
	line = strings.TrimSpace(line)
	if line == "" {
		return -1
	}
	m.agentOutput = append(m.agentOutput, line)
	m.agentOutputRunID = append(m.agentOutputRunID, strings.TrimSpace(runID))
	m.trimAgentOutputBuffer()
	m.dirtyAgentOutput = true
	// Incrementally maintain layout metadata when possible (for virtualization).
	w := m.agentOutputVP.Width
	if w <= 0 {
		w = 80
	}
	// If width changed or metadata is out of sync, fall back to recompute on next refresh.
	if m.agentOutputLayoutWidth != w || len(m.agentOutputLineStarts) != len(m.agentOutput)-1 || len(m.agentOutputLineHeights) != len(m.agentOutput)-1 {
		m.agentOutputLayoutWidth = 0
		return len(m.agentOutput) - 1
	}
	start := m.agentOutputTotalLines
	h := 1
	wrapped := wordwrap.String(line, w)
	h = 1 + strings.Count(wrapped, "\n")
	m.agentOutputLineStarts = append(m.agentOutputLineStarts, start)
	m.agentOutputLineHeights = append(m.agentOutputLineHeights, h)
	m.agentOutputTotalLines += h
	return len(m.agentOutput) - 1
}

func (m *monitorModel) appendThinkingEntry(runID, role string, summary string) {
	if m == nil {
		return
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return
	}
	m.thinkingEntries = append(m.thinkingEntries, thinkingEntry{
		RunID:   strings.TrimSpace(runID),
		Role:    strings.TrimSpace(role),
		Summary: summary,
	})
	if maxThinkingEntries > 0 && len(m.thinkingEntries) > maxThinkingEntries {
		start := len(m.thinkingEntries) - maxThinkingEntries
		m.thinkingEntries = append([]thinkingEntry(nil), m.thinkingEntries[start:]...)
	}
	m.dirtyThinking = true
}

func (m *monitorModel) trimAgentOutputBuffer() {
	if m == nil {
		return
	}
	maxLines := agentOutputMaxLines
	if maxLines <= 0 {
		return
	}
	if len(m.agentOutput) <= maxLines {
		return
	}

	// Drop a chunk at a time to amortize copying costs once we hit the limit.
	drop := len(m.agentOutput) - (maxLines - agentOutputDropChunk)
	if agentOutputDropChunk <= 0 {
		drop = len(m.agentOutput) - maxLines
	}
	if drop <= 0 {
		drop = 1
	}
	if drop >= len(m.agentOutput) {
		m.agentOutput = nil
		m.agentOutputRunID = nil
		m.agentOutputFilteredCache = nil
		m.agentOutputPending = nil
		m.agentOutputPendingFallback = nil
		m.agentOutputLayoutWidth = 0
		m.agentOutputLogicalYOffset = 0
		m.agentOutputFollow = true
		return
	}

	removedLogicalLines := 0
	// appendAgentOutputLine() trims before it appends incremental layout metadata, so
	// lineHeights may be for the previous length (len(agentOutput)-1) or the current length.
	if m.agentOutputLayoutWidth != 0 && (len(m.agentOutputLineHeights) == len(m.agentOutput) || len(m.agentOutputLineHeights) == len(m.agentOutput)-1) && drop <= len(m.agentOutputLineHeights) {
		for i := 0; i < drop; i++ {
			removedLogicalLines += m.agentOutputLineHeights[i]
		}
	}

	// Re-slice into a fresh backing array so we don't retain references to the dropped prefix.
	kept := append([]string(nil), m.agentOutput[drop:]...)
	m.agentOutput = kept
	if drop >= len(m.agentOutputRunID) {
		m.agentOutputRunID = nil
	} else {
		m.agentOutputRunID = append([]string(nil), m.agentOutputRunID[drop:]...)
	}
	m.agentOutputFilteredCache = nil

	// Adjust pending op indices so responses still update the correct lines when possible.
	if m.agentOutputPending != nil {
		for opID, entry := range m.agentOutputPending {
			entry.index -= drop
			if entry.index < 0 {
				delete(m.agentOutputPending, opID)
				continue
			}
			m.agentOutputPending[opID] = entry
		}
		if len(m.agentOutputPending) == 0 {
			m.agentOutputPending = nil
		}
	}
	if m.agentOutputPendingFallback != nil {
		e := *m.agentOutputPendingFallback
		e.index -= drop
		if e.index < 0 {
			m.agentOutputPendingFallback = nil
		} else {
			m.agentOutputPendingFallback = &e
		}
	}

	// Preserve scroll position as best-effort by shifting the logical y-offset down by the number
	// of wrapped lines we removed (when known). Always clamp on refresh.
	if removedLogicalLines > 0 && m.agentOutputLogicalYOffset > 0 {
		m.agentOutputLogicalYOffset = max(0, m.agentOutputLogicalYOffset-removedLogicalLines)
	}

	// Invalidate layout metadata; it no longer matches after dropping a prefix.
	m.agentOutputLayoutWidth = 0
}

func (m *monitorModel) takeAgentOutputPending(opID string) (agentOutputPendingEntry, bool) {
	if opID != "" && len(strings.TrimSpace(opID)) > 0 {
		if m.agentOutputPending != nil {
			if entry, ok := m.agentOutputPending[opID]; ok {
				delete(m.agentOutputPending, opID)
				return entry, true
			}
		}
		return agentOutputPendingEntry{}, false
	}
	if m.agentOutputPendingFallback != nil {
		entry := *m.agentOutputPendingFallback
		m.agentOutputPendingFallback = nil
		return entry, true
	}
	return agentOutputPendingEntry{}, false
}

func formatAgentOutputStatus(ev types.EventRecord) string {
	parts := []string{}
	if strings.TrimSpace(ev.Data["ok"]) == "true" {
		parts = append(parts, "ok")
	} else {
		parts = append(parts, "failed")
	}
	if status := strings.TrimSpace(ev.Data["status"]); status != "" {
		parts = append(parts, "status="+status)
	}
	if errStr := strings.TrimSpace(ev.Data["err"]); errStr != "" {
		parts = append(parts, "error="+errStr)
	}
	return strings.Join(parts, " ")
}

func formatTaskEventLines(ev types.EventRecord) []string {
	ts := ev.Timestamp.Local().Format("15:04:05")
	switch ev.Type {
	case "task.done", "task.quarantined":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		goal := strings.TrimSpace(ev.Data["goal"])
		status := strings.TrimSpace(ev.Data["status"])
		role := strings.TrimSpace(ev.Data["role"])
		rolePrefix := ""
		if role != "" {
			rolePrefix = "[" + role + "] "
		}
		if status == "" && ev.Type == "task.quarantined" {
			status = "quarantined"
		}
		if status == "" {
			status = "done"
		}
		header := fmt.Sprintf("[%s] %s%s: %s %s", ts, rolePrefix, ev.Type, shortID(taskID), status)
		if goal != "" {
			header += " goal=" + strconv.Quote(goal)
		}
		lines := []string{header}

		if summary := strings.TrimSpace(ev.Data["summary"]); summary != "" {
			lines = append(lines, "  summary: "+summary)
		}
		if errStr := strings.TrimSpace(ev.Data["error"]); errStr != "" {
			lines = append(lines, "  error: "+errStr)
		}
		if p := strings.TrimSpace(ev.Data["artifact0"]); p != "" {
			lines = append(lines, "  summaryPath: "+p)
		}
		if p := strings.TrimSpace(ev.Data["poisonPath"]); p != "" {
			lines = append(lines, "  poison: "+p)
		}
		return lines
	default:
		return []string{formatEventLine(ev)}
	}
}

func (m *monitorModel) refreshActivityList() {
	prevIdx := m.activityList.Index()
	prevID := ""
	wasFollowingTail := m.activityFollowingTail
	if prevIdx >= 0 && prevIdx < len(m.activityPageItems) {
		prevID = m.activityPageItems[prevIdx].ID
	}

	items := make([]list.Item, 0, len(m.activityPageItems))
	for _, a := range m.activityPageItems {
		items = append(items, activityItem{act: a})
	}
	m.activityList.SetItems(items)
	if len(items) == 0 {
		m.activityFollowingTail = true
		return
	}

	selectIdx := min(max(prevIdx, 0), len(items)-1)
	if wasFollowingTail {
		selectIdx = len(items) - 1
	} else if strings.TrimSpace(prevID) != "" {
		for i := range m.activityPageItems {
			if m.activityPageItems[i].ID == prevID {
				selectIdx = i
				break
			}
		}
	}
	m.activityList.Select(selectIdx)

	maxPage := max(0, (m.activityTotalCount+m.activityPageSize-1)/max(1, m.activityPageSize)-1)
	m.activityFollowingTail = (m.activityPage >= maxPage) && (selectIdx == len(items)-1)
}

func (m *monitorModel) refreshActivityDetail(forceTop bool) {
	if m.renderer == nil {
		return
	}
	if len(m.activityPageItems) == 0 || m.activityList.Index() < 0 || m.activityList.Index() >= len(m.activityPageItems) {
		m.activityDetail.SetContent("")
		m.activityDetailAct = ""
		m.activityDetail.GotoTop()
		return
	}
	prevYOffset := m.activityDetail.YOffset
	w := imax(24, m.activityDetail.Width-4)
	header := "### Details\n\n"
	help := "_PgUp/PgDn scroll · use Activity to change selection_\n\n"
	act := m.activityPageItems[m.activityList.Index()]
	md := renderActivityDetailMarkdown(act, false, false)
	rendered := strings.TrimRight(m.renderer.RenderMarkdown(header+help+md, w), "\n")
	rendered = wrapViewportText(rendered, imax(10, m.activityDetail.Width))
	m.activityDetail.SetContent(rendered)
	if forceTop || m.activityDetailAct != act.ID {
		m.activityDetail.GotoTop()
	} else if prevYOffset > 0 {
		m.activityDetail.YOffset = prevYOffset
	}
	m.activityDetailAct = act.ID
}

func (m *monitorModel) refreshPlanView() {
	if m.renderer == nil {
		return
	}
	prevYOffset := m.planViewport.YOffset
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
	if m.planFollowingTop {
		m.planViewport.GotoTop()
	} else {
		m.planViewport.SetYOffset(prevYOffset)
	}
}

func (m *monitorModel) refreshThinkingViewport() {
	if len(m.thinkingEntries) == 0 {
		m.thinkingVP.SetContent(kit.StyleDim.Render("No thoughts captured yet."))
		m.thinkingAutoScroll = true
		m.thinkingVP.GotoTop()
		return
	}
	prevYOffset := m.thinkingVP.YOffset

	// Timeline view: colored nodes with a dimmed vertical spine.
	w := imax(10, m.thinkingVP.Width)
	const prefixW = 4 // "● " or "│ "
	contentW := imax(1, w-prefixW)

	// Styles for the timeline
	nodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a371f7")) // Purple node
	spineStyle := kit.StyleDim
	titleStyle := kit.StyleBold

	filtered := make([]thinkingEntry, 0, len(m.thinkingEntries))
	for _, e := range m.thinkingEntries {
		if strings.TrimSpace(m.focusedRunID) != "" {
			entryRunID := strings.TrimSpace(e.RunID)
			focusedRunID := strings.TrimSpace(m.focusedRunID)
			if entryRunID != focusedRunID {
				// Backward compatibility: some stored events may not have run IDs.
				if !(entryRunID == "" && strings.TrimSpace(m.focusedRunRole) != "" && strings.EqualFold(strings.TrimSpace(e.Role), strings.TrimSpace(m.focusedRunRole))) {
					continue
				}
			}
		}
		filtered = append(filtered, e)
	}

	out := make([]string, 0, len(filtered)*3)
	last := len(filtered) - 1

	for i, e := range filtered {
		summary := strings.TrimSpace(e.Summary)
		if summary == "" {
			continue
		}
		rolePrefix := ""
		if role := strings.TrimSpace(e.Role); role != "" {
			rolePrefix = "[" + role + "] "
		}

		// Render content with markdown
		body := rolePrefix + summary
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
	if m.thinkingAutoScroll {
		m.thinkingVP.GotoBottom()
	} else {
		m.thinkingVP.SetYOffset(prevYOffset)
	}
}

func (m *monitorModel) renderHeader() string {
	content := ""
	if strings.TrimSpace(m.teamID) != "" {
		content = lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.headerTitle.Render("Workbench TEAM "),
			kit.RenderTag(kit.TagOptions{Key: "Team", Value: m.teamID}),
			kit.RenderTag(kit.TagOptions{Key: "Tasks", Value: fmt.Sprintf("%d pending, %d active, %d done", m.teamPendingCount, m.teamActiveCount, m.teamDoneCount)}),
		)
		if strings.TrimSpace(m.focusedRunID) != "" {
			focusLabel := strings.TrimSpace(m.focusedRunRole)
			if focusLabel == "" {
				focusLabel = shortID(strings.TrimSpace(m.focusedRunID))
			}
			content = lipgloss.JoinHorizontal(lipgloss.Left, content, kit.RenderTag(kit.TagOptions{Key: "Focus", Value: focusLabel}))
		}
	} else {
		content = lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.headerTitle.Render("Workbench - Always On "),
			kit.RenderTag(kit.TagOptions{Key: "Agent", Value: m.runID}),
		)
	}
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
	return lipgloss.NewStyle().MaxWidth(grid.ScreenWidth).MaxHeight(grid.AgentOutput.Height).Render(row)
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
		feedBody := m.activityList.View()
		if footer := m.renderPaginationFooter(m.activityPage, m.activityPageSize, m.activityTotalCount); footer != "" {
			feedBody = strings.TrimRight(feedBody, "\n") + "\n" + footer
		}
		feed := m.panelStyle(panelActivity).
			Width(grid.ActivityFeed.InnerWidth()).
			Height(grid.ActivityFeed.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Feed") + "\n" + feedBody)
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
		sectionTitle := "Current Task"
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
		if strings.TrimSpace(m.teamID) != "" {
			sectionTitle = "Role Status"
			if len(m.teamRoles) == 0 {
				currentTaskBody = kit.StyleDim.Render("No role activity yet.")
			} else {
				lines := make([]string, 0, len(m.teamRoles))
				for _, role := range m.teamRoles {
					lines = append(lines, fmt.Sprintf("- %s: %s", strings.TrimSpace(role.Role), strings.TrimSpace(role.Info)))
				}
				currentTaskBody = strings.Join(lines, "\n")
			}
		}
		current := m.panelStyle(panelCurrentTask).
			Width(grid.CurrentTask.InnerWidth()).
			Height(grid.CurrentTask.InnerHeight()).
			Render(m.styles.sectionTitle.Render(sectionTitle) + "\n" + currentTaskBody)
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
		feedBody := m.activityList.View()
		if footer := m.renderPaginationFooter(m.activityPage, m.activityPageSize, m.activityTotalCount); footer != "" {
			feedBody = strings.TrimRight(feedBody, "\n") + "\n" + footer
		}
		feed := m.panelStyle(panelActivity).
			Width(grid.ActivityFeed.InnerWidth()).
			Height(grid.ActivityFeed.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Feed") + "\n" + feedBody)
		detail := m.panelStyle(panelActivityDetail).
			Width(grid.ActivityDetail.InnerWidth()).
			Height(grid.ActivityDetail.InnerHeight()).
			Render(m.styles.sectionTitle.Render("Activity Details") + "\n" + m.activityDetail.View())
		return lipgloss.JoinVertical(lipgloss.Left, feed, detail)
	}
}

func (m *monitorModel) renderOutbox(spec layoutmgr.PanelSpec) string {
	return m.panelStyle(panelOutbox).Width(spec.InnerWidth()).Height(spec.InnerHeight()).Render(
		m.styles.sectionTitle.Render("Outbox") + "\n" + m.outboxVP.View(),
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
		Value: modelID,
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
	if strings.TrimSpace(m.teamID) != "" {
		teamLabel := kit.RenderTag(kit.TagOptions{
			Key:   "team",
			Value: kit.TruncateMiddle(strings.TrimSpace(m.teamID), 16),
			Styles: kit.TagStyles{
				KeyStyle:   tagKeyStyle,
				ValueStyle: tagValueStyle,
			},
		})
		statusLeft = statusLeft + "  " + teamLabel
		if mc := m.teamModelChange; mc != nil && strings.EqualFold(strings.TrimSpace(mc.Status), "pending") {
			targetModel := strings.TrimSpace(mc.RequestedModel)
			if targetModel != "" {
				modelChangeLabel := kit.RenderTag(kit.TagOptions{
					Key:   "model-change",
					Value: "pending -> " + kit.TruncateMiddle(targetModel, 16),
					Styles: kit.TagStyles{
						KeyStyle:   tagKeyStyle,
						ValueStyle: tagValueStyle,
					},
				})
				statusLeft = statusLeft + "  " + modelChangeLabel
			}
		}
	}
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
			m.refreshActivityDetail(true)
		}
		if isScrollKey(msg) {
			maxPage := max(0, (m.activityTotalCount+m.activityPageSize-1)/max(1, m.activityPageSize)-1)
			m.activityFollowingTail = len(m.activityPageItems) > 0 && m.activityPage >= maxPage && m.activityList.Index() == len(m.activityPageItems)-1
		}
		return m, cmd
	case panelActivityDetail:
		var cmd tea.Cmd
		m.activityDetail, cmd = m.activityDetail.Update(msg)
		return m, cmd
	case panelPlan:
		var cmd tea.Cmd
		m.planViewport, cmd = m.planViewport.Update(msg)
		if isScrollKey(msg) {
			m.planFollowingTop = m.planViewport.YOffset <= 0
		}
		return m, cmd
	case panelCurrentTask:
		// Current task panel is static, no interactive model.
		return m, nil
	case panelOutput:
		if isScrollKey(msg) {
			m.applyAgentOutputScroll(msg)
			m.refreshAgentOutputViewport()
			m.agentOutputFollow = m.agentOutputAtBottom()
			return m, nil
		}
		// Non-scroll keys: nothing to do (viewport is read-only).
		return m, nil
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
		if isScrollKey(msg) {
			m.thinkingAutoScroll = m.thinkingAtBottom()
		}
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
	showWarning := m.runStatus != types.RunStatusRunning
	return manager.CalculateDashboard(m.width, m.height, composerHeight, statsHeight, statusBarH, showWarning)
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

	resizeVP := func(vp *viewport.Model, w, h int) bool {
		if vp == nil {
			return false
		}
		changed := vp.Width != w || vp.Height != h
		vp.Width = w
		vp.Height = h
		return changed
	}

	compact := m.isCompactMode()

	// Resize first (size changes can force re-wrap).
	if resizeVP(&m.agentOutputVP, imax(10, grid.AgentOutput.ContentWidth), imax(1, grid.AgentOutput.ContentHeight)) {
		m.dirtyAgentOutput = true
	}
	if resizeVP(&m.outboxVP, imax(10, grid.Outbox.ContentWidth), imax(1, grid.Outbox.ContentHeight)) {
		m.dirtyOutbox = true
	}
	if resizeVP(&m.activityDetail, imax(10, grid.ActivityDetail.ContentWidth), imax(1, grid.ActivityDetail.ContentHeight)) {
		m.dirtyActivity = true
	}
	if resizeVP(&m.planViewport, imax(10, grid.Plan.ContentWidth), imax(1, grid.Plan.ContentHeight)) {
		m.dirtyPlan = true
	}
	// Thoughts share the same spec as Plan/SidePanel in dashboard mode; in compact mode it's not visible.
	if resizeVP(&m.thinkingVP, m.planViewport.Width, m.planViewport.Height) {
		m.dirtyThinking = true
	}
	if resizeVP(&m.inboxVP, imax(10, grid.Inbox.ContentWidth), imax(1, grid.Inbox.ContentHeight)) {
		m.dirtyInbox = true
	}
	if resizeVP(&m.memoryVP, imax(10, grid.Memory.ContentWidth), imax(1, grid.Memory.ContentHeight)) {
		m.dirtyMemory = true
	}

	feedW := imax(10, grid.ActivityFeed.ContentWidth)
	feedH := imax(1, grid.ActivityFeed.ContentHeight)
	m.activityList.SetSize(feedW, feedH)

	m.input.SetWidth(imax(10, grid.Composer.ContentWidth))

	// Refresh only what is (or will be) visible, and only when dirty.
	outputVisible := !compact || m.compactTab == 0
	activityVisible := (compact && m.compactTab == 1) || (!compact && m.dashboardSideTab == 0)
	planVisible := (compact && m.compactTab == 2) || (!compact && m.dashboardSideTab == 1)
	outboxVisible := (compact && m.compactTab == 3) || (!compact && m.dashboardSideTab == 2)
	inboxVisible := !compact && m.dashboardSideTab == 2
	thinkingVisible := !compact && m.dashboardSideTab == 3
	memoryVisible := grid.Memory.Height > 0 || m.focusedPanel == panelMemory

	if outputVisible && (m.dirtyLayout || m.dirtyAgentOutput) {
		m.refreshAgentOutputViewport()
		m.dirtyAgentOutput = false
	}
	if activityVisible && (m.dirtyLayout || m.dirtyActivity) {
		m.refreshActivityList()
		m.refreshActivityDetail(false)
		m.dirtyActivity = false
	}
	if planVisible && (m.dirtyLayout || m.dirtyPlan) {
		m.refreshPlanView()
		m.dirtyPlan = false
	}
	if outboxVisible && (m.dirtyLayout || m.dirtyOutbox) {
		w := m.outboxVP.Width
		m.outboxVP.SetContent(wrapViewportText(m.outboxViewportContent(w), w))
		m.dirtyOutbox = false
	}
	if inboxVisible && (m.dirtyLayout || m.dirtyInbox) {
		w := m.inboxVP.Width
		m.inboxVP.SetContent(wrapViewportText(m.inboxViewportContent(w), w))
		m.dirtyInbox = false
	}
	if thinkingVisible && (m.dirtyLayout || m.dirtyThinking) {
		m.refreshThinkingViewport()
		m.dirtyThinking = false
	}
	if memoryVisible && (m.dirtyLayout || m.dirtyMemory) {
		m.memoryVP.SetContent(wrapViewportText(renderMemResults(m.memResults), m.memoryVP.Width))
		m.dirtyMemory = false
	}

	m.dirtyLayout = false
}

func (m *monitorModel) loadPlanFiles() {
	if strings.TrimSpace(m.teamID) != "" {
		if strings.TrimSpace(m.focusedRunID) != "" {
			checklist, checklistErr, details, detailsErr := readPlanFiles(m.cfg.DataDir, strings.TrimSpace(m.focusedRunID))
			m.planMarkdown, m.planLoadErr = checklist, checklistErr
			m.planDetails, m.planDetailsErr = details, detailsErr
			m.dirtyPlan = true
			return
		}
		checklist, checklistErr, details, detailsErr := readTeamPlanFiles(m.cfg.DataDir, m.teamRunIDs, m.teamRoleByRunID)
		m.planMarkdown, m.planLoadErr = checklist, checklistErr
		m.planDetails, m.planDetailsErr = details, detailsErr
		m.dirtyPlan = true
		return
	}
	checklist, checklistErr, details, detailsErr := readPlanFiles(m.cfg.DataDir, m.runID)
	m.planMarkdown, m.planLoadErr = checklist, checklistErr
	m.planDetails, m.planDetailsErr = details, detailsErr
	m.dirtyPlan = true
}

func (m *monitorModel) loadPlanFilesCmd() tea.Cmd {
	if m == nil {
		return nil
	}
	if strings.TrimSpace(m.teamID) != "" {
		dataDir := m.cfg.DataDir
		if strings.TrimSpace(m.focusedRunID) != "" {
			runID := strings.TrimSpace(m.focusedRunID)
			return func() tea.Msg {
				checklist, checklistErr, details, detailsErr := readPlanFiles(dataDir, runID)
				return planFilesLoadedMsg{
					checklist:    checklist,
					checklistErr: checklistErr,
					details:      details,
					detailsErr:   detailsErr,
				}
			}
		}
		runIDs := append([]string(nil), m.teamRunIDs...)
		roleByRun := map[string]string{}
		for k, v := range m.teamRoleByRunID {
			roleByRun[k] = v
		}
		return func() tea.Msg {
			checklist, checklistErr, details, detailsErr := readTeamPlanFiles(dataDir, runIDs, roleByRun)
			return planFilesLoadedMsg{
				checklist:    checklist,
				checklistErr: checklistErr,
				details:      details,
				detailsErr:   detailsErr,
			}
		}
	}
	dataDir := m.cfg.DataDir
	runID := m.runID
	return func() tea.Msg {
		checklist, checklistErr, details, detailsErr := readPlanFiles(dataDir, runID)
		return planFilesLoadedMsg{
			checklist:    checklist,
			checklistErr: checklistErr,
			details:      details,
			detailsErr:   detailsErr,
		}
	}
}

func readTeamPlanFiles(dataDir string, runIDs []string, roleByRun map[string]string) (checklist string, checklistErr string, details string, detailsErr string) {
	if len(runIDs) == 0 {
		return "No team plan files found yet.", "", "Waiting for team runs to publish plan files.", ""
	}
	sort.Strings(runIDs)
	checkParts := make([]string, 0, len(runIDs))
	detailParts := make([]string, 0, len(runIDs))
	errParts := []string{}
	for _, runID := range runIDs {
		role := strings.TrimSpace(roleByRun[runID])
		if role == "" {
			role = runID
		}
		check, checkErr, det, detErr := readPlanFiles(dataDir, runID)
		if checkErr != "" {
			errParts = append(errParts, "["+role+"] checklist: "+checkErr)
		}
		if detErr != "" {
			errParts = append(errParts, "["+role+"] details: "+detErr)
		}
		if strings.TrimSpace(check) != "" {
			checkParts = append(checkParts, "## "+role+"\n\n"+strings.TrimSpace(check))
		}
		if strings.TrimSpace(det) != "" {
			detailParts = append(detailParts, "## "+role+"\n\n"+strings.TrimSpace(det))
		}
	}
	if len(checkParts) == 0 {
		checklist = "No team checklist files found yet."
	} else {
		checklist = strings.Join(checkParts, "\n\n---\n\n")
	}
	if len(detailParts) == 0 {
		details = "No team plan detail files found yet."
	} else {
		details = strings.Join(detailParts, "\n\n---\n\n")
	}
	if len(errParts) != 0 {
		joined := strings.Join(errParts, " | ")
		checklistErr = joined
		detailsErr = joined
	}
	return checklist, checklistErr, details, detailsErr
}

func readPlanFiles(dataDir, runID string) (checklist string, checklistErr string, details string, detailsErr string) {
	runDir := fsutil.GetAgentDir(dataDir, runID)
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

	details, detailsErr = load("HEAD.md")
	checklist, checklistErr = load("CHECKLIST.md")
	return checklist, checklistErr, details, detailsErr
}

func loadTeamManifest(dataDir, teamID string) (*teamManifestFile, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, fmt.Errorf("teamID is required")
	}
	path := filepath.Join(fsutil.GetTeamDir(dataDir, teamID), "team.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest teamManifestFile
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func shouldReloadPlanOnEvent(ev types.EventRecord) bool {
	// Some runtimes may emit direct fs.* events; others wrap ops in agent.op.* with op/path in data.
	if isPlanEvent(ev.Type, ev.Data["path"]) {
		return true
	}
	switch ev.Type {
	case "agent.op.request", "agent.op.response":
		// Note: stored agent.op.response events may omit "path" depending on StoreData policy,
		// so also triggering off agent.op.request keeps the Plan panel in sync.
		return isPlanEvent(ev.Data["op"], ev.Data["path"])
	default:
		return false
	}
}

func isPlanEvent(kind string, path string) bool {
	k := strings.TrimSpace(strings.ToLower(kind))
	// Events can be emitted as:
	// - fs_* event types ("fs_write")
	// - agent.op.* events with "op" values like "Write"
	if k != "fs_write" && k != "fs_append" && k != "fs_edit" && k != "fs_patch" &&
		k != "write" && k != "append" && k != "edit" && k != "patch" {
		return false
	}
	p := strings.TrimSpace(path)
	if p == "" {
		return false
	}
	// Some emitters omit the leading slash.
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.EqualFold(p, "/plan/HEAD.md") || strings.EqualFold(p, "/plan/CHECKLIST.md")
}

func formatEventLine(e types.EventRecord) string {
	ts := e.Timestamp.Local().Format("15:04:05")
	role := strings.TrimSpace(e.Data["role"])
	rolePrefix := ""
	if role != "" {
		rolePrefix = "[" + role + "] "
	}
	line := fmt.Sprintf("[%s] %s%s: %s", ts, rolePrefix, e.Type, e.Message)
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
				if strings.HasPrefix(rest, "[") {
					if rb := strings.Index(rest, "]"); rb != -1 && len(rest) > rb+1 {
						rest = strings.TrimSpace(rest[rb+1:])
					}
				}
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
	case "agent.error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	case "task.done", "task.delivered":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950"))
	case "task.start", "task.queued":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#6bbcff"))
	case "control", "control.check", "control.success", "control.error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#d29922"))
	case "agent.turn.complete":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#3fb950"))
	case "daemon.start", "daemon.stop", "daemon.control", "daemon.warning":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#a371f7"))
	case "task.quarantined":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f"))
	default:
		return kit.StyleDim
	}
}

func (m *monitorModel) currentAgentOutputLines() []string {
	if m == nil {
		return nil
	}
	if strings.TrimSpace(m.teamID) == "" || strings.TrimSpace(m.focusedRunID) == "" {
		return m.agentOutput
	}
	targetRunID := strings.TrimSpace(m.focusedRunID)
	out := make([]string, 0, len(m.agentOutput))
	for i, line := range m.agentOutput {
		if i >= len(m.agentOutputRunID) {
			break
		}
		if strings.TrimSpace(m.agentOutputRunID[i]) != targetRunID {
			continue
		}
		out = append(out, line)
	}
	m.agentOutputFilteredCache = out
	return m.agentOutputFilteredCache
}

func (m *monitorModel) refreshAgentOutputViewport() {
	if m == nil {
		return
	}
	w := m.agentOutputVP.Width
	if w <= 0 {
		w = 80
	}
	h := m.agentOutputVP.Height
	if h <= 0 {
		h = 1
	}
	source := m.currentAgentOutputLines()

	m.ensureAgentOutputLayout(w)
	maxY := m.agentOutputMaxYOffset(h)
	if m.agentOutputFollow {
		m.agentOutputLogicalYOffset = maxY
	}
	if m.agentOutputLogicalYOffset < 0 {
		m.agentOutputLogicalYOffset = 0
	}
	if m.agentOutputLogicalYOffset > maxY {
		m.agentOutputLogicalYOffset = maxY
	}

	if len(source) == 0 {
		m.agentOutputWindowStartLine = 0
		m.agentOutputVP.SetContent(kit.StyleDim.Render("No output yet."))
		m.agentOutputVP.SetYOffset(0)
		return
	}

	visibleStart := m.agentOutputLogicalYOffset
	visibleEnd := visibleStart + h
	firstVisible := m.findAgentOutputItemAtLine(visibleStart)
	lastVisible := m.findAgentOutputItemAtLine(visibleEnd)
	firstVisible = max(0, firstVisible)
	lastVisible = min(len(source)-1, lastVisible)

	const bufferItems = 50
	firstRender := max(0, firstVisible-bufferItems)
	lastRender := min(len(source)-1, lastVisible+bufferItems)
	windowStartLine := 0
	if firstRender >= 0 && firstRender < len(m.agentOutputLineStarts) {
		windowStartLine = m.agentOutputLineStarts[firstRender]
	}
	m.agentOutputWindowStartLine = windowStartLine

	lines := make([]string, 0, (lastRender-firstRender+1)*2)
	for i := firstRender; i <= lastRender; i++ {
		rawLine := strings.TrimSpace(source[i])
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
	rel := m.agentOutputLogicalYOffset - windowStartLine
	if rel < 0 {
		rel = 0
	}
	m.agentOutputVP.SetYOffset(rel)
}

func (m *monitorModel) agentOutputAtBottom() bool {
	if m == nil {
		return true
	}
	h := m.agentOutputVP.Height
	if h <= 0 {
		h = 1
	}
	maxY := m.agentOutputMaxYOffset(h)
	return m.agentOutputLogicalYOffset >= maxY
}

func (m *monitorModel) thinkingAtBottom() bool {
	return m.thinkingVP.AtBottom()
}

func (m *monitorModel) ensureAgentOutputLayout(width int) {
	if m == nil {
		return
	}
	if width <= 0 {
		width = 80
	}
	source := m.currentAgentOutputLines()
	if m.agentOutputLayoutWidth == width && len(m.agentOutputLineStarts) == len(source) && len(m.agentOutputLineHeights) == len(source) {
		return
	}

	m.agentOutputLayoutWidth = width
	m.agentOutputLineStarts = make([]int, len(source))
	m.agentOutputLineHeights = make([]int, len(source))
	lineNo := 0
	for i, rawLine := range source {
		m.agentOutputLineStarts[i] = lineNo
		rawLine = strings.TrimSpace(rawLine)
		h := 1
		if rawLine != "" {
			wrapped := wordwrap.String(rawLine, width)
			h = 1 + strings.Count(wrapped, "\n")
		}
		m.agentOutputLineHeights[i] = h
		lineNo += h
	}
	m.agentOutputTotalLines = lineNo
}

func (m *monitorModel) agentOutputMaxYOffset(viewportHeight int) int {
	if m == nil {
		return 0
	}
	if viewportHeight <= 0 {
		viewportHeight = 1
	}
	return max(0, m.agentOutputTotalLines-viewportHeight)
}

func (m *monitorModel) findAgentOutputItemAtLine(lineNo int) int {
	if len(m.agentOutputLineStarts) == 0 {
		return 0
	}
	if lineNo <= 0 {
		return 0
	}
	// Largest start <= lineNo
	i := sort.Search(len(m.agentOutputLineStarts), func(i int) bool {
		return m.agentOutputLineStarts[i] > lineNo
	})
	if i <= 0 {
		return 0
	}
	if i >= len(m.agentOutputLineStarts) {
		return len(m.agentOutputLineStarts) - 1
	}
	return i - 1
}

func (m *monitorModel) applyAgentOutputScroll(msg tea.KeyMsg) {
	if m == nil {
		return
	}
	h := m.agentOutputVP.Height
	if h <= 0 {
		h = 1
	}
	maxY := m.agentOutputMaxYOffset(h)
	delta := 0
	abs := false
	absVal := 0

	switch msg.Type {
	case tea.KeyUp:
		delta = -1
	case tea.KeyDown:
		delta = 1
	case tea.KeyPgUp, tea.KeyCtrlB:
		delta = -h
	case tea.KeyPgDown, tea.KeyCtrlF:
		delta = h
	case tea.KeyCtrlU:
		delta = -max(1, h/2)
	case tea.KeyCtrlD:
		delta = max(1, h/2)
	case tea.KeyHome:
		abs = true
		absVal = 0
	case tea.KeyEnd:
		abs = true
		absVal = maxY
	case tea.KeyRunes:
		switch strings.TrimSpace(msg.String()) {
		case "k", "K":
			delta = -1
		case "j", "J":
			delta = 1
		}
	}

	if abs {
		m.agentOutputLogicalYOffset = absVal
	} else {
		m.agentOutputLogicalYOffset += delta
	}
	if m.agentOutputLogicalYOffset < 0 {
		m.agentOutputLogicalYOffset = 0
	}
	if m.agentOutputLogicalYOffset > maxY {
		m.agentOutputLogicalYOffset = maxY
	}
	m.agentOutputFollow = m.agentOutputLogicalYOffset >= maxY
}

func isScrollKey(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyUp, tea.KeyDown, tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd,
		tea.KeyCtrlU, tea.KeyCtrlD, tea.KeyCtrlB, tea.KeyCtrlF:
		return true
	case tea.KeyRunes:
		switch strings.TrimSpace(msg.String()) {
		case "j", "k", "J", "K":
			return true
		}
	}
	return false
}

func (m *monitorModel) renderPaginationFooter(page, pageSize, totalCount int) string {
	if totalCount <= 0 || pageSize <= 0 {
		return ""
	}

	totalPages := (totalCount + pageSize - 1) / pageSize
	if totalPages <= 1 {
		return ""
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	start := page*pageSize + 1
	end := start + pageSize - 1
	if end > totalCount {
		end = totalCount
	}

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Italic(true).
		Faint(true)
	return style.Render(fmt.Sprintf(
		"%d-%d of %d | Page %d/%d | n/→ next  p/← prev  g first  G last",
		start, end, totalCount, page+1, totalPages,
	))
}

func (m *monitorModel) inboxViewportContent(width int) string {
	body := renderInbox(m.inboxList)
	footer := m.renderPaginationFooter(m.inboxPage, m.inboxPageSize, m.inboxTotalCount)
	if footer != "" {
		body = strings.TrimRight(body, "\n") + "\n" + footer
	}
	return body
}

func (m *monitorModel) outboxViewportContent(width int) string {
	body := renderOutboxLines(m.outboxResults, m.renderer, width)
	footer := m.renderPaginationFooter(m.outboxPage, m.outboxPageSize, m.outboxTotalCount)
	if footer != "" {
		body = strings.TrimRight(body, "\n") + "\n" + footer
	}
	return body
}

func renderInbox(tasks []taskState) string {
	if len(tasks) == 0 {
		return kit.StyleDim.Render("No pending inbox tasks.")
	}
	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		id := strings.TrimSpace(task.TaskID)
		if id == "" {
			continue
		}
		goal := truncateText(task.Goal, 48)
		// Use bullet + bold ID for better visual hierarchy
		line := "• " + kit.StyleBold.Render(shortID(id))
		if role := strings.TrimSpace(task.AssignedRole); role != "" {
			line += " " + kit.StyleDim.Render("["+role+"]")
		}
		if strings.TrimSpace(task.Status) != "" && strings.TrimSpace(task.Status) != string(types.TaskStatusPending) {
			line += " " + kit.StyleDim.Render("["+strings.TrimSpace(task.Status)+"]")
		}
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

		// Header: bullet + bold ID + dim goal -> colored status (+ cost/tokens).
		metaParts := make([]string, 0, 2)
		if r.CostUSD > 0 {
			metaParts = append(metaParts, fmt.Sprintf("$%.4f", r.CostUSD))
		}
		if r.TotalTokens > 0 {
			metaParts = append(metaParts, fmt.Sprintf("%d tok", r.TotalTokens))
		}
		meta := ""
		if len(metaParts) != 0 {
			meta = " " + kit.StyleDim.Render("("+strings.Join(metaParts, " • ")+")")
		}
		header := "• " + kit.StyleBold.Render(shortID(r.TaskID)) + " " +
			kit.StyleDim.Render("\""+goal+"\"") + " → " + statusStr + meta
		if role := strings.TrimSpace(r.AssignedRole); role != "" {
			header = "• " + kit.StyleBold.Render(shortID(r.TaskID)) + " " + kit.StyleDim.Render("["+role+"] ") +
				kit.StyleDim.Render("\""+goal+"\"") + " → " + statusStr + meta
		}
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
		if r.TotalTokens > 0 || r.CostUSD > 0 {
			parts := make([]string, 0, 2)
			if r.TotalTokens > 0 {
				parts = append(parts, fmt.Sprintf("tokens: %d (%d in + %d out)", r.TotalTokens, r.InputTokens, r.OutputTokens))
			}
			if r.CostUSD > 0 {
				parts = append(parts, fmt.Sprintf("cost: $%.4f", r.CostUSD))
			}
			if len(parts) != 0 {
				lines = append(lines, "  └ "+strings.Join(parts, " • "))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func renderStats(s monitorStats) string {
	uptime := ""
	if !s.started.IsZero() {
		uptime = time.Since(s.started).Round(time.Second).String()
	}
	lastTokensLine := ""
	if s.lastTurnTokens > 0 {
		lastTokensLine = fmt.Sprintf("\nLast tokens: %d (%d in + %d out)", s.lastTurnTokens, s.lastTurnTokensIn, s.lastTurnTokensOut)
	}
	totalTokensLine := fmt.Sprintf("\nTotal tokens: %d (%d in + %d out)", s.totalTokens, s.totalTokensIn, s.totalTokensOut)
	costLine := ""
	if strings.TrimSpace(s.lastTurnCostUSD) != "" {
		costLine = fmt.Sprintf("\nLast cost: $%s", s.lastTurnCostUSD)
	}
	totalLine := "\nTotal cost: Unknown"
	if s.totalCostUSD > 0 {
		totalLine = fmt.Sprintf("\nTotal cost: $%.4f", s.totalCostUSD)
	} else if s.totalTokens == 0 {
		totalLine = "\nTotal cost: $0.0000"
	}
	pricingState := "unknown"
	if s.pricingKnown || s.totalTokens == 0 || s.totalCostUSD > 0 {
		pricingState = "known"
	}
	return fmt.Sprintf("Tasks done: %d\nUptime: %s%s%s%s%s\nPricing: %s", s.tasksDone, fallback(uptime, "unknown"), lastTokensLine, totalTokensLine, costLine, totalLine, pricingState)
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
