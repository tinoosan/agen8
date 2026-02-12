package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	layoutmgr "github.com/tinoosan/workbench-core/internal/tui/layout"
	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/cost"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/protocol"
	"github.com/tinoosan/workbench-core/pkg/resources"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
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

type monitorSwitchRunMsg struct {
	RunID string
}

type monitorSwitchTeamMsg struct {
	TeamID string
}

type monitorReloadedMsg struct {
	model *monitorModel
	err   error
}

type agentsListMsg struct {
	agents []protocol.AgentListItem
	err    error
}

type tickMsg struct {
	now time.Time
}

type uiRefreshMsg struct{}

type planReloadMsg struct{}

type sessionTotalsReloadMsg struct{}

type rpcHealthMsg struct {
	reachable bool
	err       error
	manual    bool
}

type planFilesLoadedMsg struct {
	checklist    string
	checklistErr string
	details      string
	detailsErr   string
}

type sessionTotalsLoadedMsg struct {
	session      types.Session
	pricingKnown bool
	tasksDone    int
	err          error
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
	SwitchToRunID  string
	SwitchToTeamID string
}

type monitorModel struct {
	ctx            context.Context
	cfg            config.Config
	runID          string
	teamID         string
	detached       bool
	runStatus      string // loaded at init; used to show "run not active" warning
	rpcEndpoint    string
	rpcHealthKnown bool
	rpcReachable   bool
	rpcLastErr     string
	rpcChecking    bool
	result         *MonitorResult
	session        pkgstore.SessionQuery
	sessionID      string

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
	reasoningUsageByStep         map[string]int
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
	reasoningEffort              string
	reasoningSummary             string
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

	// Agent picker
	agentPickerOpen bool
	agentPickerList list.Model
	agentPickerErr  string

	// Model picker
	modelPickerOpen         bool
	modelPickerList         list.Model
	modelPickerProvider     string
	modelPickerQuery        string
	modelPickerProviderView bool

	// Profile picker
	profilePickerOpen     bool
	profilePickerList     list.Model
	profilePickerMode     string
	profilePickerTeamOnly bool

	// Team picker
	teamPickerOpen bool
	teamPickerList list.Model

	// New-session wizard
	newSessionWizardOpen bool
	newSessionWizardList list.Model

	// Command palette (inline autocomplete above composer)
	commandPaletteOpen     bool
	commandPaletteMatches  []string
	commandPaletteSelected int

	// Artifact viewer (full-screen takeover)
	artifactViewerOpen      bool
	artifactTasks           []types.Task
	artifactTree            []artifactTreeNode
	artifactAllTree         []artifactTreeNode
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
	artifactSearchScopeKey  string
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

	lastLLMErrorClass     string
	lastLLMErrorRetryable bool
	lastLLMErrorSet       bool
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

var gluedReasoningSectionRE = regexp.MustCompile(`([.!?])[ \t]*([*_]{0,2})([A-Z])`)

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
	if err := ensureRPCReachable(ctx); err != nil {
		return err
	}
	var result MonitorResult
	m, err := newMonitorModel(ctx, cfg, runID, &result)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err == nil && strings.TrimSpace(result.SwitchToRunID) != "" {
		return &MonitorSwitchRunError{RunID: strings.TrimSpace(result.SwitchToRunID)}
	}
	if err == nil && strings.TrimSpace(result.SwitchToTeamID) != "" {
		return &MonitorSwitchTeamError{TeamID: strings.TrimSpace(result.SwitchToTeamID)}
	}
	return err
}

func RunMonitorDetached(ctx context.Context, cfg config.Config) error {
	var result MonitorResult
	m, err := newDetachedMonitorModel(ctx, cfg, &result)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err == nil && strings.TrimSpace(result.SwitchToRunID) != "" {
		return &MonitorSwitchRunError{RunID: strings.TrimSpace(result.SwitchToRunID)}
	}
	if err == nil && strings.TrimSpace(result.SwitchToTeamID) != "" {
		return &MonitorSwitchTeamError{TeamID: strings.TrimSpace(result.SwitchToTeamID)}
	}
	return err
}

