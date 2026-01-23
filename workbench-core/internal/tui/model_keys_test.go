package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/types"
)

type stubRunner struct {
	final string
	err   error
}

func (s stubRunner) RunTurn(ctx context.Context, userMsg string) (string, error) {
	_ = ctx
	_ = userMsg
	return s.final, s.err
}

func (s stubRunner) ExecHostOp(ctx context.Context, req types.HostOpRequest, toolCallID string) (types.HostOpResponse, error) {
	_ = ctx
	_ = toolCallID
	return types.HostOpResponse{Op: req.Op, Ok: true}, nil
}

func (s stubRunner) AppendToolResponse(toolCallID string, resp types.HostOpResponse) {
	_ = toolCallID
	_ = resp
}

func (s stubRunner) ResumeTurn(ctx context.Context, toolOutputs []types.LLMMessage) (string, error) {
	_ = toolOutputs
	return s.RunTurn(ctx, "")
}

type recordingRunner struct {
	stubRunner
	lastMessage string
}

func (r *recordingRunner) RunTurn(ctx context.Context, userMsg string) (string, error) {
	r.lastMessage = userMsg
	return "Model set to " + strings.TrimPrefix(userMsg, "/model "), nil
}

type recordingRunnerAny struct {
	stubRunner
	lastMessage string
}

func (r *recordingRunnerAny) RunTurn(ctx context.Context, userMsg string) (string, error) {
	_ = ctx
	r.lastMessage = userMsg
	return "", nil
}

type stubRunnerWithRead struct {
	stubRunner
	files map[string]string
}

func (s stubRunnerWithRead) ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error) {
	_ = ctx
	if s.files == nil {
		return "", 0, false, nil
	}
	txt := s.files[path]
	b := []byte(txt)
	bytesLen = len(b)
	if maxBytes > 0 && len(b) > maxBytes {
		b = b[:maxBytes]
		truncated = true
	}
	return string(b), bytesLen, truncated, nil
}

type runnerWithPwd struct {
	stubRunner
	workdir string
}

func (r runnerWithPwd) RunTurn(ctx context.Context, userMsg string) (string, error) {
	_ = ctx
	if strings.TrimSpace(userMsg) == "/pwd" {
		return strings.TrimSpace(r.workdir), nil
	}
	return r.stubRunner.RunTurn(ctx, userMsg)
}

type blockingRunner struct {
	started chan struct{}
}

func (r blockingRunner) RunTurn(ctx context.Context, userMsg string) (string, error) {
	_ = userMsg
	if r.started != nil {
		select {
		case <-r.started:
			// already closed
		default:
			close(r.started)
		}
	}
	<-ctx.Done()
	return "", ctx.Err()
}

func (r blockingRunner) AppendToolResponse(toolCallID string, resp types.HostOpResponse) {
	_ = toolCallID
	_ = resp
}

func (r blockingRunner) ExecHostOp(ctx context.Context, req types.HostOpRequest, toolCallID string) (types.HostOpResponse, error) {
	_ = ctx
	_ = toolCallID
	return types.HostOpResponse{Op: req.Op, Ok: true}, nil
}

func (r blockingRunner) ResumeTurn(ctx context.Context, toolOutputs []types.LLMMessage) (string, error) {
	_ = toolOutputs
	return r.RunTurn(ctx, "")
}

func TestKeyHandling_EnterSubmitsEvenWhenDetailsVisible(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.showDetails = true
	m.layout()

	m.single.SetValue("hello")
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if cmd == nil {
		t.Fatalf("expected submit cmd, got nil")
	}
	if !updated.turnInFlight {
		t.Fatalf("expected turnInFlight true")
	}
}

func TestKeyHandling_CtrlXStopsTurnWithoutQuitting(t *testing.T) {
	started := make(chan struct{})
	m := New(context.Background(), blockingRunner{started: started}, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.layout()

	m.single.SetValue("hello")
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)
	if cmd == nil {
		t.Fatalf("expected submit cmd, got nil")
	}
	if !updated.turnInFlight {
		t.Fatalf("expected turnInFlight true after submit")
	}

	msgCh := make(chan tea.Msg, 1)
	go func() {
		msgCh <- cmd()
	}()

	// Wait for the runner to start (best-effort) so the cancel is meaningful.
	select {
	case <-started:
	default:
	}

	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
	stopping := m3.(Model)
	if !stopping.turnInFlight {
		t.Fatalf("expected turnInFlight true immediately after ctrl+x (until turn completes)")
	}

	doneMsg := <-msgCh
	m4, _ := stopping.Update(doneMsg)
	final := m4.(Model)
	if final.turnInFlight {
		t.Fatalf("expected turnInFlight false after cancellation completes")
	}
	for _, it := range final.transcriptItems {
		if it.kind == transcriptError && strings.Contains(it.text, "agent error:") {
			t.Fatalf("expected no agent error transcript line on ctrl+x stop, got: %q", it.text)
		}
	}
}

