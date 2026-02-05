package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/pkg/events"
	llmtypes "github.com/tinoosan/workbench-core/pkg/llm/types"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

const (
	composeVPath     = "/project/.workbench/compose.md"
	planVPath        = "/plan/CHECKLIST.md"
	planDetailsVPath = "/plan/HEAD.md"
	// The details pane needs enough width for both transcript and sidebar.
	detailsPaneMinTerminalWidth = 72
)

// TurnRunner executes one user turn and returns the agent final response.
//
// The host (internal/app) owns the actual agent loop, memory commit policy,
// and persistence. The TUI calls this interface and renders events as they stream.
type TurnRunner interface {
	RunTurn(ctx context.Context, userMsg string) (final string, err error)
	ExecHostOp(ctx context.Context, req types.HostOpRequest, toolCallID string) (types.HostOpResponse, error)
	AppendToolResponse(toolCallID string, resp types.HostOpResponse)
	ResumeTurn(ctx context.Context, toolOutputs []llmtypes.LLMMessage) (string, error)
	ListSessions(ctx context.Context) ([]types.Session, error)

	ListSessionsPaginated(ctx context.Context, filter store.SessionFilter) ([]types.Session, error)
	CountSessions(ctx context.Context, filter store.SessionFilter) (int, error)
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

type vfsLister interface {
	ListVFS(ctx context.Context, path string) ([]vfs.Entry, error)
}

type eventMsg events.Event

type turnDoneMsg struct {
	final          string
	err            error
	preserveScroll bool // when true, do not scroll transcript after turn completes
}

type fileViewMsg struct {
	path      string
	content   string
	truncated bool
	err       error
}

type planFileMsg struct {
	path    string
	content string
	err     error
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

type sessionsListMsg struct {
	sessions []types.Session
	total    int
	page     int
	err      error
}

type preinitStatusMsg struct {
	workdir          string
	modelID          string
	reasoningEffort  string
	reasoningSummary string
	dataDir          string
	err              error
}

type Model struct {
	ctx context.Context

	runner TurnRunner
	events <-chan events.Event

	// quitByCtrlC is set when the user quits via the ctrl+c keybinding.
	// Bubble Tea returns nil error for a normal tea.Quit, so we track this to surface
	// an interrupt signal to callers (for example, to print a resume hint).
	quitByCtrlC bool

	transcript          viewport.Model
	activityList        list.Model
	activityDetail      viewport.Model
	planViewport        viewport.Model
	planMarkdown        string
	planDetailsMarkdown string
	planTabActive       bool
	planAutoExpanded    bool
	planLoadErr         string
	planDetailsLoadErr  string

	transcriptItems         []transcriptItem
	transcriptItemStartLine []int
	// Virtualized transcript rendering state (keeps items in memory, renders a window).
	transcriptRenderCache    map[int]*renderedItem // item index -> cached render
	transcriptWindow         transcriptWindow
	transcriptTotalLines     int // estimated total line count for all items
	transcriptLogicalYOffset int // scroll position in absolute lines (top of full transcript)
	lastTurnUserItemIdx      int
	currentActionGroupIdx    int
	currentActionCategory    string

	// Streaming (Phase 1): inline incremental agent output for the current turn.
	streamingItemIdx int
	streamingBuf     *strings.Builder

	// Thinking (Phase 2): provider reasoning indicator + optional summary for the current turn.
	thinkingItemIdx  int
	thinkingStep     int
	thinkingActive   bool
	thinkingStarted  time.Time
	thinkingDuration time.Duration
	thinkingSummary  string
	thinkingExpanded bool

	// fileSnapCache is a best-effort in-memory cache of files touched during this
	// session/run, used to render diff previews in the transcript.
	fileSnapCache map[string]string // vpath -> last-known text (possibly truncated)

	// pendingFileOpsByOpID tracks in-flight file ops keyed by host-emitted opId.
	// This avoids clobbering state under batch parallelism.
	pendingFileOpsByOpID map[string]pendingFileOp // opId -> pending op metadata
	pendingFileOpsQueue  map[string][]string

	// fileChanges holds grouped diff blocks for the current turn.
	fileChangesItemIdx int               // transcript item index for the grouped block, or -1
	fileChangesByPath  map[string]string // vpath -> rendered markdown snippet (includes diff fence)
	fileChangesOrder   []string          // stable insertion order of vpaths

	activities        []Activity
	activityIndexByID map[string]int
	// activityIndexByOpID maps host-emitted opId -> activity index, so concurrent ops
	// update the correct row even under batch parallelism.
	activityIndexByOpID map[string]int
	// pendingActivityID is legacy fallback for older hosts that don't emit opId.
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

	// turnCtx/turnCancel manage cancellation of the currently in-flight turn.
	// This allows stopping model output (and in-flight host ops) without exiting the TUI.
	turnCtx             context.Context
	turnCancel          context.CancelFunc
	turnCancelRequested bool

	turnInFlight bool
	turnStarted  time.Time
	turnTitle    string
	turnN        int

	pendingActionsByOpID map[string]pendingAction // opId -> pending transcript action line

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
	// When true, closing the in-TUI editor should load its buffer into the input
	// (used for compose fallback when $EDITOR/$VISUAL is not set).
	editorComposeOnClose bool
	// Tracks an external editor session that is composing a chat message (not editing a file).
	externalEditorComposeVPath string
	externalEditorComposePath  string
	composeLoadPath            string
	dataDir                    string

	sessionTitle     string
	workflowTitle    string
	workdir          string
	modelID          string
	reasoningEffort  string
	reasoningSummary string
	webSearchEnabled bool
	sessionID        string
	runID            string

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

	styleUserBox       lipgloss.Style
	styleUserLabel     lipgloss.Style
	styleAgentBox      lipgloss.Style
	styleFileChangeBox lipgloss.Style
	styleAgent         lipgloss.Style
	styleAction        lipgloss.Style
	styleTelemetry     lipgloss.Style
	styleOutcome       lipgloss.Style
	styleError         lipgloss.Style
	styleBold          lipgloss.Style

	styleInputBox            lipgloss.Style
	styleComposerCardFocused lipgloss.Style
	styleComposerCardBlurred lipgloss.Style
	styleComposerAccentFocus lipgloss.Style
	styleComposerAccentBlur  lipgloss.Style
	styleComposerStatusKey   lipgloss.Style
	styleComposerStatusVal   lipgloss.Style
	styleHint                lipgloss.Style
	styleRightTabActive      lipgloss.Style
	styleRightTabInactive    lipgloss.Style

	renderer *ContentRenderer

	// Command palette state
	commandPaletteOpen     bool
	commandPaletteMatches  []string
	commandPaletteSelected int

	// Model picker state
	modelPickerOpen bool
	modelPickerList list.Model

	// Session picker state (/sessions command)
	sessionPickerOpen     bool
	sessionPickerList     list.Model
	sessionPickerErr      string
	sessionPickerPage     int
	sessionPickerPageSize int
	sessionPickerTotal    int
	sessionPickerFilter   string

	// Session switching request (set before quitting)
	switchSessionID string
	switchNew       bool

	// Reasoning effort picker (opened via `/reasoning effort` with no value)
	reasoningEffortPickerOpen     bool
	reasoningEffortPickerSelected int

	// Reasoning summary picker (/reasoning summary command)
	reasoningSummaryPickerOpen     bool
	reasoningSummaryPickerSelected int

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
	transcriptThinking
	transcriptActionGroup
	transcriptError
	transcriptFileChange
)

type groupedAction struct {
	// text is the rendered description shown inside a category bucket.
	text string
	// status is a short completion marker ("✓" or "✗").
	status string
	// isError controls the styling of the status marker.
	isError bool
}

type transcriptItem struct {
	kind transcriptItemKind

	// For user/agent/error content (raw, unwrapped).
	text string

	// For action lines.
	groupHeader string
	groupItems  []groupedAction
}

type pendingAction struct {
	groupIdx  int
	actionIdx int
}

const (
	maxDiffBytesRead = 128 * 1024
	planMaxBytes     = 64 * 1024
)

type pendingFileOp struct {
	op   string
	path string

	// Best-effort previous content (from in-memory cache only).
	before    string
	hadBefore bool

	// For fs_patch we can show the patch itself (previewed/redacted by host).
	patchPreview   string
	patchTruncated bool
	patchRedacted  bool
}

type fileAfterMsg struct {
	opID string
	op   string
	path string

	text      string
	truncated bool
	err       error
}

type fileBeforeMsg struct {
	opID string
	op   string
	path string

	text      string
	truncated bool
	err       error
}

// renderedItem caches the rendered string and computed height for a transcript item.
// Height is invalidated when viewport width changes.
type renderedItem struct {
	rendered string
	height   int
	width    int
}

// transcriptWindow tracks the currently visible rendering window.
type transcriptWindow struct {
	// Item range being rendered (inclusive).
	firstItem int
	lastItem  int

	// Buffer sizes (items, not lines).
	bufferAbove int
	bufferBelow int

	// Offset tracking for viewport coordinate translation.
	contentStartLine int
}