func RunTeamMonitor(ctx context.Context, cfg config.Config, teamID string) error {
	if err := ensureRPCReachable(ctx); err != nil {
		return err
	}
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
	if err == nil && strings.TrimSpace(result.SwitchToRunID) != "" {
		return &MonitorSwitchRunError{RunID: strings.TrimSpace(result.SwitchToRunID)}
	}
	if err == nil && strings.TrimSpace(result.SwitchToTeamID) != "" {
		return &MonitorSwitchTeamError{TeamID: strings.TrimSpace(result.SwitchToTeamID)}
	}
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
	sessionActiveModel := ""
	sessionReasoningEffort := ""
	sessionReasoningSummary := ""
	runProfile := ""
	if r, err := store.LoadRun(cfg, runID); err == nil {
		runStatus = r.Status
		runSessionID = strings.TrimSpace(r.SessionID)
		if r.Runtime != nil {
			runProfile = strings.TrimSpace(r.Runtime.Profile)
		}
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
			if active := strings.TrimSpace(sess.ActiveModel); active != "" {
				sessionActiveModel = active
			}
			sessionReasoningEffort = strings.TrimSpace(sess.ReasoningEffort)
			sessionReasoningSummary = strings.TrimSpace(sess.ReasoningSummary)
		}
	}

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		runID:                       runID,
		rpcEndpoint:                 monitorRPCEndpoint(),
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
		reasoningUsageByStep:        map[string]int{},
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
	if sessionActiveModel != "" {
		m.model = sessionActiveModel
	}
	if sessionReasoningEffort != "" {
		m.reasoningEffort = sessionReasoningEffort
	}
	if sessionReasoningSummary != "" {
		m.reasoningSummary = sessionReasoningSummary
	}
	if runProfile != "" {
		m.profile = runProfile
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
	if sessionActiveModel != "" {
		m.model = sessionActiveModel
	}
	if sessionReasoningEffort != "" {
		m.reasoningEffort = sessionReasoningEffort
	}
	if sessionReasoningSummary != "" {
		m.reasoningSummary = sessionReasoningSummary
	}
	if runProfile != "" {
		m.profile = runProfile
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
	sessionStore, _ := store.NewSQLiteSessionStore(cfg)
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
	teamRoleByRun := map[string]string{}
	teamRunIDs := []string{}
	teamCoordinatorRunID := ""
	teamCoordinatorRole := ""
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
		rpcEndpoint:                 monitorRPCEndpoint(),
		runStatus:                   types.RunStatusRunning,
		result:                      result,
		session:                     sessionStore,
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
		reasoningUsageByStep:        map[string]int{},
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
	if manifest, err := loadTeamManifestFromDisk(cfg, teamID); err == nil && manifest != nil {
		if profileID := strings.TrimSpace(manifest.ProfileID); profileID != "" {
			m.profile = profileID
		}
		if mc := manifest.ModelChange; mc != nil {
			if requested := strings.TrimSpace(mc.RequestedModel); requested != "" &&
				(strings.EqualFold(strings.TrimSpace(mc.Status), "pending") ||
					strings.EqualFold(strings.TrimSpace(mc.Status), "applied")) {
				m.model = requested
			}
		}
		if strings.TrimSpace(m.model) == "" {
			if teamModel := strings.TrimSpace(manifest.TeamModel); teamModel != "" {
				m.model = teamModel
			}
		}
		m.teamModelChange = manifest.ModelChange
		m.teamCoordinatorRole = strings.TrimSpace(manifest.CoordinatorRole)
		m.teamCoordinatorRunID = strings.TrimSpace(manifest.CoordinatorRun)
		roleByRun := map[string]string{}
		runIDs := make([]string, 0, len(manifest.Roles))
		for _, role := range manifest.Roles {
			runID := strings.TrimSpace(role.RunID)
			if runID == "" {
				continue
			}
			roleByRun[runID] = strings.TrimSpace(role.RoleName)
			runIDs = append(runIDs, runID)
			if strings.TrimSpace(role.SessionID) != "" && runID == m.teamCoordinatorRunID && strings.TrimSpace(m.sessionID) == "" {
				m.sessionID = strings.TrimSpace(role.SessionID)
			}
			if strings.TrimSpace(m.sessionID) == "" && strings.TrimSpace(role.SessionID) != "" {
				m.sessionID = strings.TrimSpace(role.SessionID)
			}
		}
		if len(runIDs) != 0 {
			m.teamRunIDs = runIDs
			m.teamRoleByRunID = roleByRun
		}
	}
	if m.session != nil && strings.TrimSpace(m.sessionID) != "" {
		if sess, err := m.session.LoadSession(ctx, strings.TrimSpace(m.sessionID)); err == nil {
			if v := strings.TrimSpace(sess.ReasoningEffort); v != "" {
				m.reasoningEffort = v
			}
			if v := strings.TrimSpace(sess.ReasoningSummary); v != "" {
				m.reasoningSummary = v
			}
		}
	}
	if m.profile == "" {
		m.profile = "default"
	}
	if m.model == "" {
		m.model = "default"
	}
	m.refreshViewports()
	return m, nil
}

func loadTeamManifestFromDisk(cfg config.Config, teamID string) (*teamManifestFile, error) {
	path := filepath.Join(fsutil.GetTeamDir(cfg.DataDir, strings.TrimSpace(teamID)), "team.json")
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

func newDetachedMonitorModel(ctx context.Context, cfg config.Config, result *MonitorResult) (*monitorModel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
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

	sessionStore, err := store.NewSQLiteSessionStore(cfg)
	if err != nil {
		return nil, err
	}

	m := &monitorModel{
		ctx:                         ctx,
		cfg:                         cfg,
		rpcEndpoint:                 monitorRPCEndpoint(),
		runStatus:                   types.RunStatusRunning,
		result:                      result,
		session:                     sessionStore,
		sessionID:                   "",
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
		reasoningUsageByStep:        map[string]int{},
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
		teamRoleByRunID:             map[string]string{},
		teamEventCursor:             map[string]int64{},
		detached:                    true,
	}
	m.activityDetail.MouseWheelEnabled = false
	m.planViewport.MouseWheelEnabled = false
	m.agentOutputVP.MouseWheelEnabled = false
	m.inboxVP.MouseWheelEnabled = false
	m.outboxVP.MouseWheelEnabled = false
	m.memoryVP.MouseWheelEnabled = false
	m.thinkingVP.MouseWheelEnabled = false
	m.artifactContentVP.MouseWheelEnabled = false
	m.appendAgentOutput("[system] No active context. Use /new, /sessions, or /agents.")
	m.refreshViewports()
	return m, nil
}

// loadPendingTasksFromSQLite queries pending tasks for the run. Used so the queue
// shows tasks added before the monitor started or via webhook, without scanning
// inbox files.
func (m *monitorModel) Init() tea.Cmd {
	if m.isDetached() {
		m.rpcChecking = true
		return tea.Batch(m.tick(), m.checkRPCHealthCmd(false))
	}
	cmds := []tea.Cmd{m.listenEvent(), m.listenErr(), m.tick(), m.loadInboxPage(), m.loadOutboxPage(), m.loadActivityPage()}
	if strings.TrimSpace(m.teamID) != "" {
		cmds = append(cmds, m.loadTeamStatus(), m.loadTeamEvents(), m.loadPlanFilesCmd(), m.loadTeamManifestCmd())
	}
	return tea.Batch(cmds...)
}

func (m *monitorModel) isDetached() bool {
	return m != nil && m.detached
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
		if m.isDetached() {
			cmds := []tea.Cmd{m.tick()}
			if !m.rpcChecking {
				m.rpcChecking = true
				cmds = append(cmds, m.checkRPCHealthCmd(false))
			}
			return m, tea.Batch(cmds...)
		}
		if strings.TrimSpace(m.teamID) != "" {
			return m, tea.Batch(m.tick(), m.loadInboxPage(), m.loadOutboxPage(), m.loadActivityPage(), m.loadTeamStatus(), m.loadTeamEvents(), m.loadPlanFilesCmd(), m.loadTeamManifestCmd())
		}
		return m, m.tick()

	case rpcHealthMsg:
		m.rpcChecking = false
		m.rpcHealthKnown = true
		m.rpcReachable = msg.reachable
		if msg.err != nil {
			m.rpcLastErr = msg.err.Error()
		} else {
			m.rpcLastErr = ""
		}
		if msg.manual {
			if msg.reachable {
				m.appendAgentOutput("[system] Daemon RPC connected at " + strings.TrimSpace(m.rpcEndpoint))
			} else {
				m.appendAgentOutput("[system] Daemon RPC disconnected: " + strings.TrimSpace(m.rpcLastErr) + " (retry with /reconnect)")
			}
		}
		return m, m.scheduleUIRefresh()

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
		m.appendAgentOutput(fmt.Sprintf("[%s] task.queued %s %s", time.Now().Local().Format("15:04:05"), shortID(msg.TaskID), truncateText(strings.TrimSpace(msg.Goal), 80)))
		m.dirtyInbox = true
		return m, tea.Batch(m.loadInboxPage(), m.scheduleUIRefresh())

	case monitorSwitchRunMsg:
		runID := strings.TrimSpace(msg.RunID)
		if runID == "" {
			return m, m.scheduleUIRefresh()
		}
		return m, m.reloadAsRun(runID)

	case monitorSwitchTeamMsg:
		teamID := strings.TrimSpace(msg.TeamID)
		if teamID == "" {
			return m, m.scheduleUIRefresh()
		}
		return m, m.reloadAsTeam(teamID)

	case monitorReloadedMsg:
		if msg.err != nil {
			m.appendAgentOutput("[switch] error: " + msg.err.Error())
			return m, m.scheduleUIRefresh()
		}
		if msg.model == nil {
			return m, m.scheduleUIRefresh()
		}
		return msg.model, tea.Batch(msg.model.Init(), msg.model.scheduleUIRefresh())

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
		if modelChange := manifest.ModelChange; modelChange != nil {
			requested := strings.TrimSpace(modelChange.RequestedModel)
			status := strings.ToLower(strings.TrimSpace(modelChange.Status))
			if requested != "" && (status == "pending" || status == "applied") {
				m.model = requested
			} else if teamModel := strings.TrimSpace(manifest.TeamModel); teamModel != "" {
				m.model = teamModel
			}
		} else if teamModel := strings.TrimSpace(manifest.TeamModel); teamModel != "" {
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
		items := msg.items
		if len(items) == 0 {
			items = sessionsToPickerItems(msg.sessions)
		}
		m.sessionPickerList.SetItems(items)
		if strings.TrimSpace(m.sessionPickerFilter) == "" {
			m.sessionPickerList.SetFilterText("")
			m.sessionPickerList.SetFilterState(list.Unfiltered)
		} else {
			m.sessionPickerList.SetFilterText(strings.TrimSpace(m.sessionPickerFilter))
			m.sessionPickerList.SetFilterState(list.Filtering)
		}
		if len(items) > 0 {
			m.sessionPickerList.Select(0)
		}
		return m, m.scheduleUIRefresh()

	case agentsListMsg:
		if msg.err != nil {
			m.agentPickerErr = msg.err.Error()
			m.agentPickerList.SetItems(nil)
			return m, m.scheduleUIRefresh()
		}
		m.agentPickerErr = ""
		items := agentsToPickerItems(msg.agents)
		m.agentPickerList.SetItems(items)
		if len(items) > 0 {
			m.agentPickerList.Select(0)
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
			m.stats.pricingKnown = msg.pricingKnown
			if msg.tasksDone > 0 {
				m.stats.tasksDone = msg.tasksDone
			}
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
		if m.newSessionWizardOpen {
			return m.updateNewSessionWizard(msg)
		}
		if m.agentPickerOpen {
			return m.updateAgentPicker(msg)
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

func (m *monitorModel) rpcRun() types.Run {
	if m.isDetached() {
		return types.Run{
			RunID:     "detached-control",
			SessionID: "detached-control",
		}
	}
	runID := strings.TrimSpace(m.runID)
	if strings.TrimSpace(m.teamID) != "" {
		if r := strings.TrimSpace(m.teamCoordinatorRunID); r != "" {
			runID = r
		}
	}
	sessionID := strings.TrimSpace(m.sessionID)
	if sessionID == "" {
		if strings.TrimSpace(m.teamID) != "" {
			sessionID = "team-" + strings.TrimSpace(m.teamID)
		} else {
			sessionID = "sess-" + strings.TrimSpace(m.runID)
		}
	}
	return types.Run{RunID: runID, SessionID: sessionID}
}

func (m *monitorModel) rpcRoundTrip(method string, params any, out any) error {
	if m == nil {
		return fmt.Errorf("monitor is nil")
	}
	baseCtx := m.ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
	defer cancel()
	cli := protocol.TCPClient{
		Endpoint: strings.TrimSpace(m.rpcEndpoint),
		Timeout:  2 * time.Second,
	}
	if err := cli.Call(ctx, method, params, out); err != nil {
		return fmt.Errorf("rpc %s: %w", method, err)
	}
	return nil
}

func monitorRPCEndpoint() string {
	v := strings.TrimSpace(os.Getenv("WORKBENCH_RPC_ENDPOINT"))
	if v != "" {
		return v
	}
	return protocol.DefaultRPCEndpoint
}

func ensureRPCReachable(ctx context.Context) error {
	return pingRPCEndpoint(ctx, monitorRPCEndpoint())
}

func (m *monitorModel) checkRPCHealthCmd(manual bool) tea.Cmd {
	endpoint := strings.TrimSpace(m.rpcEndpoint)
	if endpoint == "" {
		endpoint = monitorRPCEndpoint()
	}
	return func() tea.Msg {
		baseCtx := m.ctx
		if baseCtx == nil {
			baseCtx = context.Background()
		}
		ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
		defer cancel()
		if err := pingRPCEndpoint(ctx, endpoint); err != nil {
			return rpcHealthMsg{reachable: false, err: err, manual: manual}
		}
		return rpcHealthMsg{reachable: true, manual: manual}
	}
}

func pingRPCEndpoint(ctx context.Context, endpoint string) error {
	p := protocol.SessionListParams{
		ThreadID: protocol.ThreadID("detached-control"),
		Limit:    1,
		Offset:   0,
	}
	var out protocol.SessionListResult
	cli := protocol.TCPClient{
		Endpoint: strings.TrimSpace(endpoint),
		Timeout:  2 * time.Second,
	}
	if err := cli.Call(ctx, protocol.MethodSessionList, p, &out); err != nil {
		return fmt.Errorf("daemon RPC unavailable at %s: %w", strings.TrimSpace(endpoint), err)
	}
	return nil
}

func (m *monitorModel) rpcControlSetModel(ctx context.Context, threadID, target, model string) ([]string, error) {
	if strings.TrimSpace(threadID) != strings.TrimSpace(m.rpcRun().SessionID) {
		return nil, fmt.Errorf("thread not found")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	target = strings.TrimSpace(target)

	if strings.TrimSpace(m.teamID) != "" {
		return m.queueTeamModelChange(model, target, "rpc.control.setModel")
	}

	runID := strings.TrimSpace(m.runID)
	if target != "" && target != runID && target != strings.TrimSpace(m.sessionID) {
		return nil, fmt.Errorf("target does not match active run")
	}
	ss, err := store.NewSQLiteSessionStore(m.cfg)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(m.sessionID) != "" {
		if sess, serr := ss.LoadSession(ctx, strings.TrimSpace(m.sessionID)); serr == nil {
			sess.ActiveModel = model
			_ = ss.SaveSession(ctx, sess)
		}
	}
	return []string{runID}, nil
}

func (m *monitorModel) rpcControlSetProfile(_ context.Context, threadID, target, profileRef string) ([]string, error) {
	if strings.TrimSpace(threadID) != strings.TrimSpace(m.rpcRun().SessionID) {
		return nil, fmt.Errorf("thread not found")
	}
	profileRef = strings.TrimSpace(profileRef)
	if profileRef == "" {
		return nil, fmt.Errorf("profile is required")
	}
	target = strings.TrimSpace(target)
	if strings.TrimSpace(m.teamID) != "" {
		return nil, fmt.Errorf("profile switching is not supported in team mode")
	}
	runID := strings.TrimSpace(m.runID)
	if target != "" && target != runID && target != strings.TrimSpace(m.sessionID) {
		return nil, fmt.Errorf("target does not match active run")
	}
	return []string{runID}, nil
}

func (m *monitorModel) loadSessionTotalsCmd() tea.Cmd {
	if m == nil || m.isDetached() {
		return nil
	}
	return func() tea.Msg {
		params := protocol.SessionGetTotalsParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
			if strings.TrimSpace(m.focusedRunID) != "" {
				params.RunID = strings.TrimSpace(m.focusedRunID)
			}
		} else {
			params.RunID = strings.TrimSpace(m.runID)
		}
		var res protocol.SessionGetTotalsResult
		if err := m.rpcRoundTrip(protocol.MethodSessionGetTotals, params, &res); err != nil {
			return sessionTotalsLoadedMsg{err: err}
		}
		now := time.Now().UTC()
		return sessionTotalsLoadedMsg{session: types.Session{
			SessionID:    strings.TrimSpace(m.rpcRun().SessionID),
			CreatedAt:    &now,
			InputTokens:  res.TotalTokensIn,
			OutputTokens: res.TotalTokensOut,
			TotalTokens:  res.TotalTokens,
			CostUSD:      res.TotalCostUSD,
		}, pricingKnown: res.PricingKnown, tasksDone: res.TasksDone}
	}
}

func (m *monitorModel) loadInboxPage() tea.Cmd {
	if m == nil || m.isDetached() {
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
		fetch := func(targetPage int) (protocol.TaskListResult, error) {
			params := protocol.TaskListParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				View:     "inbox",
				Limit:    pageSize,
				Offset:   targetPage * pageSize,
			}
			if strings.TrimSpace(m.teamID) != "" {
				params.TeamID = strings.TrimSpace(m.teamID)
				if strings.TrimSpace(m.focusedRunID) != "" {
					params.RunID = strings.TrimSpace(m.focusedRunID)
				}
			} else {
				params.RunID = strings.TrimSpace(m.runID)
			}
			var res protocol.TaskListResult
			if err := m.rpcRoundTrip(protocol.MethodTaskList, params, &res); err != nil {
				return protocol.TaskListResult{}, err
			}
			return res, nil
		}

		res, err := fetch(page)
		if err != nil {
			return inboxLoadedMsg{tasks: prevTasks, totalCount: prevTotal, page: prevPage}
		}
		total := res.TotalCount
		if total <= 0 {
			return inboxLoadedMsg{tasks: []taskState{}, totalCount: 0, page: 0}
		}
		requestedPage := page
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
		if page != requestedPage {
			res, err = fetch(page)
			if err != nil {
				return inboxLoadedMsg{tasks: prevTasks, totalCount: prevTotal, page: prevPage}
			}
			if res.TotalCount > 0 {
				total = res.TotalCount
			}
		}

		out := make([]taskState, 0, len(res.Tasks))
		for _, t := range res.Tasks {
			status := strings.TrimSpace(t.Status)
			if status != string(types.TaskStatusPending) && status != string(types.TaskStatusActive) {
				continue
			}
			ts := taskState{
				TaskID:       strings.TrimSpace(t.ID),
				AssignedRole: strings.TrimSpace(t.AssignedRole),
				Goal:         strings.TrimSpace(t.Goal),
				Status:       status,
			}
			if !t.CreatedAt.IsZero() {
				ts.StartedAt = t.CreatedAt
			}
			if ts.TaskID != "" {
				out = append(out, ts)
			}
		}
		return inboxLoadedMsg{tasks: out, totalCount: total, page: page}
	}
}

func (m *monitorModel) loadOutboxPage() tea.Cmd {
	if m == nil || m.isDetached() {
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
		fetch := func(targetPage int) (protocol.TaskListResult, error) {
			params := protocol.TaskListParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				View:     "outbox",
				Limit:    pageSize,
				Offset:   targetPage * pageSize,
			}
			if strings.TrimSpace(m.teamID) != "" {
				params.TeamID = strings.TrimSpace(m.teamID)
				if strings.TrimSpace(m.focusedRunID) != "" {
					params.RunID = strings.TrimSpace(m.focusedRunID)
				}
			} else {
				params.RunID = strings.TrimSpace(m.runID)
			}
			var res protocol.TaskListResult
			if err := m.rpcRoundTrip(protocol.MethodTaskList, params, &res); err != nil {
				return protocol.TaskListResult{}, err
			}
			return res, nil
		}

		res, err := fetch(page)
		if err != nil {
			return outboxLoadedMsg{entries: prevEntries, totalCount: prevTotal, page: prevPage}
		}
		total := res.TotalCount
		if total <= 0 {
			return outboxLoadedMsg{entries: []outboxEntry{}, totalCount: 0, page: 0}
		}
		requestedPage := page
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
		if page != requestedPage {
			res, err = fetch(page)
			if err != nil {
				return outboxLoadedMsg{entries: prevEntries, totalCount: prevTotal, page: prevPage}
			}
			if res.TotalCount > 0 {
				total = res.TotalCount
			}
		}

		out := make([]outboxEntry, 0, len(res.Tasks))
		for _, t := range res.Tasks {
			status := strings.TrimSpace(t.Status)
			if status != string(types.TaskStatusSucceeded) && status != string(types.TaskStatusFailed) && status != string(types.TaskStatusCanceled) {
				continue
			}
			out = append(out, outboxEntry{
				TaskID:       strings.TrimSpace(t.ID),
				RunID:        strings.TrimSpace(string(t.RunID)),
				AssignedRole: strings.TrimSpace(t.AssignedRole),
				Goal:         strings.TrimSpace(t.Goal),
				Status:       status,
				Summary:      strings.TrimSpace(t.Summary),
				Error:        strings.TrimSpace(t.Error),
				InputTokens:  t.InputTokens,
				OutputTokens: t.OutputTokens,
				TotalTokens:  t.TotalTokens,
				CostUSD:      t.CostUSD,
				Timestamp:    t.CompletedAt,
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
		return filter
	}
	filter.TeamID = ""
	filter.RunID = strings.TrimSpace(m.runID)
	return filter
}

func (m *monitorModel) loadTeamStatus() tea.Cmd {
	if m == nil || m.taskStore == nil || strings.TrimSpace(m.teamID) == "" {
		return nil
	}
	return func() tea.Msg {
		var res protocol.TeamGetStatusResult
		if err := m.rpcRoundTrip(protocol.MethodTeamGetStatus, protocol.TeamGetStatusParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			TeamID:   strings.TrimSpace(m.teamID),
		}, &res); err != nil {
			return teamStatusLoadedMsg{}
		}
		roles := make([]teamRoleState, 0, len(res.Roles))
		for _, r := range res.Roles {
			roles = append(roles, teamRoleState{Role: strings.TrimSpace(r.Role), Info: strings.TrimSpace(r.Info)})
		}
		return teamStatusLoadedMsg{
			pending:      res.Pending,
			active:       res.Active,
			done:         res.Done,
			roles:        roles,
			runIDs:       append([]string(nil), res.RunIDs...),
			roleByRunID:  res.RoleByRunID,
			totalTokens:  res.TotalTokens,
			totalCostUSD: res.TotalCostUSD,
			pricingKnown: res.PricingKnown,
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
	return func() tea.Msg {
		var res protocol.TeamGetManifestResult
		err := m.rpcRoundTrip(protocol.MethodTeamGetManifest, protocol.TeamGetManifestParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			TeamID:   strings.TrimSpace(m.teamID),
		}, &res)
		if err != nil {
			if manifest, ferr := loadTeamManifestFromDisk(m.cfg, m.teamID); ferr == nil && manifest != nil {
				return teamManifestLoadedMsg{manifest: manifest, err: nil}
			}
			return teamManifestLoadedMsg{manifest: nil, err: err}
		}
		manifest := &teamManifestFile{
			TeamID:          res.TeamID,
			ProfileID:       res.ProfileID,
			TeamModel:       res.TeamModel,
			CoordinatorRole: res.CoordinatorRole,
			CoordinatorRun:  res.CoordinatorRun,
			CreatedAt:       res.CreatedAt,
		}
		if res.ModelChange != nil {
			manifest.ModelChange = &teamModelChangeFile{
				RequestedModel: res.ModelChange.RequestedModel,
				Status:         res.ModelChange.Status,
				RequestedAt:    res.ModelChange.RequestedAt,
				AppliedAt:      res.ModelChange.AppliedAt,
				Reason:         res.ModelChange.Reason,
				Error:          res.ModelChange.Error,
			}
		}
		roles := make([]teamManifestRole, 0, len(res.Roles))
		for _, r := range res.Roles {
			roles = append(roles, teamManifestRole{RoleName: r.RoleName, RunID: r.RunID, SessionID: r.SessionID})
		}
		manifest.Roles = roles
		return teamManifestLoadedMsg{manifest: manifest, err: nil}
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
	if m == nil || m.isDetached() {
		return nil
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
		fetch := func(targetPage int) (protocol.ActivityListResult, error) {
			params := protocol.ActivityListParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				Limit:    pageSize,
				Offset:   targetPage * pageSize,
				SortDesc: false,
			}
			if strings.TrimSpace(m.teamID) != "" {
				params.TeamID = strings.TrimSpace(m.teamID)
				if strings.TrimSpace(m.focusedRunID) != "" {
					params.RunID = strings.TrimSpace(m.focusedRunID)
				}
			} else {
				params.RunID = strings.TrimSpace(m.runID)
			}
			var res protocol.ActivityListResult
			if err := m.rpcRoundTrip(protocol.MethodActivityList, params, &res); err != nil {
				return protocol.ActivityListResult{}, err
			}
			return res, nil
		}

		res, err := fetch(page)
		if err != nil {
			return activityLoadedMsg{activities: prevActivities, totalCount: prevTotal, page: prevPage}
		}
		total := res.TotalCount
		if total <= 0 {
			return activityLoadedMsg{activities: []Activity{}, totalCount: 0, page: 0}
		}
		requestedPage := page
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
		if page != requestedPage {
			res, err = fetch(page)
			if err != nil {
				return activityLoadedMsg{activities: prevActivities, totalCount: prevTotal, page: prevPage}
			}
			if res.TotalCount > 0 {
				total = res.TotalCount
			}
		}
		out := make([]Activity, 0, len(res.Activities))
		for _, a := range res.Activities {
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
	if m.newSessionWizardOpen {
		return m.renderNewSessionWizard(base)
	}
	if m.agentPickerOpen {
		return m.renderAgentPicker(base)
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
	if m.isDetached() {
		w := m.width
		if w <= 0 {
			w = 80
		}
		warningText := "No active context. Use /new, /sessions, or /agents."
		if m.rpcHealthKnown && !m.rpcReachable {
			warningText = "Daemon disconnected at " + strings.TrimSpace(m.rpcEndpoint) + ". Start `workbench daemon` and retry with /reconnect."
		}
		warning := m.styles.header.Copy().MaxWidth(w).Render(kit.StyleDim.Render(warningText))
		sections = []string{headerLine, warning, "", main, "", composer, stats, statusBar}
	} else if m.runStatus != types.RunStatusRunning {
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
	if m.rpcHealthKnown && !m.rpcReachable {
		line += "  |  daemon disconnected (use /reconnect)"
	}
	if m.stats.lastLLMErrorSet {
		retryState := "no-retry"
		if m.stats.lastLLMErrorRetryable {
			retryState = "retryable"
		}
		line += "  |  LLM error: " + fallback(strings.TrimSpace(m.stats.lastLLMErrorClass), "unknown") + " (" + retryState + ")"
	}
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
	sections := []string{headerLine}
	if m.rpcHealthKnown && !m.rpcReachable {
		sections = append(sections, kit.StyleDim.Render("Daemon disconnected. Start `workbench daemon` and run /reconnect."))
	}
	sections = append(sections, tabBar, content, m.renderComposer(grid.Composer))
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
	if strings.HasPrefix(cmd, "/") && !isExactMonitorCommand(cmd) {
		return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] unknown command: " + cmd}} }
	}
	// Treat non-slash submissions as task goals.
	if cmd == "" || !strings.HasPrefix(cmd, "/") {
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
	if cmd == "/reconnect" {
		m.rpcChecking = true
		return m.checkRPCHealthCmd(true)
	}

	if cmd == "/editor" {
		return m.openComposeEditor("")
	}
	if cmd == "/new" {
		if strings.TrimSpace(rest) == "" {
			return m.openNewSessionWizard()
		}
		req := parseNewSessionRequest(strings.TrimSpace(rest), strings.TrimSpace(m.profile))
		switch req.Mode {
		case "team":
			if strings.TrimSpace(req.Profile) == "" {
				return m.openProfilePickerFor("new-team", true)
			}
			return m.startNewTeamSession(req.Profile, req.Goal)
		case "standalone":
			return m.startNewStandaloneSession(req.Profile, req.Goal)
		default:
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] usage: /new [standalone [profile]] [goal] | /new team <profile> [goal]"}}
			}
		}
	}
	if cmd == "/rename-session" {
		title := strings.TrimSpace(rest)
		if title == "" {
			return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /rename-session <title>"}} }
		}
		return func() tea.Msg {
			var res protocol.SessionRenameResult
			if err := m.rpcRoundTrip(protocol.MethodSessionRename, protocol.SessionRenameParams{
				ThreadID:  protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				SessionID: strings.TrimSpace(m.sessionID),
				Title:     title,
			}, &res); err != nil {
				return commandLinesMsg{lines: []string{"[session] error: " + err.Error()}}
			}
			return commandLinesMsg{lines: []string{"[session] renamed: " + strings.TrimSpace(res.Title)}}
		}
	}
	if cmd == "/artifact" {
		if strings.TrimSpace(rest) != "" {
			return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /artifact"}} }
		}
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
			}
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
	if cmd == "/agents" {
		if strings.TrimSpace(rest) != "" {
			return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /agents"}} }
		}
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] select or create a session first: /new or /sessions"}}
			}
		}
		return m.openAgentPicker()
	}

	if cmd == "/pause" {
		if strings.TrimSpace(rest) != "" {
			return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /pause"}} }
		}
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
			}
		}
		return func() tea.Msg {
			threadID := protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID))
			if strings.TrimSpace(m.teamID) != "" {
				var res protocol.SessionPauseResult
				if err := m.rpcRoundTrip(protocol.MethodSessionPause, protocol.SessionPauseParams{
					ThreadID:  threadID,
					SessionID: strings.TrimSpace(m.sessionID),
				}, &res); err != nil {
					return commandLinesMsg{lines: []string{"[pause] error: " + err.Error()}}
				}
				return commandLinesMsg{lines: []string{fmt.Sprintf("[pause] session paused (%d runs)", len(res.AffectedRunIDs))}}
			}
			var res protocol.AgentPauseResult
			if err := m.rpcRoundTrip(protocol.MethodAgentPause, protocol.AgentPauseParams{
				ThreadID: threadID,
				RunID:    strings.TrimSpace(m.runID),
			}, &res); err != nil {
				return commandLinesMsg{lines: []string{"[pause] error: " + err.Error()}}
			}
			return commandLinesMsg{lines: []string{"[pause] run paused: " + shortID(strings.TrimSpace(res.RunID))}}
		}
	}
	if cmd == "/resume" {
		if strings.TrimSpace(rest) != "" {
			return func() tea.Msg { return commandLinesMsg{lines: []string{"[command] usage: /resume"}} }
		}
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
			}
		}
		return func() tea.Msg {
			threadID := protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID))
			if strings.TrimSpace(m.teamID) != "" {
				var res protocol.SessionResumeResult
				if err := m.rpcRoundTrip(protocol.MethodSessionResume, protocol.SessionResumeParams{
					ThreadID:  threadID,
					SessionID: strings.TrimSpace(m.sessionID),
				}, &res); err != nil {
					return commandLinesMsg{lines: []string{"[resume] error: " + err.Error()}}
				}
				return commandLinesMsg{lines: []string{fmt.Sprintf("[resume] session resumed (%d runs)", len(res.AffectedRunIDs))}}
			}
			var res protocol.AgentResumeResult
			if err := m.rpcRoundTrip(protocol.MethodAgentResume, protocol.AgentResumeParams{
				ThreadID: threadID,
				RunID:    strings.TrimSpace(m.runID),
			}, &res); err != nil {
				return commandLinesMsg{lines: []string{"[resume] error: " + err.Error()}}
			}
			return commandLinesMsg{lines: []string{"[resume] run resumed: " + shortID(strings.TrimSpace(res.RunID))}}
		}
	}

	// /model with no arg opens picker, with arg sets directly
	if cmd == "/model" && strings.TrimSpace(rest) == "" {
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
			}
		}
		return m.openModelPicker()
	}
	if cmd == "/model" && strings.TrimSpace(rest) != "" {
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
			}
		}
		model := strings.TrimSpace(rest)
		if strings.TrimSpace(m.teamID) != "" {
			return m.writeTeamControl("set_team_model", model)
		}
		return m.writeControl("set_model", map[string]any{"model": model})
	}

	// Reasoning commands
	if cmd == "/reasoning-effort" {
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
			}
		}
		m.openReasoningEffortPicker()
		return nil
	}
	if cmd == "/reasoning-summary" {
		if m.isDetached() {
			return func() tea.Msg {
				return commandLinesMsg{lines: []string{"[command] no active context; use /new or /sessions first"}}
			}
		}
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

type newSessionRequest struct {
	Mode    string
	Profile string
	Goal    string
}

func parseNewSessionRequest(rest, defaultProfile string) newSessionRequest {
	rest = strings.TrimSpace(rest)
	defaultProfile = strings.TrimSpace(defaultProfile)
	if defaultProfile == "" {
		defaultProfile = "general"
	}
	if rest == "" {
		return newSessionRequest{Mode: "standalone", Profile: defaultProfile}
	}
	toks := strings.Fields(rest)
	if len(toks) == 0 {
		return newSessionRequest{Mode: "standalone", Profile: defaultProfile}
	}
	mode := strings.ToLower(strings.TrimSpace(toks[0]))
	switch mode {
	case "team":
		if len(toks) == 1 {
			return newSessionRequest{Mode: "team"}
		}
		return newSessionRequest{
			Mode:    "team",
			Profile: strings.TrimSpace(toks[1]),
			Goal:    strings.TrimSpace(strings.Join(toks[2:], " ")),
		}
	case "standalone":
		if len(toks) == 1 {
			return newSessionRequest{Mode: "standalone", Profile: defaultProfile}
		}
		return newSessionRequest{
			Mode:    "standalone",
			Profile: strings.TrimSpace(toks[1]),
			Goal:    strings.TrimSpace(strings.Join(toks[2:], " ")),
		}
	default:
		return newSessionRequest{
			Mode:    "standalone",
			Profile: defaultProfile,
			Goal:    strings.TrimSpace(rest),
		}
	}
}

func (m *monitorModel) startNewStandaloneSession(profileRef, goal string) tea.Cmd {
	return func() tea.Msg {
		var res protocol.SessionStartResult
		if err := m.rpcRoundTrip(protocol.MethodSessionStart, protocol.SessionStartParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Mode:     "standalone",
			Profile:  strings.TrimSpace(profileRef),
			Goal:     strings.TrimSpace(goal),
			Model:    strings.TrimSpace(m.model),
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[session] error: " + err.Error()}}
		}
		runID := strings.TrimSpace(res.PrimaryRunID)
		if runID == "" {
			return commandLinesMsg{lines: []string{"[session] error: session.start returned empty primaryRunId"}}
		}
		return monitorSwitchRunMsg{RunID: runID}
	}
}

func (m *monitorModel) startNewTeamSession(profileRef, goal string) tea.Cmd {
	return func() tea.Msg {
		var res protocol.SessionStartResult
		if err := m.rpcRoundTrip(protocol.MethodSessionStart, protocol.SessionStartParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Mode:     "team",
			Profile:  strings.TrimSpace(profileRef),
			Goal:     strings.TrimSpace(goal),
			Model:    strings.TrimSpace(m.model),
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[session] error: " + err.Error()}}
		}
		teamID := strings.TrimSpace(res.TeamID)
		if teamID == "" {
			return commandLinesMsg{lines: []string{"[session] error: session.start(team) returned empty teamId"}}
		}
		return monitorSwitchTeamMsg{TeamID: teamID}
	}
}

func (m *monitorModel) reloadAsRun(runID string) tea.Cmd {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	return func() tea.Msg {
		if m.cancel != nil {
			m.cancel()
		}
		nm, err := newMonitorModel(m.ctx, m.cfg, runID, m.result)
		if err != nil {
			return monitorReloadedMsg{err: err}
		}
		nm.width = m.width
		nm.height = m.height
		nm.refreshViewports()
		return monitorReloadedMsg{model: nm}
	}
}

func (m *monitorModel) reloadAsTeam(teamID string) tea.Cmd {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil
	}
	return func() tea.Msg {
		if m.cancel != nil {
			m.cancel()
		}
		nm, err := newTeamMonitorModel(m.ctx, m.cfg, teamID, m.result)
		if err != nil {
			return monitorReloadedMsg{err: err}
		}
		nm.width = m.width
		nm.height = m.height
		nm.refreshViewports()
		return monitorReloadedMsg{model: nm}
	}
}