func TestKeyHandling_TypingEIsNotHijackedByDetails(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.showDetails = true
	m.layout()

	// Simulate typing "e" into the input.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	updated := m2.(Model)
	if updated.single.Value() != "e" {
		t.Fatalf("expected input to contain %q, got %q", "e", updated.single.Value())
	}
}

func TestReasoningEffortPicker_OpensOnReasoningEffortNoValue(t *testing.T) {
	m := New(context.Background(), &recordingRunnerAny{}, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.layout()

	m.single.SetValue("/reasoning-effort")
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if cmd != nil {
		t.Fatalf("expected no cmd (should open picker only), got %v", cmd)
	}
	if !updated.reasoningEffortPickerOpen {
		t.Fatalf("expected reasoningEffortPickerOpen true")
	}
	if updated.turnInFlight {
		t.Fatalf("expected turnInFlight false (should not submit)")
	}
}

func TestReasoningEffortPicker_SelectRunsReasoningCommand(t *testing.T) {
	runner := &recordingRunnerAny{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.layout()

	// Open picker.
	m.single.SetValue("/reasoning-effort")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	opened := m2.(Model)
	if !opened.reasoningEffortPickerOpen {
		t.Fatalf("expected picker open")
	}

	// Move selection down once from default "medium" -> "high".
	m3, _ := opened.Update(tea.KeyMsg{Type: tea.KeyDown})
	moved := m3.(Model)
	if !moved.reasoningEffortPickerOpen {
		t.Fatalf("expected picker still open")
	}

	// Enter selects.
	beforeInFlight := moved.turnInFlight
	m4, cmd := moved.Update(tea.KeyMsg{Type: tea.KeyEnter})
	afterSelect := m4.(Model)

	if cmd == nil {
		t.Fatalf("expected cmd to run /reasoning effort <val>, got nil")
	}
	if afterSelect.reasoningEffortPickerOpen {
		t.Fatalf("expected picker closed after selection")
	}
	if afterSelect.turnInFlight != beforeInFlight {
		t.Fatalf("expected turnInFlight unchanged; before=%v after=%v", beforeInFlight, afterSelect.turnInFlight)
	}

	// Execute cmd and feed result back to update loop.
	msg := cmd()
	m5, _ := afterSelect.Update(msg)
	_ = m5.(Model)

	if runner.lastMessage != "/reasoning effort high" {
		t.Fatalf("expected runner called with %q, got %q", "/reasoning effort high", runner.lastMessage)
	}
}

func TestReasoningEffortPicker_EscClosesWithoutRunning(t *testing.T) {
	runner := &recordingRunnerAny{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.layout()

	m.single.SetValue("/reasoning-effort")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	opened := m2.(Model)
	if !opened.reasoningEffortPickerOpen {
		t.Fatalf("expected picker open")
	}

	m3, cmd := opened.Update(tea.KeyMsg{Type: tea.KeyEsc})
	closed := m3.(Model)
	if cmd != nil {
		t.Fatalf("expected no cmd on esc, got %v", cmd)
	}
	if closed.reasoningEffortPickerOpen {
		t.Fatalf("expected picker closed after esc")
	}
	if runner.lastMessage != "" {
		t.Fatalf("expected runner not called, got %q", runner.lastMessage)
	}
}

func TestFocus_CtrlATogglesActivityAndFocus(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.showDetails = false
	m.layout()

	// Open activity: should move focus away from input.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	opened := m2.(Model)
	if !opened.showDetails {
		t.Fatalf("expected showDetails true after ctrl+a")
	}
	if opened.focus != focusActivityList {
		t.Fatalf("expected focusActivityList after opening activity, got %v", opened.focus)
	}

	// Tab should return focus to input.
	m3, _ := opened.Update(tea.KeyMsg{Type: tea.KeyTab})
	back := m3.(Model)
	if back.focus != focusInput {
		t.Fatalf("expected focusInput after tab, got %v", back.focus)
	}
}

func TestKeyRouting_WhenActivityFocused_InputDoesNotConsumeKeys(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.showDetails = true
	m.focus = focusActivityList
	m.single.Blur()
	m.multiline.Blur()
	m.layout()

	// "e" should toggle details, not type into the input.
	if m.expandOutput {
		t.Fatalf("expected expandOutput false initially")
	}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	updated := m2.(Model)
	if !updated.expandOutput {
		t.Fatalf("expected expandOutput to toggle true on 'e' when activity focused")
	}
	if updated.single.Value() != "" {
		t.Fatalf("expected input to remain empty when activity focused, got %q", updated.single.Value())
	}

	// Other typing should also be ignored by the input.
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	updated2 := m3.(Model)
	if updated2.single.Value() != "" {
		t.Fatalf("expected input to remain empty when activity focused, got %q", updated2.single.Value())
	}
}

func TestKeyRouting_WhenActivityFocused_CtrlTAndCtrlGDoNotToggleInputModes(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.showDetails = true
	m.focus = focusActivityList
	m.single.Blur()
	m.multiline.Blur()
	m.layout()

	telemetryBefore := m.showTelemetry
	isMultiBefore := m.isMulti

	// Ctrl+T SHOULD toggle telemetry when activity is focused.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	updated := m2.(Model)
	if updated.showTelemetry == telemetryBefore {
		t.Fatalf("expected showTelemetry to toggle when activity focused; before=%v after=%v", telemetryBefore, updated.showTelemetry)
	}

	// Plain "t" SHOULD toggle telemetry when activity is focused.
	telemetry2 := updated.showTelemetry
	m2b, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	updatedB := m2b.(Model)
	if updatedB.showTelemetry == telemetry2 {
		t.Fatalf("expected showTelemetry to toggle on 't' when activity focused; before=%v after=%v", telemetry2, updatedB.showTelemetry)
	}

	// Ctrl+G should NOT toggle multiline when activity is focused.
	m3, _ := updatedB.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	updated2 := m3.(Model)
	if updated2.isMulti != isMultiBefore {
		t.Fatalf("expected isMulti unchanged when activity focused; before=%v after=%v", isMultiBefore, updated2.isMulti)
	}
}

func TestKeyRouting_WhenInputFocused_TypingTDoesNotToggleTelemetry(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.focus = focusInput
	m.showDetails = false
	m.layout()

	before := m.showTelemetry
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	updated := m2.(Model)

	if updated.showTelemetry != before {
		t.Fatalf("expected showTelemetry unchanged when input focused and typing 't'; before=%v after=%v", before, updated.showTelemetry)
	}
	if updated.single.Value() != "t" {
		t.Fatalf("expected input to contain %q, got %q", "t", updated.single.Value())
	}
}

func TestTranscript_FirstUserMessageVisibleAtTop(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.layout()

	// Simulate first user message.
	m.addTranscriptItem(transcriptItem{kind: transcriptUser, text: "line1\nline2\nline3"})

	if m.transcript.YOffset != 0 {
		t.Fatalf("expected first message to start at top (YOffset=0), got %d", m.transcript.YOffset)
	}
}

func TestTranscript_ScrollAnchorsToTurnStartOnCompletion(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	// Small viewport to force scrolling with a long agent message.
	m.width = 120
	m.height = 20
	m.layout()

	// Start a turn.
	_ = m.submit("User request\nwith multiple lines")

	// Add a long agent message to create lots of lines.
	long := ""
	for i := 0; i < 200; i++ {
		long += "line\n"
	}
	m2, _ := m.Update(turnDoneMsg{final: long})
	updated := m2.(Model)

	if updated.lastTurnUserItemIdx < 0 {
		t.Fatalf("expected lastTurnUserItemIdx to be set")
	}
	if updated.lastTurnUserItemIdx >= len(updated.transcriptItemStartLine) {
		t.Fatalf("expected transcriptItemStartLine to include user item")
	}
	want := updated.transcriptItemStartLine[updated.lastTurnUserItemIdx]
	if updated.transcript.YOffset != want {
		t.Fatalf("expected YOffset to anchor to turn start %d, got %d", want, updated.transcript.YOffset)
	}
}

func TestLayout_WithActivitySidebar_DoesNotClipHeader(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 30
	m.showDetails = true
	m.layout()

	view := m.View()
	// Header should be present at the top of the view string.
	if !strings.HasPrefix(view, "workbench") {
		t.Fatalf("expected header to start with %q, got prefix %q", "workbench", view[:min(len(view), 32)])
	}
}

func TestLayout_NarrowTerminal_HeaderStillVisibleWhenFooterWraps(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 52
	m.height = 20
	m.showDetails = true
	m.layout()

	view := m.View()
	firstLine := strings.SplitN(view, "\n", 2)[0]
	if !strings.Contains(firstLine, "workbench") {
		t.Fatalf("expected header line to contain %q, got %q", "workbench", firstLine)
	}
}

func TestLayout_ViewNeverExceedsTerminalHeight_WhenActivityOpen(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 30
	m.showDetails = true
	m.layout()

	view := m.View()
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("expected View() height <= %d, got %d", m.height, got)
	}
}

func TestTranscript_PgDnScrollsEvenWhenInputFocused(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 100
	m.height = 16
	m.layout()

	// Fill transcript so it can scroll.
	for i := 0; i < 80; i++ {
		m.addTranscriptItem(transcriptItem{kind: transcriptAgent, text: "line"})
	}

	// Force to top so PgDn has room to scroll.
	m.transcript.SetYOffset(0)
	before := m.transcript.YOffset
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	updated := m2.(Model)
	after := updated.transcript.YOffset
	if after <= before {
		t.Fatalf("expected PgDn to increase YOffset, before=%d after=%d", before, after)
	}
}

func TestLayout_ViewLinesDoNotExceedTerminalWidth_WhenActivityOpen(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 30
	m.showDetails = true
	m.layout()

	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > m.width {
			t.Fatalf("line %d exceeds terminal width: got %d, want <= %d; line=%q", i+1, w, m.width, line)
		}
	}
}

func TestLayout_WithCommandPalette_ViewNeverExceedsTerminalBounds(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.showDetails = true
	m.layout()

	// Open palette by typing "/".
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := m2.(Model)
	if !updated.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen true")
	}

	view := updated.View()
	if got := lipgloss.Height(view); got > updated.height {
		t.Fatalf("expected View() height <= %d, got %d", updated.height, got)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > updated.width {
			t.Fatalf("line %d exceeds terminal width: got %d, want <= %d; line=%q", i+1, w, updated.width, line)
		}
	}
}

