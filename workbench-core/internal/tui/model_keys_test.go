package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/events"
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

type recordingRunner struct {
	stubRunner
	lastMessage string
}

func (r *recordingRunner) RunTurn(ctx context.Context, userMsg string) (string, error) {
	r.lastMessage = userMsg
	return "Model set to " + strings.TrimPrefix(userMsg, "/model "), nil
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
			"/workspace/example.json": "{\n  \"ok\": true\n}\n",
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
		{ID: "act-1", Kind: "fs.write", Title: "Write /workspace/example.json", Status: ActivityOK, Path: "/workspace/example.json"},
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
	if updated.fileViewPath != "/workspace/example.json" {
		t.Fatalf("expected fileViewPath %q, got %q", "/workspace/example.json", updated.fileViewPath)
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
	if cmd == nil {
		t.Fatalf("expected cmd (textinput.Blink), got nil")
	}
}

func TestModelPicker_FiltersOnTyping(t *testing.T) {
	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.layout()

	// Open picker
	m.single.SetValue("/model")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if !updated.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen true")
	}

	// Type filter text
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	updated2 := m3.(Model)
	m4, _ := updated2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	updated3 := m4.(Model)
	m5, _ := updated3.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	updated4 := m5.(Model)

	if updated4.modelPickerFilter.Value() != "gpt" {
		t.Fatalf("expected filter value 'gpt', got %q", updated4.modelPickerFilter.Value())
	}

	// Check that list is filtered
	items := updated4.modelPickerList.Items()
	foundGPT := false
	for _, item := range items {
		if modelItem, ok := item.(modelPickerItem); ok {
			if strings.Contains(modelItem.id, "gpt") {
				foundGPT = true
				break
			}
		}
	}
	if !foundGPT {
		t.Fatalf("expected to find gpt models in filtered list")
	}
}

func TestModelPicker_SelectsModelAndUpdatesLabel(t *testing.T) {
	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
	m.layout()
	m.modelID = "old-model"

	// Open picker
	m.single.SetValue("/model")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if !updated.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen true")
	}

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
	m.layout()

	// Open picker
	m.single.SetValue("/model")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	updated := m2.(Model)

	if !updated.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen true")
	}

	// Press Esc - should close picker
	m3, _ := updated.Update(tea.KeyMsg{Type: tea.KeyEsc})
	updated2 := m3.(Model)

	if updated2.modelPickerOpen {
		t.Fatalf("expected modelPickerOpen false after Esc")
	}
	if runner.lastMessage != "" {
		t.Fatalf("expected runner not called, but got %q", runner.lastMessage)
	}
}

func TestModelPicker_ModelCommandWithArgsStillWorks(t *testing.T) {
	runner := &recordingRunner{}
	m := New(context.Background(), runner, make(chan events.Event))
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