func (m *monitorModel) enqueueTask(goal string, priority int) tea.Cmd {
	return func() tea.Msg {
		if m.isDetached() {
			return commandLinesMsg{lines: []string{"[queued] error: no active context; use /new or /sessions first"}}
		}
		goal = strings.TrimSpace(goal)
		if goal == "" {
			return commandLinesMsg{lines: []string{"[queued] error: goal is empty"}}
		}
		params := protocol.TaskCreateParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Goal:     goal,
			Priority: priority,
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
			if strings.TrimSpace(m.focusedRunID) != "" {
				params.RunID = strings.TrimSpace(m.focusedRunID)
				params.AssignedToType = "agent"
				params.AssignedTo = strings.TrimSpace(m.focusedRunID)
				if strings.TrimSpace(m.focusedRunRole) != "" {
					params.AssignedRole = strings.TrimSpace(m.focusedRunRole)
				}
			} else {
				params.AssignedToType = "team"
				params.AssignedTo = strings.TrimSpace(m.teamID)
			}
		} else {
			params.RunID = strings.TrimSpace(m.runID)
			params.AssignedToType = "agent"
			params.AssignedTo = strings.TrimSpace(m.runID)
		}
		var res protocol.TaskCreateResult
		if err := m.rpcRoundTrip(protocol.MethodTaskCreate, params, &res); err != nil {
			return commandLinesMsg{lines: []string{"[queued] error: " + err.Error()}}
		}
		id := strings.TrimSpace(res.Task.ID)
		if id == "" {
			id = "task-" + uuid.NewString()
		}
		suffix := "run " + m.runID
		extra := []tea.Cmd{}
		if strings.TrimSpace(m.teamID) != "" {
			suffix = "team " + m.teamID
			extra = append(extra, m.loadTeamStatus())
		}
		cmds := []tea.Cmd{
			func() tea.Msg {
				return commandLinesMsg{lines: []string{"[queued] " + id + " " + goal + " — task queued to " + suffix}}
			},
			func() tea.Msg { return taskQueuedLocallyMsg{TaskID: id, Goal: goal} },
		}
		cmds = append(cmds, extra...)
		return tea.Batch(
			cmds...,
		)
	}
}