func TestActivity_OpenFileViewer_OKey(t *testing.T) {
	r := stubRunnerWithRead{
		stubRunner: stubRunner{final: "ok"},
		files: map[string]string{
			"/scratch/example.json": "{\n  \"ok\": true\n}\n",
		},
	}
	m := New(context.Background(), r, make(chan events.Event))
	m.width = 120
	m.height = 30
	m.showDetails = true
	m.focus = focusActivityList
	m.single.Blur()
	m.multiline.Blur()
	m.layout()

	m.activities = []Activity{
		{ID: "act-1", Kind: "fs.write", Title: "Write /scratch/example.json", Status: ActivityOK, Path: "/scratch/example.json"},
	}
	m.refreshActivityList()
	m.activityList.Select(0)
	m.refreshActivityDetail()

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	updated := m2.(Model)
	if cmd == nil {
		t.Fatalf("expected open-file cmd, got nil")
	}
	if !updated.fileViewOpen {
		t.Fatalf("expected fileViewOpen true")
	}
	if updated.fileViewPath != "/scratch/example.json" {
		t.Fatalf("expected fileViewPath %q, got %q", "/scratch/example.json", updated.fileViewPath)
	}

	msg := cmd()
	m3, _ := updated.Update(msg)
	updated2 := m3.(Model)
	if updated2.fileViewErr != "" {
		t.Fatalf("unexpected file view error: %s", updated2.fileViewErr)
	}
	if !strings.Contains(updated2.fileViewContent, "\"ok\"") {
		t.Fatalf("expected file content to be loaded, got %q", updated2.fileViewContent)
	}
}

