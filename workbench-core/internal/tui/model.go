package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func cursorDebugLog(hypothesisId, location, message string, data map[string]any) {
	// #region agent log
	const logPath = "/Users/santinoonyeme/personal/dev/Projects/workbench/.cursor/debug.log"
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	payload := map[string]any{
		"sessionId":    "debug-session",
		"runId":        "pre-fix",
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	if b, err := json.Marshal(payload); err == nil {
		_, _ = f.Write(append(b, '\n'))
	}
	// #endregion
}

func New(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) Model {
	main := viewport.New(0, 0)
	// Important: avoid horizontal padding on viewports.
	//
	// If viewport content becomes wider than the terminal (due to padding + borders
	// in transcript elements), the terminal will soft-wrap long lines. That increases
	// the effective number of screen lines and can make the header appear to
	// "disappear" (scrolled off the top) on resize or when toggling the sidebar.
	main.Style = lipgloss.NewStyle()
	main.MouseWheelEnabled = true

	details := viewport.New(0, 0)
	details.Style = lipgloss.NewStyle()
	details.MouseWheelEnabled = true

	planView := viewport.New(0, 0)
	planView.Style = lipgloss.NewStyle()
	planView.MouseWheelEnabled = true

	helpVp := viewport.New(0, 0)
	helpVp.Style = lipgloss.NewStyle()
	helpVp.MouseWheelEnabled = true

	activity := list.New([]list.Item{}, newActivityDelegate(), 0, 0)
	activity.Title = ""
	activity.SetShowHelp(false)
	activity.SetShowStatusBar(false)
	activity.SetShowPagination(false)
	activity.SetFilteringEnabled(false)
	activity.SetShowFilter(false)
	activity.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#707070")).
		Bold(true)

	// Textarea focus styling:
	//
	// The default bubbles/textarea focused style uses a visible cursor-line highlight.
	// For Workbench, we want focus to affect behavior (cursor + key handling) but not
	// introduce a distinct "selected" visual treatment in the input box.
	//
	// So: use identical styles for focused + blurred, and avoid background/reverse
	// effects on the cursor line.
	plainTextAreaStyle := textarea.Style{
		Base:        lipgloss.NewStyle(),
		CursorLine:  lipgloss.NewStyle(),
		EndOfBuffer: lipgloss.NewStyle().Foreground(lipgloss.Color("#404040")),
		LineNumber:  lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),
		Prompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		Text: lipgloss.NewStyle().Foreground(lipgloss.Color("#eaeaea")),
	}

	single := textarea.New()
	single.Placeholder = "Type a message…"
	single.Focus()
	single.Prompt = ""
	single.ShowLineNumbers = false
	single.SetHeight(1)
	single.CharLimit = 0
	single.KeyMap.InsertNewline.SetEnabled(false) // Enter should submit in single-line mode.
	single.FocusedStyle = plainTextAreaStyle
	single.BlurredStyle = plainTextAreaStyle

	multi := textarea.New()
	multi.Placeholder = "Multiline message (Ctrl+O to send)…"
	multi.Prompt = "…> "
	multi.ShowLineNumbers = false
	multi.CharLimit = 0
	multi.SetHeight(6)
	// Keep prompt dimmer for multiline mode, but still avoid focus highlighting.
	multiStyle := plainTextAreaStyle
	multiStyle.Prompt = lipgloss.NewStyle().Foreground(lipgloss.Color("#707070"))
	multi.FocusedStyle = multiStyle
	multi.BlurredStyle = multiStyle

	editor := textarea.New()
	editor.Placeholder = ""
	editor.Prompt = ""
	editor.ShowLineNumbers = true
	editor.CharLimit = 0
	editor.SetHeight(1)
	editorStyle := plainTextAreaStyle
	editorStyle.Prompt = lipgloss.NewStyle()
	editor.FocusedStyle = editorStyle
	editor.BlurredStyle = editorStyle

	m := Model{
		ctx:            ctx,
		runner:         runner,
		events:         evCh,
		transcript:     main,
		activityList:   activity,
		activityDetail: details,
		planViewport:   planView,
		helpViewport:   helpVp,
		// Default: start with the activity panel closed. Users can toggle it with
		// Ctrl+A, or enable it by default via WORKBENCH_ACTIVITY/--activity.
		showDetails:         false,
		activityIndexByID:   map[string]int{},
		activityIndexByOpID: map[string]int{},
		single:              single,
		multiline:           multi,
		editorBuf:           editor,

		styleHeaderBar: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleHeaderApp: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")).
			Bold(true),
		styleHeaderMid: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleHeaderRHS: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")),

		styleDim: lipgloss.NewStyle().Foreground(lipgloss.Color("#707070")),

		styleUserLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		styleUserBox: lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleAgentBox: lipgloss.NewStyle().
			Padding(0, 1),
		styleFileChangeBox: lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#303030")),
		styleAgent: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")),
		styleAction: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0c0c0")),
		styleTelemetry: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6b6b6b")),
		styleOutcome: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8a8a8a")),
		styleError: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff5f5f")).
			Bold(true),
		styleBold: lipgloss.NewStyle().Bold(true),

		styleInputBox: lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleComposerCardFocused: lipgloss.NewStyle().
			Margin(0, 1).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#6bbcff")),
		styleComposerCardBlurred: lipgloss.NewStyle().
			Margin(0, 1).
			Padding(0, 1).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#404040")),
		styleComposerAccentFocus: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6bbcff")).
			Bold(true),
		styleComposerAccentBlur: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#404040")),
		styleComposerStatusKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		styleComposerStatusVal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#eaeaea")),
		styleHint: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#707070")),
		styleRightTabActive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ad0ff")).
			Bold(true),
		styleRightTabInactive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#707070")),

		currentActionGroupIdx: -1,
		renderer:              newContentRenderer(),
		focus:                 focusInput,
	}

	m.fileChangesItemIdx = -1
	m.lastTurnUserItemIdx = -1
	m.streamingItemIdx = -1
	m.thinkingItemIdx = -1
	m.approvalsMode = "enabled"
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.waitEvent(),
		m.prefetchPlanCmd(),
		func() tea.Msg {
			wd, err := m.runner.RunTurn(m.ctx, "/pwd")
			if err != nil {
				return preinitStatusMsg{err: err}
			}
			// /model (or /model show) is a host-side command and works pre-init.
			out, err := m.runner.RunTurn(m.ctx, "/model")
			if err != nil {
				return preinitStatusMsg{workdir: strings.TrimSpace(wd), err: err}
			}
			dd, ddErr := m.runner.RunTurn(m.ctx, "/datadir")
			// /reasoning is a host-side command and works pre-init.
			ro, _ := m.runner.RunTurn(m.ctx, "/reasoning")

			modelID := ""
			// Expected: "Current model: <id>"
			if s := strings.TrimSpace(out); s != "" {
				const pfx = "Current model:"
				if strings.HasPrefix(s, pfx) {
					modelID = strings.TrimSpace(strings.TrimPrefix(s, pfx))
				}
			}
			reasoningEffort := parseReasoningEffortFromReasoningInfo(ro)
			// DataDir is best-effort; never block the UI on it.
			_ = ddErr
			return preinitStatusMsg{
				workdir:         strings.TrimSpace(wd),
				modelID:         strings.TrimSpace(modelID),
				reasoningEffort: strings.TrimSpace(reasoningEffort),
				dataDir:         strings.TrimSpace(dd),
			}
		},
	)
}