func (m *monitorModel) writeControl(command string, args map[string]any) tea.Cmd {
	return func() tea.Msg {
		switch strings.ToLower(strings.TrimSpace(command)) {
		case "set_model":
			model := ""
			if args != nil {
				if v, ok := args["model"].(string); ok {
					model = strings.TrimSpace(v)
				}
			}
			if model == "" {
				return commandLinesMsg{lines: []string{"[control] error: model is required"}}
			}
			var res protocol.ControlSetModelResult
			if err := m.rpcRoundTrip(protocol.MethodControlSetModel, protocol.ControlSetModelParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				Model:    model,
			}, &res); err != nil {
				return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
			}
			m.model = model
			return commandLinesMsg{lines: []string{"[control] applied set_model -> " + model}}
		case "set_reasoning":
			effort := ""
			summary := ""
			if args != nil {
				if v, ok := args["effort"].(string); ok {
					effort = strings.ToLower(strings.TrimSpace(v))
				}
				if v, ok := args["summary"].(string); ok {
					summary = strings.ToLower(strings.TrimSpace(v))
				}
			}
			if summary == "none" {
				summary = "off"
			}
			if effort == "" && summary == "" {
				return commandLinesMsg{lines: []string{"[control] error: effort or summary is required"}}
			}
			var res protocol.ControlSetReasoningResult
			if err := m.rpcRoundTrip(protocol.MethodControlSetReasoning, protocol.ControlSetReasoningParams{
				ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
				Effort:   effort,
				Summary:  summary,
			}, &res); err != nil {
				return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
			}
			parts := make([]string, 0, 2)
			if v := strings.TrimSpace(res.Effort); v != "" {
				parts = append(parts, "effort="+v)
			} else if effort != "" {
				parts = append(parts, "effort="+effort)
			}
			if v := strings.TrimSpace(res.Summary); v != "" {
				parts = append(parts, "summary="+v)
			} else if summary != "" {
				parts = append(parts, "summary="+summary)
			}
			if len(parts) == 0 {
				parts = append(parts, "updated")
			}
			return commandLinesMsg{lines: []string{"[control] applied set_reasoning -> " + strings.Join(parts, ", ")}}
		default:
			return commandLinesMsg{lines: []string{"[control] error: unsupported command " + command}}
		}
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
		var res protocol.ControlSetModelResult
		if err := m.rpcRoundTrip(protocol.MethodControlSetModel, protocol.ControlSetModelParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
			Model:    model,
		}, &res); err != nil {
			return commandLinesMsg{lines: []string{"[control] error: " + err.Error()}}
		}
		m.model = model
		return tea.Batch(
			func() tea.Msg {
				return commandLinesMsg{lines: []string{"[control] queued team model change -> " + model}}
			},
			m.loadTeamManifestCmd(),
		)
	}
}