func TestCommandPalette_OpensOnSlashPrefix(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.layout()

	// Type "/" - palette should open.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := m2.(Model)
	if !updated.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen true after typing '/'")
	}
	if len(updated.commandPaletteMatches) == 0 {
		t.Fatalf("expected commandPaletteMatches to have items")
	}
}

func TestCommandPalette_FiltersOnTyping(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.layout()

	// Type "/mo" - should filter to "/model".
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := m2.(Model)
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	updated2 := m3.(Model)
	m4, _ := updated2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	updated3 := m4.(Model)

	if !updated3.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen true after typing '/mo'")
	}
	found := false
	for _, match := range updated3.commandPaletteMatches {
		if match == "/model" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected '/model' in matches, got %v", updated3.commandPaletteMatches)
	}
}

func TestCommandPalette_EnterAutocompletesWithoutSubmitting(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.layout()

	// Type "/mo" to get "/model" as first match.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := m2.(Model)
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	updated2 := m3.(Model)
	m4, _ := updated2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	updated3 := m4.(Model)

	if !updated3.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen true")
	}
	if updated3.commandPaletteSelected != 0 {
		t.Fatalf("expected selected index 0, got %d", updated3.commandPaletteSelected)
	}

	// Press Enter - should autocomplete to "/model" but NOT submit.
	m5, cmd := updated3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated4 := m5.(Model)

	if updated4.single.Value() != "/model" {
		t.Fatalf("expected input value '/model', got %q", updated4.single.Value())
	}
	if updated4.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen false after Enter")
	}
	if updated4.turnInFlight {
		t.Fatalf("expected turnInFlight false (should not submit)")
	}
	if cmd != nil {
		t.Fatalf("expected no cmd (should not submit), got %v", cmd)
	}

	// After autocompletion, typing args (e.g. Space) should NOT reopen the palette.
	m6, _ := updated4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	updated5 := m6.(Model)
	if updated5.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen false after typing space following autocomplete")
	}
}

func TestCommandPalette_UpDownNavigation(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.layout()

	// Type "/" to open palette with multiple matches.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := m2.(Model)

	if !updated.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen true")
	}
	if len(updated.commandPaletteMatches) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(updated.commandPaletteMatches))
	}

	// Press Down - should move selection.
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyDown})
	updated2 := m3.(Model)
	if updated2.commandPaletteSelected != 1 {
		t.Fatalf("expected selected index 1 after Down, got %d", updated2.commandPaletteSelected)
	}

	// Press Up - should move back.
	m4, _ := updated2.Update(tea.KeyMsg{Type: tea.KeyUp})
	updated3 := m4.(Model)
	if updated3.commandPaletteSelected != 0 {
		t.Fatalf("expected selected index 0 after Up, got %d", updated3.commandPaletteSelected)
	}
}