func parseReasoningEffortFromReasoningInfo(s string) string {
	// Expected shape (best-effort):
	//   Reasoning:
	//     effort:  high
	//     summary: concise
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(strings.ToLower(t), "effort:") {
			continue
		}
		v := strings.TrimSpace(t[len("effort:"):])
		// Host prints "(default)" when unset; treat as empty so UI can show a default.
		if strings.EqualFold(v, "(default)") {
			return ""
		}
		return v
	}
	return ""
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case list.FilterMatchesMsg:
		// Critical: bubbles/list filtering runs asynchronously and sends FilterMatchesMsg
		// back through the Bubble Tea update loop. If we don't forward this message
		// into the picker list, the visible items will never update.
		if m.modelPickerOpen {
			var cmd tea.Cmd
			m.modelPickerList, cmd = m.modelPickerList.Update(msg)
			return m, cmd
		}

	case tea.KeyMsg:
		return m.updateKey(msg)

	case eventMsg:
		ev := events.Event(msg)

		// Phase 1 streaming: update in-progress agent transcript inline.
		if ev.Type == "model.token" {
			// Bug fix (duplication): ignore late buffered token events after the turn completes.
			if !m.turnInFlight {
				return m, m.waitEvent()
			}
			txt := ev.Data["text"]
			if txt != "" {
				// If the streaming item is no longer the last transcript entry, start a new one.
				if m.streamingItemIdx >= 0 {
					lastIdx := len(m.transcriptItems) - 1
					if m.streamingItemIdx != lastIdx {
						m.streamingItemIdx = -1
						m.streamingBuf = nil
					}
				}
				if m.streamingItemIdx < 0 {
					// Start a streaming agent message at the end of the transcript.
					// If the last transcript item is a Thinking block, insert a blank spacer
					// so the agent output is visually separated from the thinking summary.
					if n := len(m.transcriptItems); n != 0 && m.transcriptItems[n-1].kind == transcriptThinking {
						m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
					}
					if m.streamingBuf == nil {
						m.streamingBuf = &strings.Builder{}
					} else {
						m.streamingBuf.Reset()
					}
					m.streamingBuf.WriteString(txt)
					m.streamingItemIdx = len(m.transcriptItems)
					m.addTranscriptItem(transcriptItem{kind: transcriptAgent, text: m.streamingBuf.String() + "▌"})
				} else if m.streamingItemIdx < len(m.transcriptItems) {
					if m.streamingBuf == nil {
						// Safety: should not happen, but avoid nil deref.
						m.streamingBuf = &strings.Builder{}
					}
					m.streamingBuf.WriteString(txt)
					wasAtBottom := m.transcript.AtBottom()
					it := m.transcriptItems[m.streamingItemIdx]
					if it.kind == transcriptAgent {
						it.text = m.streamingBuf.String() + "▌"
						m.transcriptItems[m.streamingItemIdx] = it
						m.rebuildTranscript()
						if wasAtBottom {
							m.transcript.GotoBottom()
						}
					}
				}
			}
			return m, m.waitEvent()
		}

		// Phase 2 thinking: indicator + optional provider-supplied summary.
		if ev.Type == "model.thinking.start" || ev.Type == "model.thinking.summary" || ev.Type == "model.thinking.end" {
			// Ignore late buffered thinking events after the turn completes.
			// We finalize best-effort in turnDoneMsg.
			if !m.turnInFlight {
				return m, m.waitEvent()
			}
			// #region agent log
			if ev.Type == "model.thinking.start" || ev.Type == "model.thinking.end" {
				cursorDebugLog("H4", "model.go:update", "thinking_event_received", map[string]any{
					"type":           strings.TrimSpace(ev.Type),
					"step":           strings.TrimSpace(ev.Data["step"]),
					"modelID":        strings.TrimSpace(m.modelID),
					"thinkingActive": m.thinkingActive,
					"thinkingStep":   m.thinkingStep,
				})
			}
			// #endregion
			step := 0
			if v := strings.TrimSpace(ev.Data["step"]); v != "" {
				if n, err := strconv.Atoi(v); err == nil {
					step = n
				}
			}
			switch ev.Type {
			case "model.thinking.start":
				m.thinkingActive = true
				m.thinkingStep = step
				m.thinkingStarted = time.Now()
				m.thinkingDuration = 0
				m.thinkingSummary = ""
				m.thinkingItemIdx = len(m.transcriptItems)
				m.addTranscriptItem(transcriptItem{kind: transcriptThinking, text: m.formatThinkingText()})
			case "model.thinking.summary":
				if txt := ev.Data["text"]; txt != "" {
					m.thinkingSummary += txt
					m.updateThinkingTranscriptItem()
				}
			case "model.thinking.end":
				// Close out thinking for this step.
				if m.thinkingActive && !m.thinkingStarted.IsZero() {
					m.thinkingDuration = time.Since(m.thinkingStarted)
				}
				m.thinkingActive = false
				m.thinkingStep = step
				m.updateThinkingTranscriptItem()
			}
			return m, m.waitEvent()
		}

		evCmd := m.onEvent(ev)
		cmds := []tea.Cmd{m.waitEvent()}
		if evCmd != nil {
			cmds = append(cmds, evCmd)
		}
		if ev.Type == "ui.editor.open" {
			purpose := strings.TrimSpace(ev.Data["purpose"])
			if purpose == "compose" {
				if abs := strings.TrimSpace(ev.Data["absPath"]); abs != "" {
					m.externalEditorComposePath = abs
					// If we don't yet know DataDir (rare), infer it from the compose path.
					if strings.TrimSpace(m.dataDir) == "" {
						m.dataDir = strings.TrimSpace(filepath.Dir(abs))
					}
					cmds = append(cmds, m.openComposeEditor(abs))
				} else if v := strings.TrimSpace(ev.Data["vpath"]); v != "" {
					// Back-compat: older hosts used a /project compose file.
					m.externalEditorComposeVPath = v
					cmds = append(cmds, m.openEditor(v))
				}
			} else if v := strings.TrimSpace(ev.Data["vpath"]); v != "" {
				cmds = append(cmds, m.openEditor(v))
			}
		}
		return m, tea.Batch(cmds...)

	case fileAfterMsg:
		callID := strings.TrimSpace(msg.callID)
		opID := strings.TrimSpace(msg.opID)
		path := strings.TrimSpace(msg.path)
		hasApprovalRecord := callID != "" && m.hasApprovalRecord(callID)
		if callID != "" && m.approvedCallIDs != nil {
			delete(m.approvedCallIDs, callID)
		}
		if !msg.suppressDiff && hasApprovalRecord {
			msg.suppressDiff = true
		}
		if path != "" && m.approvedFileOpsByPath != nil {
			if count, ok := m.approvedFileOpsByPath[path]; ok && count > 0 {
				msg.suppressDiff = true
				remaining := count - 1
				if remaining == 0 {
					delete(m.approvedFileOpsByPath, path)
				} else {
					m.approvedFileOpsByPath[path] = remaining
				}
			}
		}
		// Best-effort: update cache and render a transcript diff/patch block.
		if path == "" || msg.err != nil {
			return m, nil
		}
		if m.fileSnapCache == nil {
			m.fileSnapCache = make(map[string]string)
		}
		// Back-compat: when opId isn't available, try matching by path.
		if opID == "" {
			opID = m.findPendingFileOpIDByPath(path)
		}
		if opID == "" || m.pendingFileOpsByOpID == nil {
			// No pending metadata; still update cache and skip transcript.
			m.fileSnapCache[path] = msg.text
			// If the file picker is open, refresh it so newly created files appear.
			if m.filePickerOpen && (strings.HasPrefix(path, "/scratch/") || strings.HasPrefix(path, "/project/")) && strings.TrimSpace(m.workdir) != "" {
				if all, err := m.scanFilePickerPaths(m.workdir); err == nil {
					m.filePickerAllPaths = all
					m.filePickerWorkdir = strings.TrimSpace(m.workdir)
					m.applyFilePickerQuery(m.filePickerQuery)
					m.layout()
				}
			}
			return m, nil
		}
		p, ok := m.pendingFileOpsByOpID[opID]
		if !ok {
			cursorDebugLog("H2", "model.go:fileAfterMsg", "pending_file_op_missing", map[string]any{
				"opId": opID,
				"path": path,
			})
			// No pending metadata; still update cache and skip transcript.
			m.fileSnapCache[path] = msg.text
			return m, nil
		}

		before := p.before
		after := msg.text
		m.fileSnapCache[path] = after
		delete(m.pendingFileOpsByOpID, opID)
		refreshFilePicker := func() {
			if m.filePickerOpen && (strings.HasPrefix(path, "/scratch/") || strings.HasPrefix(path, "/project/")) && strings.TrimSpace(m.workdir) != "" {
				if all, err := m.scanFilePickerPaths(m.workdir); err == nil {
					m.filePickerAllPaths = all
					m.filePickerWorkdir = strings.TrimSpace(m.workdir)
					m.applyFilePickerQuery(m.filePickerQuery)
					m.layout()
				}
			}
		}
		if msg.suppressDiff {
			refreshFilePicker()
			return m, nil
		}

		verb := "Updated"
		if p.op != "fs.patch" && !p.hadBefore {
			verb = "Created"
		}

		preview, truncated, added, deleted := buildFileChangePreview(p.op, path, before, after, p.hadBefore, msg.truncated, p.patchPreview, p.patchTruncated, p.patchRedacted)
		if strings.TrimSpace(preview) == "" {
			return m, nil
		}
		if truncated {
			preview = strings.TrimRight(preview, "\n") + "\n\n_(truncated)_\n"
		}
		_ = verb // verb is currently unused in the transcript snippet (diff header implies create vs update)
		displayPath := strings.TrimPrefix(path, "/")
		if strings.TrimSpace(displayPath) == "" {
			displayPath = path
		}
		header := displayPath
		if !(added == 0 && deleted == 0) {
			// No parentheses (user preference).
			header = fmt.Sprintf("%s  +%d -%d", displayPath, added, deleted)
		}
		// Keep diff close to header (no extra blank line).
		snippet := header + "\n" + preview
		if m.fileChangesByPath == nil {
			m.fileChangesByPath = make(map[string]string)
		}
		_, exists := m.fileChangesByPath[path]
		if !exists {
			m.fileChangesOrder = append(m.fileChangesOrder, path)
		}
		// If we already captured a real diff for this path, don't overwrite it with a
		// later "no changes" write (common when a tool re-writes identical content).
		if exists && added == 0 && deleted == 0 {
			// Still keep ordering stable; just skip updating the snippet body.
			return m, nil
		}
		if exists {
			// Move the path to the end so the most recently changed file is last in the
			// grouped block (best UX when the block is kept at the bottom).
			oldIdx := -1
			for i, p := range m.fileChangesOrder {
				if p == path {
					oldIdx = i
					break
				}
			}
			if oldIdx >= 0 && oldIdx != len(m.fileChangesOrder)-1 {
				// Remove oldIdx and append.
				m.fileChangesOrder = append(m.fileChangesOrder[:oldIdx], m.fileChangesOrder[oldIdx+1:]...)
				m.fileChangesOrder = append(m.fileChangesOrder, path)
			}
		}
		m.fileChangesByPath[path] = snippet
		m.upsertGroupedFileChanges()
		refreshFilePicker()
		return m, nil

	case fileBeforeMsg:
		// Best-effort: fill in missing "before" content for Created/Updated labeling
		// and diff previews (request arrives pre-exec).
		opID := strings.TrimSpace(msg.opID)
		path := strings.TrimSpace(msg.path)
		if opID == "" {
			opID = m.findPendingFileOpIDByPath(path)
		}
		if opID == "" || path == "" {
			return m, nil
		}
		if m.pendingFileOpsByOpID == nil {
			return m, nil
		}
		p, ok := m.pendingFileOpsByOpID[opID]
		if !ok || p.hadBefore {
			return m, nil
		}
		// If the read succeeded, we know the file existed and we can diff against it.
		if msg.err == nil {
			p.before = msg.text
			p.hadBefore = true
			m.pendingFileOpsByOpID[opID] = p
			if path != "" {
				m.enqueuePendingFileOp(path, opID)
			}
		}
		return m, nil

	case workdirPrefetchMsg:
		if msg.err != nil {
			// Keep picker open but empty; user can still type, or try again later.
			return m, nil
		}
		if strings.TrimSpace(msg.workdir) != "" {
			m.workdir = strings.TrimSpace(msg.workdir)
		}
		// If picker is open, populate it now that we know the workdir.
		if m.filePickerOpen && strings.TrimSpace(m.workdir) != "" {
			if all, err := m.scanFilePickerPaths(m.workdir); err == nil {
				m.filePickerAllPaths = all
				m.filePickerWorkdir = strings.TrimSpace(m.workdir)
				m.applyFilePickerQuery(m.filePickerQuery)
				m.filePickerList.Title = "Select File"
				m.layout()
			}
		}
		return m, nil

	case preinitStatusMsg:
		// Best-effort; do not surface errors as transcript output.
		if strings.TrimSpace(msg.workdir) != "" {
			m.workdir = strings.TrimSpace(msg.workdir)
		}
		if strings.TrimSpace(msg.dataDir) != "" {
			m.dataDir = strings.TrimSpace(msg.dataDir)
		}
		if strings.TrimSpace(msg.modelID) != "" {
			m.modelID = strings.TrimSpace(msg.modelID)
		}
		if strings.TrimSpace(msg.reasoningEffort) != "" {
			m.reasoningEffort = strings.TrimSpace(msg.reasoningEffort)
		}
		// If picker is open and workdir arrived, populate it.
		if m.filePickerOpen && strings.TrimSpace(m.workdir) != "" && m.filePickerWorkdir == "" {
			if all, err := m.scanFilePickerPaths(m.workdir); err == nil {
				m.filePickerAllPaths = all
				m.filePickerWorkdir = strings.TrimSpace(m.workdir)
				m.applyFilePickerQuery(m.filePickerQuery)
				if strings.Contains(m.filePickerList.Title, "loading") {
					m.filePickerList.Title = "Select File"
				}
			}
		}
		m.layout()
		return m, nil

	case editorLoadMsg:
		if strings.TrimSpace(msg.vpath) == "" || msg.vpath != m.editorVPath {
			return m, nil
		}
		if msg.err != nil {
			m.editorErr = msg.err.Error()
			m.editorBuf.SetValue("")
			m.editorDirty = false
			m.editorNotice = ""
		} else {
			m.editorErr = ""
			m.editorBuf.SetValue(msg.content)
			m.editorDirty = false
			m.editorNotice = strings.TrimSpace(msg.notice)
		}
		m.layout()
		return m, nil

	case editorSaveMsg:
		if strings.TrimSpace(msg.vpath) == "" || msg.vpath != m.editorVPath {
			return m, nil
		}
		if msg.err != nil {
			m.editorErr = msg.err.Error()
			return m, nil
		}
		m.editorErr = ""
		m.editorNotice = "saved"
		m.editorDirty = false
		return m, nil

	case editorExternalDoneMsg:
		if strings.TrimSpace(msg.vpath) == "" {
			return m, nil
		}
		if msg.err != nil {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "editor error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.rebuildTranscript()
		} else if strings.TrimSpace(m.externalEditorComposePath) != "" && msg.vpath == m.externalEditorComposePath {
			// Compose flow: load the compose buffer into the multiline input.
			m.composeLoadPath = msg.vpath
			cmd := m.loadComposeBufferAbs(msg.vpath)
			m.externalEditorComposePath = ""
			m.focus = focusInput
			m.isMulti = true
			m.multiline.Focus()
			return m, cmd
		} else if strings.TrimSpace(m.externalEditorComposeVPath) != "" && msg.vpath == m.externalEditorComposeVPath {
			// Compose flow: load the compose buffer into the multiline input.
			m.composeLoadPath = msg.vpath
			cmd := m.loadComposeBuffer(msg.vpath)
			m.externalEditorComposeVPath = ""
			m.focus = focusInput
			m.isMulti = true
			m.multiline.Focus()
			return m, cmd
		}
		m.focus = focusInput
		if m.isMulti {
			m.multiline.Focus()
		} else {
			m.single.Focus()
		}
		return m, nil

	case editorComposeLoadMsg:
		if strings.TrimSpace(msg.vpath) == "" || strings.TrimSpace(m.composeLoadPath) == "" || msg.vpath != m.composeLoadPath {
			return m, nil
		}
		m.composeLoadPath = ""
		if msg.err != nil {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "compose load error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.rebuildTranscript()
			return m, nil
		}
		m.focus = focusInput
		m.isMulti = true
		m.multiline.SetValue(msg.text)
		m.multiline.Focus()
		m.layout()
		return m, nil

	case fileViewMsg:
		// Ignore stale responses.
		if strings.TrimSpace(msg.path) != "" && msg.path != m.fileViewPath {
			return m, nil
		}
		if msg.err != nil {
			m.fileViewErr = msg.err.Error()
			m.fileViewContent = ""
			m.fileViewTruncated = false
		} else {
			m.fileViewErr = ""
			m.fileViewContent = msg.content
			m.fileViewTruncated = msg.truncated
		}
		m.refreshActivityDetail()
		return m, nil

	case planFileMsg:
		prevHadContent := strings.TrimSpace(m.planMarkdown) != ""
		m.planLoadErr = ""
		if msg.err != nil {
			m.planLoadErr = msg.err.Error()
		}
		m.planMarkdown = msg.content
		newHasContent := strings.TrimSpace(m.planMarkdown) != ""
		if !m.planAutoExpanded && !prevHadContent && newHasContent {
			m.planAutoExpanded = true
			m.planTabActive = true
			if !m.showDetails {
				m.showDetails = true
				m.focus = focusInput
				if m.isMulti {
					m.multiline.Focus()
				} else {
					m.single.Focus()
				}
			}
		}
		m.layout()
		return m, nil

	case turnDoneMsg:
		m.turnInFlight = false
		// Clear per-turn cancel state (the turn is over regardless of outcome).
		m.turnCtx = nil
		m.turnCancel = nil
		// Best-effort: finalize any in-progress thinking indicator. We may receive
		// a late model.thinking.end after turn completion, but we ignore late events
		// to prevent duplication.
		if m.thinkingItemIdx >= 0 {
			if m.thinkingActive && !m.thinkingStarted.IsZero() {
				m.thinkingDuration = time.Since(m.thinkingStarted)
			}
			m.thinkingActive = false
			m.thinkingStep = 0
			m.updateThinkingTranscriptItem()
		}
		if msg.err != nil {
			var approvalErr agent.ErrApprovalRequired
			if errors.As(msg.err, &approvalErr) {
				diffs := m.generateApprovalDiffs(approvalErr.PendingOps)
				m.startApprovalFlow(approvalErr, diffs)
				return m, nil
			}
			// Treat user-initiated stop (Ctrl+X) as a normal outcome, not an error.
			if m.turnCancelRequested || errors.Is(msg.err, context.Canceled) {
				// Finalize any in-progress streaming state.
				if m.streamingItemIdx >= 0 && m.streamingItemIdx < len(m.transcriptItems) {
					it := m.transcriptItems[m.streamingItemIdx]
					if it.kind == transcriptAgent {
						txt := ""
						if m.streamingBuf != nil {
							txt = m.streamingBuf.String()
						} else {
							txt = strings.TrimSuffix(it.text, "▌")
						}
						txt = strings.TrimRight(txt, "\n")
						if txt == "" {
							txt = "_(stopped)_"
						} else {
							txt = txt + "\n\n_(stopped)_"
						}
						it.text = txt
						m.transcriptItems[m.streamingItemIdx] = it
						m.streamingItemIdx = -1
						m.streamingBuf = nil
						wasAtBottom := m.transcript.AtBottom()
						m.rebuildTranscript()
						if wasAtBottom {
							m.transcript.GotoBottom()
						}
						m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
					}
				} else {
					// No partial output observed; still surface a minimal stop marker.
					if n := len(m.transcriptItems); n != 0 && m.transcriptItems[n-1].kind == transcriptThinking {
						m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
					}
					m.addTranscriptItem(transcriptItem{kind: transcriptAgent, text: "_(stopped)_"})
					m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
				}
				m.streamingItemIdx = -1
				m.streamingBuf = nil
				m.thinkingItemIdx = -1
				m.thinkingSummary = ""
				m.turnTitle = ""
				m.turnCancelRequested = false
				m.scrollToCurrentTurnStart()
				return m, nil
			}

			// Clear any in-progress streaming state on error.
			m.streamingItemIdx = -1
			m.streamingBuf = nil
			m.thinkingItemIdx = -1
			m.thinkingSummary = ""
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: "agent error: " + msg.err.Error()})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			m.turnTitle = ""
			m.turnCancelRequested = false
			return m, nil
		}
		finalText := strings.TrimSpace(msg.final)
		if finalText != "" {
			if m.streamingItemIdx >= 0 && m.streamingItemIdx < len(m.transcriptItems) {
				it := m.transcriptItems[m.streamingItemIdx]
				if it.kind == transcriptAgent {
					it.text = finalText
					m.transcriptItems[m.streamingItemIdx] = it
					m.streamingItemIdx = -1
					m.streamingBuf = nil
					wasAtBottom := m.transcript.AtBottom()
					m.rebuildTranscript()
					if wasAtBottom {
						m.transcript.GotoBottom()
					}
					m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
				} else {
					// Fallback: unexpected kind, append normally.
					m.streamingItemIdx = -1
					m.streamingBuf = nil
					if n := len(m.transcriptItems); n != 0 && m.transcriptItems[n-1].kind == transcriptThinking {
						m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
					}
					m.addTranscriptItem(transcriptItem{kind: transcriptAgent, text: finalText})
					m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
				}
			} else {
				if n := len(m.transcriptItems); n != 0 && m.transcriptItems[n-1].kind == transcriptThinking {
					m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
				}
				m.addTranscriptItem(transcriptItem{kind: transcriptAgent, text: finalText})
				m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			}
		} else {
			// No final output; clear streaming state.
			m.streamingItemIdx = -1
			m.streamingBuf = nil
		}
		// Anchor the viewport at the start of the turn so the user message stays visible.
		// For silent commands (e.g., model picker), preserve the current scroll position.
		if !msg.preserveScroll {
			m.scrollToCurrentTurnStart()
		}
		m.turnTitle = ""
		m.turnCancelRequested = false
		return m, nil
	}

	// Mouse wheel scrolling:
	// - Always allow scrolling the transcript.
	// - When the Activity pane is open, scroll the Details panel if the cursor is over it,
	//   otherwise scroll the transcript.
	switch msg := msg.(type) {
	case tea.MouseMsg:
		if m.helpModalOpen {
			var cmd tea.Cmd
			m.helpViewport, cmd = m.helpViewport.Update(msg)
			m.clampHelpViewport()
			return m, cmd
		}
		// If details are visible and the mouse is within the right pane, scroll details.
		if m.showDetails && m.activityList.Width() > 0 {
			leftW := m.transcript.Width
			if msg.X >= leftW {
				var cmd tea.Cmd
				if m.planTabActive {
					m.planViewport, cmd = m.planViewport.Update(msg)
				} else {
					m.activityDetail, cmd = m.activityDetail.Update(msg)
				}
				return m, cmd
			}
		}
		var cmd tea.Cmd
		m.transcript, cmd = m.transcript.Update(msg)
		return m, cmd
	}

	// Default fallthrough: keep viewport responsive.
	var cmd tea.Cmd
	m.transcript, cmd = m.transcript.Update(msg)
	return m, cmd
}