func (m *monitorModel) queueTeamModelChange(model, target, reason string) ([]string, error) {
	teamID := strings.TrimSpace(m.teamID)
	if teamID == "" {
		return nil, fmt.Errorf("team id is required")
	}
	manifestPath := filepath.Join(fsutil.GetTeamDir(m.cfg.DataDir, teamID), "team.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest teamManifestFile
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	manifest.ModelChange = &teamModelChangeFile{
		RequestedModel: strings.TrimSpace(model),
		Status:         "pending",
		RequestedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Reason:         strings.TrimSpace(reason),
	}
	b, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(manifestPath, b, 0o644); err != nil {
		return nil, err
	}
	if strings.TrimSpace(target) != "" {
		return []string{strings.TrimSpace(target)}, nil
	}
	return []string{}, nil
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
	if ev.Type == "control.success" || ev.Type == "control.check" || ev.Type == "control.error" {
		if strings.EqualFold(strings.TrimSpace(ev.Data["command"]), "set_reasoning") {
			if v := strings.TrimSpace(ev.Data["effort"]); v != "" {
				m.reasoningEffort = strings.ToLower(v)
			}
			if v := strings.TrimSpace(ev.Data["summary"]); v != "" {
				m.reasoningSummary = strings.ToLower(v)
			}
		}
	}
	if v := strings.TrimSpace(ev.Data["effectiveModel"]); v != "" {
		m.model = v
	} else if v := strings.TrimSpace(ev.Data["model"]); v != "" {
		if strings.TrimSpace(m.teamID) != "" {
			if strings.TrimSpace(m.model) == "" || strings.EqualFold(strings.TrimSpace(m.model), "team") {
				m.model = v
			}
		} else if strings.TrimSpace(m.model) == "" || ev.Type == "control.success" {
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
		step := strings.TrimSpace(ev.Data["step"])
		key := reasoningStepKey(strings.TrimSpace(ev.RunID), strings.TrimSpace(ev.Data["role"]), step)
		summary := strings.TrimSpace(ev.Data["reasoningSummary"])
		if summary != "" {
			m.appendThinkingEntry(strings.TrimSpace(ev.RunID), strings.TrimSpace(ev.Data["role"]), summary)
			delete(m.reasoningUsageByStep, key)
		} else if n := m.reasoningUsageByStep[key]; n > 0 {
			m.appendThinkingEntry(strings.TrimSpace(ev.RunID), strings.TrimSpace(ev.Data["role"]),
				fmt.Sprintf("Reasoning used (%d tokens); provider did not return a reasoning summary.", n))
			delete(m.reasoningUsageByStep, key)
		}
		m.stats.lastLLMErrorSet = false
		m.stats.lastLLMErrorClass = ""
	case "llm.usage.total":
		m.stats.lastTurnTokensIn = parseInt(ev.Data["input"])
		m.stats.lastTurnTokensOut = parseInt(ev.Data["output"])
		m.stats.lastTurnTokens = parseInt(ev.Data["total"])
		reasoning := parseInt(ev.Data["reasoning"])
		if reasoning > 0 {
			step := strings.TrimSpace(ev.Data["step"])
			key := reasoningStepKey(strings.TrimSpace(ev.RunID), strings.TrimSpace(ev.Data["role"]), step)
			if m.reasoningUsageByStep == nil {
				m.reasoningUsageByStep = map[string]int{}
			}
			m.reasoningUsageByStep[key] = reasoning
		}
	case "llm.cost.total":
		known := parseBool(ev.Data["known"])
		m.stats.lastTurnCostUSD = getCostUSD(ev.Data)
		if !known && m.stats.lastTurnCostUSD == "" {
			m.stats.lastTurnCostUSD = "?"
		}
		m.stats.pricingKnown = known
	case "llm.error":
		m.stats.lastLLMErrorClass = fallback(strings.TrimSpace(ev.Data["class"]), "unknown")
		m.stats.lastLLMErrorRetryable = parseBool(ev.Data["retryable"])
		m.stats.lastLLMErrorSet = true
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
		m.inbox[taskID] = ts
		m.currentTask = &ts
	case "task.done":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.currentTask = nil
		m.stats.tasksDone++
		if v := getCostUSD(ev.Data); v != "" {
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
	case "llm.error", "llm.retry":
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
	summary = normalizeThinkingSummary(summary)
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

func normalizeThinkingSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	// Some providers emit adjacent reasoning sections without a separator, e.g.
	// "...in planning.Listing capabilities". Split those into distinct blocks.
	summary = gluedReasoningSectionRE.ReplaceAllString(summary, "$1\n\n$2$3")
	summary = strings.ReplaceAll(summary, "\r\n", "\n")
	for strings.Contains(summary, "\n\n\n") {
		summary = strings.ReplaceAll(summary, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(summary)
}

func reasoningStepKey(runID, role, step string) string {
	return strings.TrimSpace(runID) + "|" + strings.TrimSpace(role) + "|" + strings.TrimSpace(step)
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
	if m.isDetached() {
		content = lipgloss.JoinHorizontal(lipgloss.Left,
			m.styles.headerTitle.Render("Workbench Control Shell "),
			kit.RenderTag(kit.TagOptions{Key: "Status", Value: "detached"}),
		)
	} else if strings.TrimSpace(m.teamID) != "" {
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
		Value: profileRef,
		Styles: kit.TagStyles{
			KeyStyle:   tagKeyStyle,
			ValueStyle: tagValueStyle,
		},
	})

	segments := []string{modelLabel}
	if cost.SupportsReasoningSummary(modelID) {
		effort := strings.TrimSpace(m.reasoningEffort)
		if effort == "" {
			effort = "medium"
		}
		summary := strings.TrimSpace(m.reasoningSummary)
		if summary == "" {
			summary = "auto"
		}
		reasoningEffortLabel := kit.RenderTag(kit.TagOptions{
			Key:   "reasoning-effort",
			Value: effort,
			Styles: kit.TagStyles{
				KeyStyle:   tagKeyStyle,
				ValueStyle: tagValueStyle,
			},
		})
		reasoningSummaryLabel := kit.RenderTag(kit.TagOptions{
			Key:   "reasoning-summary",
			Value: summary,
			Styles: kit.TagStyles{
				KeyStyle:   tagKeyStyle,
				ValueStyle: tagValueStyle,
			},
		})
		// Keep summary/effort high-priority so they remain visible in narrow widths.
		segments = append(segments, reasoningSummaryLabel, reasoningEffortLabel)
	}
	segments = append(segments, profileLabel)
	if strings.TrimSpace(m.teamID) != "" {
		teamLabel := kit.RenderTag(kit.TagOptions{
			Key:   "team",
			Value: kit.TruncateMiddle(strings.TrimSpace(m.teamID), 16),
			Styles: kit.TagStyles{
				KeyStyle:   tagKeyStyle,
				ValueStyle: tagValueStyle,
			},
		})
		segments = append(segments, teamLabel)
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
				segments = append(segments, modelChangeLabel)
			}
		}
	}
	statusLeft := ""
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		candidate := seg
		if statusLeft != "" {
			candidate = statusLeft + "  " + seg
		}
		if lipgloss.Width(candidate) > contentW {
			break
		}
		statusLeft = candidate
	}

	// Wrap cleanly to additional lines instead of hard truncating tags.
	wrappedStatusLines := make([]string, 0, 2)
	currentLine := ""
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if currentLine == "" {
			currentLine = seg
			continue
		}
		candidate := currentLine + "  " + seg
		if lipgloss.Width(candidate) <= contentW {
			currentLine = candidate
			continue
		}
		wrappedStatusLines = append(wrappedStatusLines, currentLine)
		currentLine = seg
	}
	if currentLine != "" {
		wrappedStatusLines = append(wrappedStatusLines, currentLine)
	}
	statusLeft = strings.Join(wrappedStatusLines, "\n")

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
			m.activityFollowingTail = false
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
	if m == nil || m.isDetached() {
		return
	}
	params := protocol.PlanGetParams{
		ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
	}
	if strings.TrimSpace(m.teamID) != "" {
		params.TeamID = strings.TrimSpace(m.teamID)
		if strings.TrimSpace(m.focusedRunID) != "" {
			params.RunID = strings.TrimSpace(m.focusedRunID)
		} else {
			params.AggregateTeam = true
		}
	} else {
		params.RunID = strings.TrimSpace(m.runID)
	}
	var res protocol.PlanGetResult
	if err := m.rpcRoundTrip(protocol.MethodPlanGet, params, &res); err != nil {
		m.planLoadErr = err.Error()
		m.planDetailsErr = err.Error()
		m.dirtyPlan = true
		return
	}
	m.planMarkdown, m.planLoadErr = res.Checklist, res.ChecklistErr
	m.planDetails, m.planDetailsErr = res.Details, res.DetailsErr
	m.dirtyPlan = true
}

func (m *monitorModel) loadPlanFilesCmd() tea.Cmd {
	if m == nil || m.isDetached() {
		return nil
	}
	return func() tea.Msg {
		params := protocol.PlanGetParams{
			ThreadID: protocol.ThreadID(strings.TrimSpace(m.rpcRun().SessionID)),
		}
		if strings.TrimSpace(m.teamID) != "" {
			params.TeamID = strings.TrimSpace(m.teamID)
			if strings.TrimSpace(m.focusedRunID) != "" {
				params.RunID = strings.TrimSpace(m.focusedRunID)
			} else {
				params.AggregateTeam = true
			}
		} else {
			params.RunID = strings.TrimSpace(m.runID)
		}
		var res protocol.PlanGetResult
		if err := m.rpcRoundTrip(protocol.MethodPlanGet, params, &res); err != nil {
			return planFilesLoadedMsg{checklistErr: err.Error(), detailsErr: err.Error()}
		}
		return planFilesLoadedMsg{
			checklist:    res.Checklist,
			checklistErr: res.ChecklistErr,
			details:      res.Details,
			detailsErr:   res.DetailsErr,
		}
	}
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
		entryRunID := strings.TrimSpace(m.agentOutputRunID[i])
		if entryRunID != "" && entryRunID != targetRunID {
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
	if s == "" || runewidth.StringWidth(s) <= max {
		return s
	}
	if max <= 3 {
		return runewidth.Truncate(s, max, "")
	}
	return runewidth.Truncate(s, max-3, "") + "..."
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