func TestCommandPalette_EscClosesPalette(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.layout()

	// Type "/mo" to open palette.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := m2.(Model)
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	updated2 := m3.(Model)

	if !updated2.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen true")
	}

	// Press Esc - should close palette but keep input.
	m4, _ := updated2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated3 := m4.(Model)

	if updated3.commandPaletteOpen {
		t.Fatalf("expected commandPaletteOpen false after Esc")
	}
	if updated3.single.Value() != "/m" {
		t.Fatalf("expected input to remain '/m', got %q", updated3.single.Value())
	}
}

func TestCommandPalette_PreservesTrailingArgs(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.layout()

	// Type "/mo arg1 arg2" - should preserve args after autocomplete.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	updated := m2.(Model)
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	updated2 := m3.(Model)
	m4, _ := updated2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	updated3 := m4.(Model)
	m5, _ := updated3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	updated4 := m5.(Model)
	m6, _ := updated4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	updated5 := m6.(Model)
	m7, _ := updated5.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	updated6 := m7.(Model)
	m8, _ := updated6.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	updated7 := m8.(Model)

	if updated7.single.Value() != "/mo arg" {
		t.Fatalf("expected input '/mo arg', got %q", updated7.single.Value())
	}

	// Press Enter - should autocomplete to "/model arg".
	m9, _ := updated7.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated8 := m9.(Model)

	if updated8.single.Value() != "/model arg" {
		t.Fatalf("expected input '/model arg', got %q", updated8.single.Value())
	}
}

func TestModelPicker_OpensOnModelCommand(t *testing.T) {
	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.showDetails = true
	m.layout()

	// Type "/model" and submit
	m.single.SetValue("/model")
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if updated.modelPickerOpen != true {
		t.Fatalf("expected modelPickerOpen true after submitting '/model'")
	}
	if updated.turnInFlight {
		t.Fatalf("expected turnInFlight false (should not submit)")
	}
	if cmd != nil {
		t.Fatalf("expected no cmd, got %v", cmd)
	}
}

func TestModelPicker_FiltersOnTyping(t *testing.T) {
	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.showDetails = true
	m.layout()

	// Open picker
	m.single.SetValue("/model")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if !updated.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen true")
	}

	// Prime list internals (VisibleItems can be empty until first render).
	_ = updated.modelPickerList.View()

	beforeN := len(updated.modelPickerList.VisibleItems())
	if beforeN == 0 {
		t.Fatalf("expected visible items > 0 before filtering")
	}

	// Type filter text
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	updated2 := m3.(Model)
	m4, _ := updated2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	updated3 := m4.(Model)
	m5, _ := updated3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	updated4 := m5.(Model)

	// Prime list internals after filter updates.
	_ = updated4.modelPickerList.View()

	if got := updated4.modelPickerList.FilterValue(); got != "gpt" {
		t.Fatalf("expected filter value 'gpt', got %q", got)
	}

	afterItems := updated4.modelPickerList.VisibleItems()
	afterN := len(afterItems)
	if afterN == 0 {
		t.Fatalf("expected visible items > 0 after filtering")
	}
	if afterN > beforeN {
		t.Fatalf("expected visible items to not increase after filtering: before=%d after=%d", beforeN, afterN)
	}
	for _, it := range afterItems {
		mi, ok := it.(modelPickerItem)
		if !ok {
			continue
		}
		if !strings.Contains(mi.id, "gpt") {
			t.Fatalf("expected all visible items to match filter 'gpt'; got %q", mi.id)
		}
	}
}

func TestModelPicker_SelectsModelAndUpdatesLabel(t *testing.T) {
	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.showDetails = true
	m.layout()
	m.modelID = "old-model"

	// Open picker
	m.single.SetValue("/model")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if !updated.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen true")
	}

	// Prime list internals so selection works consistently.
	_ = updated.modelPickerList.View()

	// Select first model (should be sorted, likely starting with openai/gpt-4o-mini or similar)
	if len(updated.modelPickerList.Items()) == 0 {
		t.Fatalf("expected at least one model in picker")
	}

	// Press Enter to select
	m3, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated2 := m3.(Model)

	if updated2.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen false after selecting")
	}
	if cmd == nil {
		t.Fatalf("expected cmd (RunTurn), got nil")
	}

	// Check that modelID was optimistically updated
	selectedItem := updated.modelPickerList.SelectedItem()
	if selectedItem == nil {
		t.Fatalf("expected selected item")
	}
	modelItem, ok := selectedItem.(modelPickerItem)
	if !ok {
		t.Fatalf("expected modelPickerItem")
	}
	expectedModel := modelItem.id

	// The optimistic update happens before the command runs
	// We need to execute the command to see the final state
	msg := cmd()
	m4, _ := updated2.Update(msg)
	_ = m4.(Model)

	// After turnDoneMsg, the modelID should still be set (optimistic update persists)
	// and the runner should have been called
	if runner.lastMessage != "/model "+expectedModel {
		t.Fatalf("expected runner.RunTurn called with '/model %s', got %q", expectedModel, runner.lastMessage)
	}
}