func (m *Model) startApprovalFlow(err agent.ErrApprovalRequired, diffs []string) {
	m.awaitingApprovalOps = make([]approvalOp, len(err.PendingOps))
	m.approvalTranscriptIdxs = nil
	for i := range err.PendingOps {
		callID := ""
		if i < len(err.PendingToolCallIDs) {
			callID = err.PendingToolCallIDs[i]
		}
		diff := ""
		if i < len(diffs) {
			diff = diffs[i]
		}
		m.awaitingApprovalOps[i] = approvalOp{
			Req:        err.PendingOps[i],
			ToolCallID: callID,
			Diff:       diff,
		}
	}
	m.appendApprovalTranscriptItems()
	m.layout()
}

func (m *Model) appendApprovalTranscriptItems() {
	if m == nil {
		return
	}
	m.approvalTranscriptIdxs = nil
	for _, op := range m.awaitingApprovalOps {
		reqCopy := op.Req
		item := transcriptItem{
			kind:            transcriptApprovalRequest,
			approvalOp:      &reqCopy,
			approvalDiff:    strings.TrimSpace(op.Diff),
			approvalStatus:  "pending",
			approvalPending: true,
			approvalCallID:  strings.TrimSpace(op.ToolCallID),
		}
		m.addTranscriptItem(item)
		m.approvalTranscriptIdxs = append(m.approvalTranscriptIdxs, len(m.transcriptItems)-1)
	}
}

