package tui

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/wordwrap"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tui/kit"
	agentstate "github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/protocol"
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
	artifactTaskSummaryMap  map[string]string
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

func resolveTeamControlSessionID(manifest *teamManifestFile, fallback string) string {
	fallback = strings.TrimSpace(fallback)
	if manifest == nil {
		return fallback
	}
	coordinatorRun := strings.TrimSpace(manifest.CoordinatorRun)
	firstSession := ""
	for _, role := range manifest.Roles {
		sessionID := strings.TrimSpace(role.SessionID)
		if sessionID == "" {
			continue
		}
		if firstSession == "" {
			firstSession = sessionID
		}
		if coordinatorRun != "" && strings.TrimSpace(role.RunID) == coordinatorRun {
			return sessionID
		}
	}
	if firstSession != "" {
		return firstSession
	}
	return fallback
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
	agentOutputMaxLines      = 50_000
	agentOutputDropChunk     = 5_000
	agentOutputSummaryMarker = "__WB_SUMMARY__:"

	// Keep a small, bounded thoughts history to avoid unbounded RAM growth.
	maxThinkingEntries = 50
)

// Breakpoints for responsive layout: below these use compact mode (tabs/single column).
const (
	compactModeMinWidth  = 110
	compactModeMinHeight = 32
)

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
	return m.dispatchUpdate(msg)
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

func (m *monitorModel) isCompactMode() bool {
	return m.width < compactModeMinWidth || m.height < compactModeMinHeight
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