func TestModelPicker_EscClosesPicker(t *testing.T) {
	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.showDetails = true
	m.layout()

	// Open picker
	m.single.SetValue("/model")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if !updated.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen true")
	}
	beforeShowDetails := updated.showDetails

	// Press Esc - should close picker
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated2 := m3.(Model)

	if updated2.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen false after Esc")
	}
	if updated2.showDetails != beforeShowDetails {
		t.Fatalf("expected showDetails unchanged; before=%t after=%t", beforeShowDetails, updated2.showDetails)
	}
	if runner.lastMessage != "" {
		t.Fatalf("expected runner not called, but got %q", runner.lastMessage)
	}
}

func TestModelPicker_ModelCommandWithArgsStillWorks(t *testing.T) {
	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.showDetails = true
	m.layout()

	// Type "/model openai/gpt-4o" and submit - should NOT open picker
	m.single.SetValue("/model openai/gpt-4o")
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if updated.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen false (should submit normally)")
	}
	if !updated.turnInFlight {
		t.Fatalf("expected turnInFlight true (should submit)")
	}
	if cmd == nil {
		t.Fatalf("expected cmd (submit), got nil")
	}
}

func writeTempFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("writefile: %v", err)
	}
}

func filePickerItems(m Model) []string {
	out := []string{}
	for _, it := range m.filePickerList.Items() {
		fi, ok := it.(filePickerItem)
		if !ok {
			continue
		}
		out = append(out, fi.rel)
	}
	return out
}