func (m *Model) generateApprovalDiffs(ops []types.HostOpRequest) []string {
	diffs := make([]string, len(ops))
	type vfsProvider interface {
		VFS() *vfs.FS
	}
	provider, ok := m.runner.(vfsProvider)
	if !ok {
		return diffs
	}
	fs := provider.VFS()
	if fs == nil {
		return diffs
	}
	for i, op := range ops {
		if !isFileModificationOp(op.Op) {
			continue
		}
		diff, err := GeneratePendingOpDiff(fs, op)
		if err != nil {
			cursorDebugLog("H2", "model.go:generateApprovalDiffs", "pending_diff_failed", map[string]any{
				"op":   op.Op,
				"path": op.Path,
				"err":  err.Error(),
			})
			diff = fmt.Sprintf("```text\n(diff preview unavailable: %s)\n```", err)
		}
		diffs[i] = diff
	}
	return diffs
}

func isFileModificationOp(op string) bool {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case types.HostOpFSWrite, types.HostOpFSAppend, types.HostOpFSEdit, types.HostOpFSPatch:
		return true
	default:
		return false
	}
}

func (m *Model) processApproval(approve bool) tea.Cmd {
	if len(m.awaitingApprovalOps) == 0 {
		return nil
	}
	idx := -1
	if len(m.approvalTranscriptIdxs) > 0 {
		idx = m.approvalTranscriptIdxs[0]
		m.approvalTranscriptIdxs = m.approvalTranscriptIdxs[1:]
		if len(m.approvalTranscriptIdxs) == 0 {
			m.approvalTranscriptIdxs = nil
		}
	}
	status := "denied"
	if approve {
		status = "approved"
	}
	if idx >= 0 && idx < len(m.transcriptItems) {
		it := m.transcriptItems[idx]
		if it.kind == transcriptApprovalRequest {
			it.approvalStatus = status
			it.approvalPending = false
			m.transcriptItems[idx] = it
			wasAtBottom := m.transcript.AtBottom()
			m.rebuildTranscript()
			if wasAtBottom {
				m.transcript.GotoBottom()
			}
		}
	}
	ctx := m.turnCtx
	if ctx == nil {
		ctx = m.ctx
	}
	op := m.awaitingApprovalOps[0]
	if approve {
		callID := strings.TrimSpace(op.ToolCallID)
		if callID != "" {
			if m.approvedCallIDs == nil {
				m.approvedCallIDs = make(map[string]bool)
			}
			m.approvedCallIDs[callID] = true
		}
		if path := strings.TrimSpace(op.Req.Path); path != "" && isFileModificationOp(op.Req.Op) {
			if m.approvedFileOpsByPath == nil {
				m.approvedFileOpsByPath = make(map[string]int)
			}
			m.approvedFileOpsByPath[path]++
		}
	}
	m.awaitingApprovalOps = m.awaitingApprovalOps[1:]

	if approve {
		resp, err := m.runner.ExecHostOp(ctx, op.Req, op.ToolCallID)
		if err != nil {
			resp = types.HostOpResponse{Op: op.Req.Op, Ok: false, Error: err.Error()}
			m.runner.AppendToolResponse(op.ToolCallID, resp)
		}
	} else {
		resp := types.HostOpResponse{
			Op:        op.Req.Op,
			Ok:        false,
			Error:     types.CommandRejectedErrorMessage,
			ErrorCode: types.CommandRejectedErrorCode,
		}
		m.runner.AppendToolResponse(op.ToolCallID, resp)
	}

	m.layout()
	if len(m.awaitingApprovalOps) == 0 {
		return m.resumeAfterApprovals(nil)
	}
	return nil
}

