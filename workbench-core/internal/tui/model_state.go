package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/events"
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
	dataDir string
	err     error
}

type Model struct {
	ctx context.Context

	runner TurnRunner
	events <-chan events.Event

	// quitByCtrlC is set when the user quits via the ctrl+c keybinding.
	// Bubble Tea returns nil error for a normal tea.Quit, so we track this to surface
	// an interrupt signal to callers (for example, to print a resume hint).
	quitByCtrlC bool

	transcript     viewport.Model
	activityList   list.Model
	activityDetail viewport.Model

	transcriptItems         []transcriptItem
	transcriptItemStartLine []int
	lastTurnUserItemIdx     int

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

	// pendingFileOps tracks the last in-flight file op per path (request->response).
	pendingFileOps map[string]pendingFileOp // vpath -> pending op metadata

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
	// When true, closing the in-TUI editor should load its buffer into the input
	// (used for compose fallback when $EDITOR/$VISUAL is not set).
	editorComposeOnClose bool
	// Tracks an external editor session that is composing a chat message (not editing a file).
	externalEditorComposeVPath string
	externalEditorComposePath  string
	composeLoadPath            string
	dataDir                    string

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

	styleUserBox       lipgloss.Style
	styleUserLabel     lipgloss.Style
	styleAgentBox      lipgloss.Style
	styleFileChangeBox lipgloss.Style
	styleAgent         lipgloss.Style
	styleAction        lipgloss.Style
	styleTelemetry     lipgloss.Style
	styleOutcome       lipgloss.Style
	styleError         lipgloss.Style

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
	transcriptAction
	transcriptError
	transcriptFileChange
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

const maxDiffBytesRead = 128 * 1024

type pendingFileOp struct {
	op   string
	path string

	// Best-effort previous content (from in-memory cache only).
	before    string
	hadBefore bool

	// For fs.patch we can show the patch itself (previewed/redacted by host).
	patchPreview   string
	patchTruncated bool
	patchRedacted  bool
}

type fileAfterMsg struct {
	op   string
	path string

	text      string
	truncated bool
	err       error
}

type fileBeforeMsg struct {
	op   string
	path string

	text      string
	truncated bool
	err       error
}