func TestFilePicker_OpensOnAtAndFiltersFromInput(t *testing.T) {
	tmp, err := os.MkdirTemp("", "workbench-filepicker-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	writeTempFile(t, tmp, "README.md", "hi")
	writeTempFile(t, tmp, "cmd/main.go", "package main")
	writeTempFile(t, tmp, "docs/guide.md", "guide")

	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.workdir = tmp
	m.layout()

	// Type "@" -> opens picker.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	updated := m2.(Model)
	if !updated.filePickerOpen {
		t.Fatalf("expected filePickerOpen true after typing '@'")
	}
	if len(updated.filePickerList.Items()) == 0 {
		t.Fatalf("expected file picker to have items")
	}

	// Type "@read" -> should filter to README.md (case-insensitive substring).
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m4 := m3.(Model)
	m5, _ := m4.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m6 := m5.(Model)
	m7, _ := m6.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m8 := m7.(Model)
	m9, _ := m8.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	updated2 := m9.(Model)

	items := filePickerItems(updated2)
	if len(items) == 0 {
		t.Fatalf("expected items after filtering")
	}
	found := false
	for _, p := range items {
		if p == "README.md" {
			found = true
		}
		// Ensure all items match substring "read".
		if !strings.Contains(strings.ToLower(p), "read") {
			t.Fatalf("expected all items to contain 'read', got %q", p)
		}
	}
	if !found {
		t.Fatalf("expected README.md in filtered items, got %v", items)
	}
}

func TestFilePicker_OpensEvenWhenWorkdirUnknown(t *testing.T) {
	tmp, err := os.MkdirTemp("", "workbench-filepicker-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	writeTempFile(t, tmp, "a.txt", "a")

	m := New(context.Background(), runnerWithPwd{workdir: tmp}, make(chan events.Event))
	m.width = 120
	m.height = 24
	// m.workdir intentionally left empty.
	m.layout()

	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	opened := m2.(Model)
	if !opened.filePickerOpen {
		t.Fatalf("expected filePickerOpen true even when workdir unknown")
	}
	if cmd == nil {
		t.Fatalf("expected cmd to prefetch /pwd")
	}

	// Simulate prefetch completion (Bubble Tea would execute the cmd asynchronously).
	m3, _ := opened.Update(workdirPrefetchMsg{workdir: tmp})
	updated := m3.(Model)
	if strings.TrimSpace(updated.workdir) != tmp {
		t.Fatalf("expected workdir set to %q, got %q", tmp, updated.workdir)
	}
	if len(updated.filePickerList.Items()) == 0 {
		t.Fatalf("expected picker populated after workdir prefetch")
	}
}

func TestFilePicker_EnterInsertsAtRefAndCloses(t *testing.T) {
	tmp, err := os.MkdirTemp("", "workbench-filepicker-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	writeTempFile(t, tmp, "a.txt", "a")

	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.workdir = tmp
	m.layout()

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	opened := m2.(Model)
	if !opened.filePickerOpen {
		t.Fatalf("expected filePickerOpen true after '@'")
	}

	m3, cmd := opened.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m3.(Model)
	if cmd != nil {
		t.Fatalf("expected no cmd (insert only), got %v", cmd)
	}
	if updated.filePickerOpen {
		t.Fatalf("expected filePickerOpen false after selection")
	}
	if updated.single.Value() != "@a.txt " {
		t.Fatalf("expected input to be %q, got %q", "@a.txt ", updated.single.Value())
	}
}

func TestFilePicker_EditorCommand_SelectRunsImmediately(t *testing.T) {
	tmp, err := os.MkdirTemp("", "workbench-filepicker-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	writeTempFile(t, tmp, "a.txt", "a")

	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.workdir = tmp
	m.layout()

	m.single.SetValue("/editor ")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	opened := m2.(Model)
	if !opened.filePickerOpen {
		t.Fatalf("expected filePickerOpen true after '@' in /editor arg")
	}

	m3, cmd := opened.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m3.(Model)
	if cmd == nil {
		t.Fatalf("expected cmd (auto-run /editor) got nil")
	}
	if !updated.turnInFlight {
		t.Fatalf("expected turnInFlight true after selecting for /editor")
	}
	if updated.single.Value() != "" {
		t.Fatalf("expected input cleared, got %q", updated.single.Value())
	}

	// Execute the cmd and feed it back so the runner records the message.
	msg := cmd()
	m4, _ := updated.Update(msg)
	_ = m4.(Model)

	if runner.lastMessage != "/editor @a.txt" {
		t.Fatalf("expected runner called with %q, got %q", "/editor @a.txt", runner.lastMessage)
	}
}

func TestFilePicker_CdCommand_DoesNotAutoRun(t *testing.T) {
	tmp, err := os.MkdirTemp("", "workbench-filepicker-*")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	writeTempFile(t, tmp, "a.txt", "a")

	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.workdir = tmp
	m.layout()

	m.single.SetValue("/cd ")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	opened := m2.(Model)
	if !opened.filePickerOpen {
		t.Fatalf("expected filePickerOpen true after '@' in /cd arg")
	}

	m3, cmd := opened.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m3.(Model)
	if cmd != nil {
		t.Fatalf("expected no cmd (should not auto-run /cd), got %v", cmd)
	}
	if updated.turnInFlight {
		t.Fatalf("expected turnInFlight false after /cd selection (insert only)")
	}
	if runner.lastMessage != "" {
		t.Fatalf("expected runner not called, got %q", runner.lastMessage)
	}
	if updated.single.Value() != "/cd @a.txt " {
		t.Fatalf("expected input %q, got %q", "/cd @a.txt ", updated.single.Value())
	}
}

func TestEditorCompose_LoadsComposeFileIntoMultilineOnExit(t *testing.T) {
	t.Setenv("EDITOR", "true") // ensure compose uses external editor path

	dataDir := t.TempDir()
	composeAbs := filepath.Join(dataDir, "compose.md")
	if err := os.MkdirAll(filepath.Dir(composeAbs), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	want := "hello\nfrom editor\n"
	if err := os.WriteFile(composeAbs, []byte(want), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.showDetails = true
	m.dataDir = dataDir
	m.layout()

	// Simulate host telling us to open compose editor.
	m2, _ := m.Update(eventMsg(events.Event{
		Type: "ui.editor.open",
		Data: map[string]string{
			"absPath": composeAbs,
			"purpose": "compose",
		},
	}))
	updated := m2.(Model)
	if strings.TrimSpace(updated.externalEditorComposePath) == "" {
		t.Fatalf("expected externalEditorComposePath to be set")
	}

	// Simulate external editor exiting successfully.
	m3, cmd := updated.Update(editorExternalDoneMsg{vpath: composeAbs, err: nil})
	updated2 := m3.(Model)
	if cmd == nil {
		t.Fatalf("expected compose load cmd")
	}
	msg := cmd()
	m4, _ := updated2.Update(msg)
	updated3 := m4.(Model)

	if !updated3.isMulti {
		t.Fatalf("expected isMulti true")
	}
	if got := updated3.multiline.Value(); got != want {
		t.Fatalf("expected multiline to contain compose text; got %q", got)
	}
	if updated3.turnInFlight {
		t.Fatalf("expected not to submit")
	}
}

func TestCtrlE_PrefillsComposeFileAndLoadsOnExit(t *testing.T) {
	t.Setenv("EDITOR", "true") // ensure editorExecCmd can construct a command

	workdir := t.TempDir()
	dataDir := t.TempDir()
	composeAbs := filepath.Join(dataDir, "compose.md")
	if err := os.MkdirAll(filepath.Dir(composeAbs), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 24
	m.showDetails = true
	m.workdir = workdir
	m.dataDir = dataDir
	m.layout()

	// Start with some input.
	m.isMulti = true
	m.multiline.SetValue("draft line 1\nline 2\n")

	// Ctrl+E should write compose file and return a cmd (editor launch).
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	updated := m2.(Model)
	if cmd == nil {
		t.Fatalf("expected cmd from ctrl+e")
	}
	b, err := os.ReadFile(composeAbs)
	if err != nil {
		t.Fatalf("read compose: %v", err)
	}
	if got := string(b); got != "draft line 1\nline 2\n" {
		t.Fatalf("expected compose prefill to match input; got %q", got)
	}

	// Now simulate editor changed the file.
	want := "edited in vim\nsecond line\n"
	if err := os.WriteFile(composeAbs, []byte(want), 0644); err != nil {
		t.Fatalf("write compose: %v", err)
	}

	m3, loadCmd := updated.Update(editorExternalDoneMsg{vpath: composeAbs, err: nil})
	updated2 := m3.(Model)
	if loadCmd == nil {
		t.Fatalf("expected compose load cmd")
	}
	loadMsg := loadCmd()
	m4, _ := updated2.Update(loadMsg)
	updated3 := m4.(Model)
	if got := updated3.multiline.Value(); got != want {
		t.Fatalf("expected multiline updated from compose; got %q", got)
	}
}

func TestHelpModal_CtrlPOpens_Scrolls_AndEscCloses(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 30
	m.showDetails = true
	m.layout()

	// Open help modal.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	opened := m2.(Model)
	if !opened.helpModalOpen {
		t.Fatalf("expected helpModalOpen true after ctrl+p")
	}

	// Ensure view stays within terminal bounds (no weird clipping artifacts).
	view := opened.View()
	if got := lipgloss.Height(view); got > opened.height {
		t.Fatalf("expected View() height <= %d, got %d", opened.height, got)
	}
	for i, line := range strings.Split(view, "\n") {
		if w := lipgloss.Width(line); w > opened.width {
			t.Fatalf("line %d exceeds terminal width: got %d, want <= %d; line=%q", i+1, w, opened.width, line)
		}
	}

	// Scroll down a bit.
	before := opened.helpViewport.YOffset
	m3, _ := opened.Update(tea.KeyMsg{Type: tea.KeyDown})
	scrolled := m3.(Model)
	if scrolled.helpViewport.YOffset == before {
		// It's OK for tiny terminals, but with our fixed content it should scroll.
		t.Fatalf("expected help modal to scroll on Down; before=%d after=%d", before, scrolled.helpViewport.YOffset)
	}

	// Vim keys should work too.
	beforeJK := scrolled.helpViewport.YOffset
	mJK1, _ := scrolled.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	scrolledJ := mJK1.(Model)
	if scrolledJ.helpViewport.YOffset <= beforeJK {
		t.Fatalf("expected 'j' to scroll down; before=%d after=%d", beforeJK, scrolledJ.helpViewport.YOffset)
	}
	mJK2, _ := scrolledJ.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	scrolledK := mJK2.(Model)
	if scrolledK.helpViewport.YOffset >= scrolledJ.helpViewport.YOffset {
		t.Fatalf("expected 'k' to scroll up; before=%d after=%d", scrolledJ.helpViewport.YOffset, scrolledK.helpViewport.YOffset)
	}

	// Clamp: repeated scrolling should stop at the bottom (no infinite blank scroll).
	bottom := scrolledK
	for i := 0; i < 200; i++ {
		mNext, _ := bottom.Update(tea.KeyMsg{Type: tea.KeyDown})
		bottom = mNext.(Model)
	}
	atBottom := bottom.helpViewport.YOffset
	mNext, _ := bottom.Update(tea.KeyMsg{Type: tea.KeyDown})
	bottom2 := mNext.(Model)
	if bottom2.helpViewport.YOffset != atBottom {
		t.Fatalf("expected YOffset to clamp at bottom; before=%d after=%d", atBottom, bottom2.helpViewport.YOffset)
	}

	// Close on Esc.
	m4, _ := bottom2.Update(tea.KeyMsg{Type: tea.KeyEsc})
	closed := m4.(Model)
	if closed.helpModalOpen {
		t.Fatalf("expected helpModalOpen false after Esc")
	}
}

func TestHelpModal_CtrlPDoesNotOverrideActivityCtrlP(t *testing.T) {
	m := New(context.Background(), stubRunner{final: "ok"}, make(chan events.Event))
	m.width = 120
	m.height = 30
	m.showDetails = true
	m.focus = focusActivityList
	m.single.Blur()
	m.multiline.Blur()
	m.layout()

	// Make activity list have some items so cursor can move.
	m.activities = []Activity{
		{ID: "a1", Kind: "noop", Title: "One", Status: ActivityOK},
		{ID: "a2", Kind: "noop", Title: "Two", Status: ActivityOK},
	}
	m.refreshActivityList()
	m.activityList.Select(1)

	// Ctrl+P is an Activity shortcut (cursor up). Help modal should NOT open.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	updated := m2.(Model)
	if updated.helpModalOpen {
		t.Fatalf("expected help modal not to open when activity focused")
	}
	if updated.activityList.Index() != 0 {
		t.Fatalf("expected ctrl+p to move activity cursor up to 0, got %d", updated.activityList.Index())
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