func (m *Model) resumeAfterApprovals(toolOutputs []types.LLMMessage) tea.Cmd {
	if m.turnCancel != nil {
		m.turnCancel()
	}
	turnCtx, turnCancel := context.WithCancel(m.ctx)
	m.turnCtx = turnCtx
	m.turnCancel = turnCancel
	m.turnCancelRequested = false
	m.turnInFlight = true
	m.turnStarted = time.Now()
	return m.resumeTurnCmd(toolOutputs)
}

func (m Model) View() string {
	if m.editorOpen {
		return m.renderEditorView()
	}
	header := m.renderHeader()
	body := m.renderBody()
	input := m.renderInput()
	base := header + "\n" + body + "\n" + input

	// Overlay help modal if open.
	if m.helpModalOpen {
		return m.renderHelpModal(base)
	}

	// Overlay file picker modal if open.
	if m.filePickerOpen {
		return m.renderFilePicker(base)
	}

	// Overlay model picker modal if open
	if m.modelPickerOpen {
		return m.renderModelPicker(base)
	}

	return base
}

func (m *Model) appendDetails(line string) {
	_ = line
}

func (m *Model) onEvent(ev events.Event) tea.Cmd {
	rr := classifyEvent(ev)
	m.observeActivityEvent(ev)

	// Session title can come from run.started event.
	if ev.Type == "run.started" {
		if v := strings.TrimSpace(ev.Data["sessionTitle"]); v != "" {
			m.sessionTitle = v
		}
		if v := strings.TrimSpace(ev.Data["sessionId"]); v != "" {
			m.sessionID = v
		}
		if v := strings.TrimSpace(ev.Data["runId"]); v != "" {
			m.runID = v
		}
	}
	// Model identifier comes from agent.loop.start (host source of truth).
	if ev.Type == "agent.loop.start" {
		if v := strings.TrimSpace(ev.Data["model"]); v != "" {
			m.modelID = v
		}
	}
	// Model identifier can change at runtime via the host /model command.
	if ev.Type == "model.changed" {
		// #region agent log
		cursorDebugLog("H1", "model.go:onEvent", "model_changed_event", map[string]any{
			"from":        strings.TrimSpace(ev.Data["from"]),
			"to":          strings.TrimSpace(ev.Data["to"]),
			"modelBefore": strings.TrimSpace(m.modelID),
		})
		// #endregion
		if v := strings.TrimSpace(ev.Data["to"]); v != "" {
			m.modelID = v
		}
		// #region agent log
		cursorDebugLog("H1", "model.go:onEvent", "model_changed_applied", map[string]any{
			"modelAfter": strings.TrimSpace(m.modelID),
		})
		// #endregion
	}
	// Reasoning effort can change at runtime via the host /reasoning command.
	if ev.Type == "reasoning.changed" {
		if v := strings.TrimSpace(ev.Data["effort"]); v != "" {
			m.reasoningEffort = v
		}
	}
	// Web search can be toggled at runtime via the host /web command.
	if ev.Type == "web.changed" {
		m.webSearchEnabled = strings.TrimSpace(ev.Data["enabled"]) == "true"
	}
	// Approval mode can change via /approval or TUI picker.
	if ev.Type == "approvals.changed" {
		if v := strings.TrimSpace(ev.Data["mode"]); v != "" {
			m.approvalsMode = v
		}
	}
	// Fallback: /reasoning (no args) emits reasoning.info with a text block.
	if ev.Type == "reasoning.info" {
		if v := parseReasoningEffortFromReasoningInfo(ev.Data["text"]); strings.TrimSpace(v) != "" {
			m.reasoningEffort = strings.TrimSpace(v)
		}
	}
	// Workdir is discovered via host.mounted and updated via /cd at runtime.
	if ev.Type == "host.mounted" {
		if wd := strings.TrimSpace(ev.Data["/project"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "workdir.changed" {
		if wd := strings.TrimSpace(ev.Data["to"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "workdir.pwd" {
		if wd := strings.TrimSpace(ev.Data["workdir"]); wd != "" {
			m.workdir = wd
		}
	}
	if ev.Type == "ui.editor.open" {
		// Pre-run /editor includes the absolute workdir so the TUI can resolve
		// and open $VISUAL/$EDITOR without creating a run.
		if wd := strings.TrimSpace(ev.Data["workdir"]); wd != "" {
			m.workdir = wd
		}
	}

	// If a file picker is open and the workdir changed/arrived, refresh it.
	if m.filePickerOpen {
		wdNow := strings.TrimSpace(m.workdir)
		if wdNow != "" && wdNow != m.filePickerWorkdir {
			if all, err := m.scanFilePickerPaths(wdNow); err == nil {
				m.filePickerAllPaths = all
				m.filePickerWorkdir = wdNow
				m.applyFilePickerQuery(m.filePickerQuery)
				if strings.Contains(m.filePickerList.Title, "loading") {
					m.filePickerList.Title = "Select File"
				}
			}
		}
	}

	// Chrome metrics only (never rendered as transcript lines).
	switch ev.Type {
	case "llm.usage.total":
		m.lastTurnTokensIn = parseInt(ev.Data["input"])
		m.lastTurnTokensOut = parseInt(ev.Data["output"])
		m.lastTurnTokens = parseInt(ev.Data["total"])
		m.totalTokens += m.lastTurnTokens
	case "llm.cost.total":
		known := parseBool(ev.Data["known"])
		m.lastTurnCostUSD = strings.TrimSpace(ev.Data["costUsd"])
		if !known && m.lastTurnCostUSD == "" {
			m.lastTurnCostUSD = "?"
		}
		if v := strings.TrimSpace(ev.Data["costUsd"]); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				m.totalCostUSD += f
			}
		}
	case "agent.turn.complete":
		m.lastTurnDuration = strings.TrimSpace(ev.Data["duration"])
		m.lastTurnSteps = strings.TrimSpace(ev.Data["steps"])
	}

	// Chat transcript: only compact action summaries, paired request+response.
	if rr.Class != RenderAction {
		return nil
	}

	switch ev.Type {
	case "agent.op.request":
		opID := strings.TrimSpace(ev.Data["opId"])
		// Back-compat: older hosts may not emit opId; use a best-effort synthetic key.
		if opID == "" {
			opID = fmt.Sprintf("legacy-%d", time.Now().UnixNano())
		}
		op := strings.TrimSpace(ev.Data["op"])
		path := strings.TrimSpace(ev.Data["path"])
		callID := strings.TrimSpace(ev.Data["callId"])
		if callID != "" && op != "" && path != "" {
			m.setApprovalCallID(op, path, callID)
		}
		var beforeCmd tea.Cmd
		if (op == "fs.write" || op == "fs.append" || op == "fs.edit" || op == "fs.patch") && path != "" {
			if m.fileSnapCache == nil {
				m.fileSnapCache = make(map[string]string)
			}
			if m.pendingFileOpsByOpID == nil {
				m.pendingFileOpsByOpID = make(map[string]pendingFileOp)
			}
			before, ok := m.fileSnapCache[path]
			p := pendingFileOp{
				op:        op,
				path:      path,
				before:    before,
				hadBefore: ok,
			}
			if op == "fs.patch" {
				p.patchPreview = strings.TrimSpace(ev.Data["patchPreview"])
				p.patchTruncated = strings.TrimSpace(ev.Data["patchTruncated"]) == "true"
				p.patchRedacted = strings.TrimSpace(ev.Data["patchRedacted"]) == "true"
			}
			m.pendingFileOpsByOpID[opID] = p
			// Best-effort: if we don't have a cached snapshot, try reading the current
			// file BEFORE the op executes so we can label Created vs Updated correctly.
			if !p.hadBefore {
				if acc, ok := m.runner.(vfsAccessor); ok {
					beforeCmd = func() tea.Msg {
						txt, _, truncated, err := acc.ReadVFS(m.ctx, path, maxDiffBytesRead)
						return fileBeforeMsg{opID: opID, op: op, path: path, text: txt, truncated: truncated, err: err}
					}
				}
			}
		}

		txt := strings.TrimSpace(rr.Text)
		if txt == "" {
			return beforeCmd
		}
		category := actionCategory(op, strings.TrimSpace(ev.Data["toolId"]))
		groupIdx, actionIdx := m.addGroupedAction(category, groupedAction{text: txt})
		if m.pendingActionsByOpID == nil {
			m.pendingActionsByOpID = make(map[string]pendingAction)
		}
		m.pendingActionsByOpID[opID] = pendingAction{groupIdx: groupIdx, actionIdx: actionIdx}
		return beforeCmd
	case "agent.op.response":
		op := strings.TrimSpace(ev.Data["op"])
		path := strings.TrimSpace(ev.Data["path"])
		callID := strings.TrimSpace(ev.Data["callId"])

		rawOpID := strings.TrimSpace(ev.Data["opId"])
		opID := rawOpID
		if path != "" && isFileModificationOp(op) {
			opID = m.resolvePendingFileOpID(path, rawOpID)
		}

		status, isError := actionStatusIcon(ev.Data)
		statusHandled := false
		if opID != "" && m.pendingActionsByOpID != nil {
			if pa, ok := m.pendingActionsByOpID[opID]; ok {
				statusHandled = m.markGroupedActionCompleted(pa.groupIdx, pa.actionIdx, status, isError)
				delete(m.pendingActionsByOpID, opID)
			}
		}
		if !statusHandled {
			for i := len(m.transcriptItems) - 1; i >= 0 && !statusHandled; i-- {
				it := m.transcriptItems[i]
				if it.kind != transcriptActionGroup {
					continue
				}
				for j := len(it.groupItems) - 1; j >= 0; j-- {
					if it.groupItems[j].status != "" {
						continue
					}
					statusHandled = m.markGroupedActionCompleted(i, j, status, isError)
					if statusHandled {
						break
					}
				}
				if statusHandled {
					break
				}
			}
		}

		// If this was a successful file op, read the resulting file content so we can
		// render a diff/patch preview in the transcript.
		// If the host provided a patchPreview (diff), attach it to the pending op so
		// buildFileChangePreview can use it instead of racing on before snapshots.
		if opID != "" && m.pendingFileOpsByOpID != nil {
			if p, ok := m.pendingFileOpsByOpID[opID]; ok {
				if pv := strings.TrimSpace(ev.Data["patchPreview"]); pv != "" {
					p.patchPreview = pv
					p.patchTruncated = strings.TrimSpace(ev.Data["patchTruncated"]) == "true"
					p.patchRedacted = strings.TrimSpace(ev.Data["patchRedacted"]) == "true"
					m.pendingFileOpsByOpID[opID] = p
				}
			}
		}
		if strings.EqualFold(strings.TrimSpace(path), planVPath) && strings.TrimSpace(ev.Data["ok"]) == "true" {
			if acc, ok := m.runner.(vfsAccessor); ok {
				return func() tea.Msg {
					txt, _, _, err := acc.ReadVFS(m.ctx, path, planMaxBytes)
					return planFileMsg{path: path, content: txt, err: err}
				}
			}
			return nil
		}
		if (op == "fs.write" || op == "fs.append" || op == "fs.edit" || op == "fs.patch") && strings.TrimSpace(ev.Data["ok"]) == "true" && path != "" {
			if acc, ok := m.runner.(vfsAccessor); ok {
				return func() tea.Msg {
					txt, _, truncated, err := acc.ReadVFS(m.ctx, path, maxDiffBytesRead)
					suppress := callID != "" && m.approvedCallIDs != nil && m.approvedCallIDs[callID]
					return fileAfterMsg{opID: opID, op: op, path: path, text: txt, truncated: truncated, err: err, callID: callID, suppressDiff: suppress}
				}
			}
		}
		return nil
	default:
		txt := strings.TrimSpace(rr.Text)
		if txt == "" {
			return nil
		}
		// Host-side command errors should appear as errors in the transcript.
		if ev.Type == "workdir.error" {
			m.addTranscriptItem(transcriptItem{kind: transcriptError, text: txt})
			m.addTranscriptItem(transcriptItem{kind: transcriptSpacer})
			return nil
		}
		status := ""
		isError := false
		if ev.Type == "refs.ambiguous" || ev.Type == "refs.unresolved" {
			status = "✗"
			isError = true
		}
		m.addGroupedAction("Action", groupedAction{text: txt, status: status, isError: isError})
		return nil
	}
}

func (m *Model) upsertGroupedFileChanges() {
	if m == nil {
		return
	}
	if len(m.fileChangesOrder) == 0 || m.fileChangesByPath == nil {
		return
	}

	// Build markdown for the grouped diff block.
	//
	// UX: if there is only one file involved, render just that snippet (no big header).
	md := ""
	if len(m.fileChangesOrder) == 1 {
		md = strings.TrimSpace(m.fileChangesByPath[m.fileChangesOrder[0]])
	} else {
		var b strings.Builder
		b.WriteString("## File changes\n\n")
		for i, p := range m.fileChangesOrder {
			snippet := strings.TrimSpace(m.fileChangesByPath[p])
			if snippet == "" {
				continue
			}
			if i != 0 {
				b.WriteString("\n---\n\n")
			}
			b.WriteString(snippet)
			b.WriteString("\n")
		}
		md = strings.TrimSpace(b.String())
	}
	if md == "" {
		return
	}

	wasAtBottom := m.transcript.AtBottom()
	// First insert: append a single file-change box at the end of the transcript.
	if m.fileChangesItemIdx < 0 || m.fileChangesItemIdx >= len(m.transcriptItems) {
		m.fileChangesItemIdx = len(m.transcriptItems)
		m.addTranscriptItem(transcriptItem{kind: transcriptFileChange, text: md})
		// Do not add a spacer here: in batch parallelism additional action lines may still
		// arrive, and we want this block to be easy to keep updated in-place.
		return
	}

	it := m.transcriptItems[m.fileChangesItemIdx]
	if it.kind != transcriptFileChange {
		// Safety: if the slot was overwritten, fall back to appending a new block.
		m.fileChangesItemIdx = len(m.transcriptItems)
		m.addTranscriptItem(transcriptItem{kind: transcriptFileChange, text: md})
		return
	}

	// If the grouped file-changes block is no longer the last transcript item, updating it
	// in-place makes the diffs appear to "update up-thread" (the user has to scroll up).
	// Instead, keep the newest version at the bottom by re-appending and turning the old
	// block into a spacer (preserves indices for other in-flight transcript items).
	if m.fileChangesItemIdx != len(m.transcriptItems)-1 {
		m.transcriptItems[m.fileChangesItemIdx] = transcriptItem{kind: transcriptSpacer}
		m.fileChangesItemIdx = len(m.transcriptItems)
		m.addTranscriptItem(transcriptItem{kind: transcriptFileChange, text: md})
		return
	}
	it.text = md
	m.transcriptItems[m.fileChangesItemIdx] = it
	m.rebuildTranscript()
	if wasAtBottom {
		m.transcript.GotoBottom()
	}
}

func (m *Model) resetActionGroupState() {
	if m == nil {
		return
	}
	m.currentActionGroupIdx = -1
	m.currentActionCategory = ""
}

func (m *Model) addGroupedAction(category string, action groupedAction) (int, int) {
	if m == nil {
		return -1, -1
	}
	category = strings.TrimSpace(category)
	if category == "" {
		category = "Action"
	}
	if m.currentActionGroupIdx >= 0 && m.currentActionCategory == category {
		return m.appendToCurrentActionGroup(action)
	}
	return m.startNewActionGroup(category, action)
}

func (m *Model) appendToCurrentActionGroup(action groupedAction) (int, int) {
	if m == nil {
		return -1, -1
	}
	if m.currentActionGroupIdx < 0 || m.currentActionGroupIdx >= len(m.transcriptItems) {
		return m.startNewActionGroup(m.currentActionCategory, action)
	}
	it := m.transcriptItems[m.currentActionGroupIdx]
	it.groupItems = append(it.groupItems, action)
	actionIdx := len(it.groupItems) - 1
	m.transcriptItems[m.currentActionGroupIdx] = it
	wasAtBottom := m.transcript.AtBottom()
	m.rebuildTranscript()
	if wasAtBottom {
		m.transcript.GotoBottom()
	}
	return m.currentActionGroupIdx, actionIdx
}

func (m *Model) startNewActionGroup(category string, action groupedAction) (int, int) {
	if m == nil {
		return -1, -1
	}
	m.resetActionGroupState()
	m.currentActionCategory = category
	it := transcriptItem{
		kind:        transcriptActionGroup,
		groupHeader: category,
		groupItems:  []groupedAction{action},
	}
	m.addTranscriptItem(it)
	idx := len(m.transcriptItems) - 1
	m.currentActionGroupIdx = idx
	return idx, 0
}

func (m *Model) markGroupedActionCompleted(groupIdx, actionIdx int, status string, isError bool) bool {
	if m == nil {
		return false
	}
	if groupIdx < 0 || groupIdx >= len(m.transcriptItems) {
		return false
	}
	it := m.transcriptItems[groupIdx]
	if it.kind != transcriptActionGroup || actionIdx < 0 || actionIdx >= len(it.groupItems) {
		return false
	}
	item := it.groupItems[actionIdx]
	item.status = status
	item.isError = isError
	it.groupItems[actionIdx] = item
	m.transcriptItems[groupIdx] = it
	wasAtBottom := m.transcript.AtBottom()
	m.rebuildTranscript()
	if wasAtBottom {
		m.transcript.GotoBottom()
	}
	return true
}

func (m *Model) hasApprovalRecord(callID string) bool {
	if m == nil {
		return false
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	for i := len(m.transcriptItems) - 1; i >= 0; i-- {
		it := m.transcriptItems[i]
		if it.kind != transcriptApprovalRequest {
			continue
		}
		if strings.TrimSpace(it.approvalCallID) == callID {
			return true
		}
	}
	return false
}

func (m *Model) setApprovalCallID(op, path, callID string) {
	if m == nil {
		return
	}
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)
	callID = strings.TrimSpace(callID)
	if op == "" || path == "" || callID == "" {
		return
	}

	// Update awaiting approvals so approval/denial events carry the correct callId.
	for i := range m.awaitingApprovalOps {
		req := &m.awaitingApprovalOps[i]
		if strings.TrimSpace(req.ToolCallID) != "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(req.Req.Op), op) && strings.TrimSpace(req.Req.Path) == path {
			req.ToolCallID = callID
			break
		}
	}

	// Update transcript approval items so suppression can match by callId.
	updated := false
	for i := range m.transcriptItems {
		it := m.transcriptItems[i]
		if it.kind != transcriptApprovalRequest || strings.TrimSpace(it.approvalCallID) != "" {
			continue
		}
		if it.approvalOp == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(it.approvalOp.Op), op) && strings.TrimSpace(it.approvalOp.Path) == path {
			it.approvalCallID = callID
			m.transcriptItems[i] = it
			updated = true
		}
	}
	if updated {
		wasAtBottom := m.transcript.AtBottom()
		m.rebuildTranscript()
		if wasAtBottom {
			m.transcript.GotoBottom()
		}
	}
}

func (m *Model) findPendingFileOpIDByPath(path string) string {
	if m == nil {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" || m.pendingFileOpsByOpID == nil {
		return ""
	}
	if m.pendingFileOpsQueue != nil {
		if q, ok := m.pendingFileOpsQueue[path]; ok && len(q) > 0 {
			return q[0]
		}
	}
	for id, p := range m.pendingFileOpsByOpID {
		if strings.TrimSpace(p.path) == path {
			return id
		}
	}
	return ""
}

func (m *Model) enqueuePendingFileOp(path, opID string) {
	if m == nil {
		return
	}
	path = strings.TrimSpace(path)
	opID = strings.TrimSpace(opID)
	if path == "" || opID == "" {
		return
	}
	if m.pendingFileOpsQueue == nil {
		m.pendingFileOpsQueue = make(map[string][]string)
	}
	m.pendingFileOpsQueue[path] = append(m.pendingFileOpsQueue[path], opID)
}

func (m *Model) popPendingFileOpID(path string) string {
	if m == nil {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" || m.pendingFileOpsQueue == nil {
		return ""
	}
	q, ok := m.pendingFileOpsQueue[path]
	if !ok || len(q) == 0 {
		delete(m.pendingFileOpsQueue, path)
		return ""
	}
	opID := q[0]
	if len(q) == 1 {
		delete(m.pendingFileOpsQueue, path)
	} else {
		m.pendingFileOpsQueue[path] = q[1:]
	}
	return opID
}

func (m *Model) removePendingFileOpFromQueue(path, opID string) {
	if m == nil {
		return
	}
	path = strings.TrimSpace(path)
	opID = strings.TrimSpace(opID)
	if path == "" || opID == "" || m.pendingFileOpsQueue == nil {
		return
	}
	q, ok := m.pendingFileOpsQueue[path]
	if !ok || len(q) == 0 {
		return
	}
	for i, entry := range q {
		if entry == opID {
			q = append(q[:i], q[i+1:]...)
			if len(q) == 0 {
				delete(m.pendingFileOpsQueue, path)
			} else {
				m.pendingFileOpsQueue[path] = q
			}
			return
		}
	}
}

func (m *Model) resolvePendingFileOpID(path, opID string) string {
	path = strings.TrimSpace(path)
	opID = strings.TrimSpace(opID)
	if path == "" {
		return opID
	}
	if opID != "" {
		m.removePendingFileOpFromQueue(path, opID)
		return opID
	}
	return m.popPendingFileOpID(path)
}

func (m Model) waitEvent() tea.Cmd {
	if m.events == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-m.events
		if !ok {
			return nil
		}
		return eventMsg(ev)
	}
}

func (m Model) prefetchPlanCmd() tea.Cmd {
	acc, ok := m.runner.(vfsAccessor)
	if !ok {
		return nil
	}
	return func() tea.Msg {
		txt, _, _, err := acc.ReadVFS(m.ctx, planVPath, planMaxBytes)
		return planFileMsg{path: planVPath, content: txt, err: err}
	}
}

// Run starts the Workbench Bubble Tea program.
func Run(ctx context.Context, runner TurnRunner, evCh <-chan events.Event) error {
	if runner == nil {
		return fmt.Errorf("tui runner is required")
	}
	m := New(ctx, runner, evCh)

	// Activity panel: off by default. Enable via:
	//   - env: WORKBENCH_ACTIVITY=true/false
	//   - flag: --activity (wired in cmd/workbench)
	enableActivity := strings.TrimSpace(os.Getenv("WORKBENCH_ACTIVITY"))
	if enableActivity != "" {
		m.showDetails = enableActivity == "1" || strings.EqualFold(enableActivity, "true") || strings.EqualFold(enableActivity, "yes")
	}

	// Mouse capture enables mouse wheel / trackpad scrolling in the transcript.
	//
	// Note: enabling xterm mouse tracking often disables native terminal click+drag
	// selection unless your terminal supports shift-drag selection. Workbench defaults
	// to mouse scrolling; set WORKBENCH_MOUSE=false (or --mouse=false) to restore
	// native selection behavior.
	//
	// Mouse mode is opt-in via:
	//   - env: WORKBENCH_MOUSE=true/false
	//   - flag: --mouse (wired in cmd/workbench)
	enableMouse := strings.TrimSpace(os.Getenv("WORKBENCH_MOUSE"))
	mouseOn := true
	if enableMouse != "" {
		mouseOn = enableMouse == "1" || strings.EqualFold(enableMouse, "true") || strings.EqualFold(enableMouse, "yes")
	}

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if mouseOn {
		// Cell motion is enough for wheel/trackpad scrolling and reduces event spam
		// compared to "all motion".
		opts = append(opts, tea.WithMouseCellMotion())
	}

	p := tea.NewProgram(m, opts...)
	finalModel, err := p.Run()
	if err == nil {
		// Bubble Tea ctrl+c is handled as a keybinding above and triggers tea.Quit,
		// which yields a nil error. Surface an interrupt sentinel so callers can
		// react (e.g., print resume commands).
		if fm, ok := finalModel.(Model); ok && fm.quitByCtrlC {
			return tea.ErrInterrupted
		}
	}
	return err
}
