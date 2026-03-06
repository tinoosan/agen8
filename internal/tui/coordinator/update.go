package coordinator

import (
	"strconv"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/agen8/internal/tui/adapter"
	"github.com/tinoosan/agen8/internal/tui/kit"
	"github.com/tinoosan/agen8/internal/tui/modelpicker"
	"github.com/tinoosan/agen8/internal/tui/rpcscope"
	"github.com/tinoosan/agen8/pkg/types"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.modelPicker.IsOpen() {
		pickerCmd, event := m.modelPicker.Update(msg)
		switch event.Type {
		case modelpicker.EventModelSelected:
			setCmd := setModelCmd(m.endpoint, m.sessionID, event.ModelID)
			if pickerCmd != nil {
				return m, tea.Batch(pickerCmd, setCmd)
			}
			return m, setCmd
		case modelpicker.EventError:
			if event.Err != nil {
				m.setFeedback("model picker error: "+event.Err.Error(), feedbackErr)
			}
			// Keep modal open so user can retry with another query/provider.
			if pickerCmd != nil {
				return m, pickerCmd
			}
		case modelpicker.EventClosed:
			if pickerCmd != nil {
				return m, pickerCmd
			}
		default:
			if pickerCmd != nil {
				return m, pickerCmd
			}
		}
		if _, ok := msg.(tea.KeyMsg); ok {
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(12, m.width-30)
		m.modelPicker.SetSize(m.width, m.height)
		return m, nil

	case animTickMsg:
		m.spinFrame = (m.spinFrame + 1) % len(kit.SpinnerFrames)
		m.flushPendingLiveEntries()
		return m, animTickCmd()

	case tickMsg:
		if m.feedback != "" && time.Since(m.feedbackAt) > 3*time.Second {
			m.feedback = ""
		}
		m.expireAgentStatus()
		return m, tea.Batch(
			fetchSessionCmd(m.endpoint, m.sessionID),
			fetchActivityCmd(m.endpoint, m.sessionID),
			fetchThinkingEventsCmd(m.endpoint, m.runID, m.lastEventSeq),
			tickCmd(),
		)

	case adapter.NotificationConnErrorMsg:
		return m, tea.Sequence(
			tea.Tick(2*time.Second, func(time.Time) tea.Msg { return reconnectNotificationMsg{} }),
		)

	case reconnectNotificationMsg:
		return m, tea.Batch(
			fetchActivityCmd(m.endpoint, m.sessionID),
			adapter.StartNotificationListenerCmd(m.endpoint),
		)

	case adapter.EventPushedMsg:
		if !m.isRunInScope(msg.Record.RunID) {
			// In team mode, events from child runs should still trigger an activity refresh.
			if strings.TrimSpace(m.teamID) != "" {
				return m, tea.Batch(
					fetchActivityCmd(m.endpoint, m.sessionID),
					adapter.WaitForNextNotificationCmd(msg.Ch, msg.ErrCh),
				)
			}
			return m, adapter.WaitForNextNotificationCmd(msg.Ch, msg.ErrCh)
		}
		act, ok := adapter.EventRecordToActivity(msg.Record)
		if !ok {
			return m, tea.Batch(
				fetchActivityCmd(m.endpoint, m.sessionID),
				adapter.WaitForNextNotificationCmd(msg.Ch, msg.ErrCh),
			)
		}

		if entry := activityToFeedEntry(act); entry != nil {
			// Avoid showing the same summary twice: one task-response per sourceID (taskId).
			if !entry.isTaskResponse || !m.feedAlreadyHasTaskResponse(entry.sourceID) {
				m.pendingLiveEntries = append(m.pendingLiveEntries, *entry)
			}
		}
		return m, adapter.WaitForNextNotificationCmd(msg.Ch, msg.ErrCh)

	case sessionLoadedMsg:
		if msg.err != nil {
			m.connected = false
			m.lastErr = msg.err.Error()
			m.setFeedback(msg.err.Error(), feedbackErr)
			return m, nil
		}
		m.connected = true
		m.lastErr = ""

		// Check if this is the very first connection initialization
		initialConnect := m.runID == "" && m.threadID == ""

		m.sessionMode = msg.sessionMode
		m.teamID = msg.teamID
		m.runID = msg.runID
		m.threadID = msg.threadID
		m.coordinatorRole = msg.coordinatorRole

		if initialConnect && m.runID != "" && m.threadID != "" {
			// Trigger immediate loads to bypass the 1s tick delay on startup
			return m, fetchThinkingEventsCmd(m.endpoint, m.runID, m.lastEventSeq)
		}
		return m, nil

	case activityLoadedMsg:
		if msg.err != nil {
			m.connected = false
			m.lastErr = msg.err.Error()
			m.setFeedback(msg.err.Error(), feedbackErr)
			return m, nil
		}
		m.connected = true
		m.lastErr = ""
		m.mergeActivityEntries(msg.entries)
		m.deriveAgentStatus()
		return m, nil

	case thinkingEventsMsg:
		if msg.err == nil {
			m.processThinkingEvents(msg.events)
			if len(msg.entries) > 0 {
				m.mergeThinkingEntries(msg.entries)
			}
		}
		if msg.lastSeq > m.lastEventSeq {
			m.lastEventSeq = msg.lastSeq
		}
		// If we got a full page, fetch the next page immediately to drain
		// historical events on re-entry instead of waiting for the next tick.
		if len(msg.events)+len(msg.entries) >= 500 {
			return m, fetchThinkingEventsCmd(m.endpoint, m.runID, m.lastEventSeq)
		}
		return m, nil

	case modelSetMsg:
		if msg.err != nil {
			m.setFeedback("model set failed: "+msg.err.Error(), feedbackErr)
			return m, nil
		}
		switch {
		case !msg.accepted:
			m.setFeedback("model change rejected for "+msg.model, feedbackErr)
		case msg.applied > 0:
			m.setFeedback("model applied to "+strconv.Itoa(msg.applied)+" run(s): "+msg.model, feedbackOK)
		default:
			m.setFeedback("model change queued: "+msg.model, feedbackInfo)
		}
		return m, tea.Batch(
			fetchSessionCmd(m.endpoint, m.sessionID),
			fetchActivityCmd(m.endpoint, m.sessionID),
			fetchThinkingEventsCmd(m.endpoint, m.runID, m.lastEventSeq),
		)

	case goalSubmittedMsg:
		if msg.err != nil {
			m.setFeedback(msg.err.Error(), feedbackErr)
			return m, nil
		}
		m.applyRecoveredScope(msg.scope)
		m.appendReconnectNotice(msg.recovered)
		// Don't add a local user entry here. The daemon emits a user_message
		// event synchronously before task.create returns, so the event.append
		// notification arrives almost immediately and brings the entry in via
		// the normal event path — avoiding a brief duplicate that would otherwise
		// appear until the next poll collapsed the two sources.
		m.setFeedback("queued", feedbackOK)
		m.pinFeedToBottom()
		return m, nil

	case sessionActionMsg:
		if msg.err != nil {
			m.setFeedback(msg.err.Error(), feedbackErr)
			return m, nil
		}
		m.applyRecoveredScope(msg.scope)
		text := "Session " + msg.action + "d"
		if msg.action == "stop" {
			text = "Session stopped"
		}
		m.feed = append(m.feed, feedEntry{
			kind:      feedSystem,
			timestamp: time.Now(),
			text:      text,
		})
		m.feedGen++
		m.appendReconnectNotice(msg.recovered)
		m.setFeedback(text, feedbackOK)
		m.pinFeedToBottom()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.input.SetValue("")
			return m, nil
		case "up":
			if strings.TrimSpace(m.input.Value()) == "" {
				m.liveFollow = false
				m.feedScroll--
				if m.feedScroll < 0 {
					m.feedScroll = 0
				}
				return m, nil
			}
		case "down":
			if strings.TrimSpace(m.input.Value()) == "" {
				m.feedScroll++
				maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
				if m.feedScroll >= maxScroll {
					m.liveFollow = true
					m.feedScroll = maxScroll
				}
				return m, nil
			}
		case "pgup", "shift+up", "ctrl+u":
			m.liveFollow = false
			m.feedScroll -= max(1, m.feedHeight()/2)
			if m.feedScroll < 0 {
				m.feedScroll = 0
			}
			return m, nil
		case "pgdown", "shift+down", "ctrl+d":
			m.feedScroll += max(1, m.feedHeight()/2)
			maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
			if m.feedScroll >= maxScroll {
				m.liveFollow = true
				m.feedScroll = maxScroll
			}
			return m, nil
		case "home":
			m.liveFollow = false
			m.feedScroll = 0
			return m, nil
		case "g":
			if strings.TrimSpace(m.input.Value()) == "" {
				m.liveFollow = false
				m.feedScroll = 0
				return m, nil
			}
		case "end":
			m.liveFollow = true
			m.pinFeedToBottom()
			return m, nil
		case "G":
			if strings.TrimSpace(m.input.Value()) == "" {
				m.liveFollow = true
				m.pinFeedToBottom()
				return m, nil
			}
		case "ctrl+o":
			// Toggle all thinking blocks expanded / collapsed globally.
			m.thinkingExpanded = !m.thinkingExpanded
			return m, nil
		case "ctrl+e":
			// Toggle inline diff display.
			m.hideDiffs = !m.hideDiffs
			m.feedGen++
			maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
			if m.feedScroll > maxScroll {
				m.feedScroll = maxScroll
			}
			return m, nil
		case "enter":
			line := strings.TrimSpace(m.input.Value())
			if line == "" {
				return m, nil
			}
			m.input.SetValue("")
			if strings.HasPrefix(line, "/") {
				return m, m.handleSlash(line)
			}
			return m, submitGoalCmd(m.endpoint, m.sessionID, m.teamID, m.runID, m.coordinatorRole, line)
		}
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case tea.MouseMsg:
		now := time.Now()
		if time.Since(m.lastWheelEvent) < 15*time.Millisecond {
			return m, nil
		}
		m.lastWheelEvent = now

		if msg.Type == tea.MouseWheelUp {
			m.liveFollow = false
			m.feedScroll -= 1
			if m.feedScroll < 0 {
				m.feedScroll = 0
			}
			return m, nil
		} else if msg.Type == tea.MouseWheelDown {
			m.feedScroll += 1
			maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
			if m.feedScroll >= maxScroll {
				m.liveFollow = true
				m.feedScroll = maxScroll
			}
			return m, nil
		}

	}

	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) handleSlash(line string) tea.Cmd {
	line = strings.TrimSpace(line)
	cmd := strings.ToLower(line)
	parts := strings.Fields(line)
	base := ""
	if len(parts) > 0 {
		base = strings.ToLower(strings.TrimSpace(parts[0]))
	}
	switch cmd {
	case "/pause":
		return sessionActionCmd(m.endpoint, m.sessionID, m.teamID, "pause")
	case "/resume":
		return sessionActionCmd(m.endpoint, m.sessionID, m.teamID, "resume")
	case "/stop":
		return sessionActionCmd(m.endpoint, m.sessionID, m.teamID, "stop")
	case "/diffs":
		m.hideDiffs = !m.hideDiffs
		m.feedGen++
		maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
		if m.feedScroll > maxScroll {
			m.feedScroll = maxScroll
		}
		return nil
	case "/help":
		m.setFeedback("commands: /model /pause /resume /stop /diffs /help /quit", feedbackInfo)
		return nil
	case "/quit":
		return tea.Quit
	}
	switch base {
	case "/model":
		if len(parts) == 1 {
			return m.modelPicker.Open(m.endpoint, m.sessionID)
		}
		model := strings.TrimSpace(strings.Join(parts[1:], " "))
		return setModelCmd(m.endpoint, m.sessionID, model)
	default:
		m.setFeedback("unknown command: "+line, feedbackErr)
		return nil
	}
}

func (m *Model) setFeedback(msg string, kind int) {
	m.feedback = strings.TrimSpace(msg)
	m.feedbackKind = kind
	m.feedbackAt = time.Now()
}

func taskResponseKeyFromEntry(e feedEntry) string {
	if !e.isTaskResponse {
		return ""
	}
	return strings.TrimSpace(e.sourceID)
}

func (m *Model) feedAlreadyHasTaskResponse(sourceID string) bool {
	target := strings.TrimSpace(sourceID)
	if target == "" {
		return false
	}
	for _, e := range m.feed {
		if taskResponseKeyFromEntry(e) == target {
			return true
		}
	}
	return false
}

// upsertFeedEntry adds a single entry from the streaming path.
// Unlike mergeActivityEntries it does NOT wipe existing tool-op entries — it
// either updates the entry in-place (matched by kind + sourceID) or appends it.
func (m *Model) upsertFeedEntry(e feedEntry) {
	oldLines := 0
	if !m.liveFollow {
		oldLines = m.totalFeedLines()
	}
	if !m.applyLiveFeedEntry(e) {
		return
	}
	m.feedGen++
	if m.liveFollow {
		m.pinFeedToBottom()
	} else {
		newLines := m.totalFeedLines()
		if newLines > oldLines {
			m.feedScroll += (newLines - oldLines)
		}
		maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
		if m.feedScroll > maxScroll {
			m.feedScroll = maxScroll
		}
	}
}

// applyLiveFeedEntry applies a live streaming entry in-place or appends it.
// Bridge ops are absorbed into their parent code_exec entry at the data layer
// so they never appear as independent feed entries.
// Returns true only when the rendered output would change.
func (m *Model) applyLiveFeedEntry(e feedEntry) bool {
	normalizeFeedEntry(&e)

	// Bridge ops are absorbed into their parent code_exec rather than
	// being added as separate feed entries. This prevents role-mismatch
	// flickering and enables live-ticking tool counters.
	if isBridgeOp(e) {
		if idx := m.findParentCodeExec(e); idx >= 0 {
			old := m.feed[idx]
			absorbBridgeOp(&m.feed[idx], e)
			return feedEntryRenderChanged(old, m.feed[idx])
		}
	}

	if key := strings.TrimSpace(e.identityKey); key != "" {
		for i := range m.feed {
			if strings.TrimSpace(m.feed[i].identityKey) == key {
				changed := feedEntryRenderChanged(m.feed[i], e)
				m.feed[i] = e
				sort.SliceStable(m.feed, func(a, b int) bool {
					return m.feed[a].timestamp.Before(m.feed[b].timestamp)
				})
				return changed
			}
		}
	}
	if sid := strings.TrimSpace(e.sourceID); sid != "" {
		normSID := normalizeOpSourceID(sid)
		for i := range m.feed {
			existingSID := normalizeOpSourceID(strings.TrimSpace(m.feed[i].sourceID))
			if m.feed[i].kind == e.kind && existingSID == normSID {
				changed := feedEntryRenderChanged(m.feed[i], e)
				m.feed[i] = e
				sort.SliceStable(m.feed, func(a, b int) bool {
					return m.feed[a].timestamp.Before(m.feed[b].timestamp)
				})
				return changed
			}
		}
	}
	m.feed = append(m.feed, e)
	sort.SliceStable(m.feed, func(i, j int) bool {
		return m.feed[i].timestamp.Before(m.feed[j].timestamp)
	})
	return true
}

// findParentCodeExec returns the index of the most recent code_exec entry
// in m.feed that temporally contains the given bridge op, or -1 if none.
func (m *Model) findParentCodeExec(bridge feedEntry) int {
	for i := len(m.feed) - 1; i >= 0; i-- {
		e := m.feed[i]
		if strings.ToLower(strings.TrimSpace(e.opKind)) != "code_exec" {
			continue
		}
		if e.timestamp.After(bridge.timestamp) {
			continue
		}
		if !e.finishedAt.IsZero() && bridge.timestamp.After(e.finishedAt) {
			continue
		}
		return i
	}
	return -1
}

func (m *Model) flushPendingLiveEntries() {
	if len(m.pendingLiveEntries) == 0 {
		return
	}
	oldLines := 0
	if !m.liveFollow {
		oldLines = m.totalFeedLines()
	}
	changed := false
	for _, e := range m.pendingLiveEntries {
		if m.applyLiveFeedEntry(e) {
			changed = true
		}
	}
	m.pendingLiveEntries = m.pendingLiveEntries[:0]
	if !changed {
		return
	}
	m.feedGen++
	m.deriveAgentStatus()
	if m.liveFollow {
		m.pinFeedToBottom()
	} else {
		newLines := m.totalFeedLines()
		if newLines > oldLines {
			m.feedScroll += (newLines - oldLines)
		}
		maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
		if m.feedScroll > maxScroll {
			m.feedScroll = maxScroll
		}
	}
}

func (m *Model) mergeActivityEntries(entries []feedEntry) {
	if len(entries) == 0 {
		return
	}
	oldLines := 0
	if !m.liveFollow {
		oldLines = m.totalFeedLines()
	}

	// Keep thinking entries and agent text/task-response entries across polls so
	// they are not dropped and re-added (which would bump feedGen and cause
	// flickering).  Tool-op entries are intentionally excluded: live-streamed
	// versions carry different sourceIDs than their polled counterparts, so
	// retaining them prevents the dedup from collapsing the two and produces
	// duplicate rows.  User/system entries have no stable identity key either,
	// so keeping them would also cause duplicates.
	others := make([]feedEntry, 0, len(m.feed))
	for _, e := range m.feed {
		if e.kind == feedThinking || (e.kind == feedAgent && (e.isText || e.isTaskResponse)) {
			others = append(others, *normalizeFeedEntry(&e))
		}
	}
	// Dedupe task-response (summary) entries: only one per sourceID so the summary is not shown twice.
	filtered := make([]feedEntry, 0, len(entries))
	seenTaskResponse := make(map[string]bool)
	for _, e := range entries {
		normalizeFeedEntry(&e)
		if e.isTaskResponse {
			sid := strings.TrimSpace(e.sourceID)
			if sid != "" && (m.feedAlreadyHasTaskResponse(sid) || seenTaskResponse[sid]) {
				continue
			}
			if sid != "" {
				seenTaskResponse[sid] = true
			}
		}
		filtered = append(filtered, e)
	}
	if len(filtered) == 0 {
		return
	}
	merged := append(others, filtered...)
	merged = dedupeFeedEntriesByIdentity(merged)
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].timestamp.Before(merged[j].timestamp)
	})
	if !feedEqual(m.feed, merged) {
		m.feed = merged
		m.feedGen++
	}

	if m.liveFollow {
		m.pinFeedToBottom()
	} else {
		newLines := m.totalFeedLines()
		if newLines > oldLines {
			m.feedScroll += (newLines - oldLines)
		}
		maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
		if m.feedScroll > maxScroll {
			m.feedScroll = maxScroll
		}
	}
}

// processThinkingEvents applies raw model.thinking.* events to the feed.
// It finds existing thinking entries by thinkingStep for cross-batch correlation.
func (m *Model) processThinkingEvents(events []types.EventRecord) {
	if len(events) == 0 {
		return
	}

	// findThinking returns the index of the thinking feedEntry for the given step, or -1.
	findThinking := func(step string) int {
		for i := range m.feed {
			if m.feed[i].kind == feedThinking && m.feed[i].thinkingStep == step {
				return i
			}
		}
		return -1
	}

	oldLines := m.totalFeedLines()
	changed := false

	for _, ev := range events {
		step := strings.TrimSpace(ev.Data["step"])
		switch ev.Type {
		case "context.size":
			m.contextTokens = parseIntStr(ev.Data["currentTokens"])
			m.contextBudgetTokens = parseIntStr(ev.Data["budgetTokens"])
			continue
		case "context.compacted":
			m.feed = append(m.feed, feedEntry{
				kind:      feedSystem,
				timestamp: ev.Timestamp,
				text:      strings.TrimSpace(ev.Message),
			})
			changed = true
			continue
		case "model.thinking.start":
			if findThinking(step) >= 0 {
				continue // already tracking this step
			}
			m.feed = append(m.feed, feedEntry{
				kind:         feedThinking,
				timestamp:    ev.Timestamp,
				text:         "Thinking",
				sourceID:     ev.EventID,
				live:         true,
				thinkingStep: step,
			})
			changed = true

		case "model.thinking.summary":
			idx := findThinking(step)
			if idx < 0 {
				continue
			}
			txt := strings.TrimSpace(ev.Data["text"])
			if txt != "" {
				m.feed[idx].thinkingLines = append(m.feed[idx].thinkingLines, txt)
				changed = true
			}

		case "model.thinking.end":
			idx := findThinking(step)
			if idx < 0 {
				continue
			}
			if !m.feed[idx].live {
				continue // already closed
			}
			m.feed[idx].live = false
			start := m.feed[idx].timestamp
			if !start.IsZero() && ev.Timestamp.After(start) {
				m.feed[idx].thinkingDuration = ev.Timestamp.Sub(start)
			}
			changed = true
		}
	}

	if !changed {
		return
	}
	sort.SliceStable(m.feed, func(i, j int) bool {
		return m.feed[i].timestamp.Before(m.feed[j].timestamp)
	})
	m.feedGen++

	if m.liveFollow {
		m.pinFeedToBottom()
	} else {
		newLines := m.totalFeedLines()
		if newLines > oldLines {
			m.feedScroll += (newLines - oldLines)
		}
		maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
		if m.feedScroll > maxScroll {
			m.feedScroll = maxScroll
		}
	}
}

func (m *Model) mergeThinkingEntries(entries []feedEntry) {
	oldLines := m.totalFeedLines()

	// Deduplicate by sourceID against existing thinking or agent text entries.
	existing := make(map[string]bool)
	for _, e := range m.feed {
		if e.sourceID != "" {
			if e.kind == feedThinking {
				existing["thinking_"+e.sourceID] = true
			} else if key := taskResponseKeyFromEntry(e); key != "" {
				existing["task_"+key] = true
			}
		}
	}
	for _, e := range entries {
		normalizeFeedEntry(&e)
		key := ""
		if e.sourceID != "" {
			if e.kind == feedThinking {
				key = "thinking_" + e.sourceID
			} else if taskKey := taskResponseKeyFromEntry(e); taskKey != "" {
				key = "task_" + taskKey
			}
		}
		if key != "" && existing[key] {
			continue
		}
		if key != "" {
			existing[key] = true
		}
		m.feed = append(m.feed, e)
	}
	m.feed = dedupeFeedEntriesByIdentity(m.feed)
	sort.SliceStable(m.feed, func(i, j int) bool {
		return m.feed[i].timestamp.Before(m.feed[j].timestamp)
	})
	m.feedGen++

	if m.liveFollow {
		m.pinFeedToBottom()
	} else {
		newLines := m.totalFeedLines()
		if newLines > oldLines {
			m.feedScroll += (newLines - oldLines)
		}
		maxScroll := max(0, m.totalFeedLines()-m.feedHeight())
		if m.feedScroll > maxScroll {
			m.feedScroll = maxScroll
		}
	}
}

func (m *Model) pinFeedToBottom() {
	m.liveFollow = true
	m.feedScroll = max(0, m.totalFeedLines()-m.feedHeight())
}

func (m *Model) applyRecoveredScope(scope rpcscope.ScopeState) {
	if strings.TrimSpace(scope.SessionID) != "" {
		m.sessionID = strings.TrimSpace(scope.SessionID)
	}
	if strings.TrimSpace(scope.TeamID) != "" {
		m.teamID = strings.TrimSpace(scope.TeamID)
	}
	if strings.TrimSpace(scope.RunID) != "" {
		m.runID = strings.TrimSpace(scope.RunID)
	}
	if strings.TrimSpace(scope.ThreadID) != "" {
		m.threadID = strings.TrimSpace(scope.ThreadID)
	}
	if strings.TrimSpace(scope.CoordinatorRole) != "" {
		m.coordinatorRole = strings.TrimSpace(scope.CoordinatorRole)
	}
}

func (m *Model) appendReconnectNotice(recovered bool) {
	if !recovered {
		return
	}
	if time.Since(m.lastReconnectAt) < 5*time.Second {
		return
	}
	m.lastReconnectAt = time.Now()
	m.feed = append(m.feed, feedEntry{
		kind:      feedSystem,
		timestamp: m.lastReconnectAt,
		text:      "reconnected context",
	})
	m.feedGen++
}

// feedEntryRenderChanged returns true if anything that affects the rendered
// output of a single entry has changed between old and new.
func feedEntryRenderChanged(old, next feedEntry) bool {
	if old.status != next.status {
		return true
	}
	if old.opKind != next.opKind {
		return true
	}
	if old.text != next.text {
		return true
	}
	if old.path != next.path {
		return true
	}
	if old.live != next.live {
		return true
	}
	if old.thinkingDuration != next.thinkingDuration {
		return true
	}
	if len(old.thinkingLines) != len(next.thinkingLines) {
		return true
	}
	if old.childCount != next.childCount {
		return true
	}
	if len(old.bridgeWriteOps) != len(next.bridgeWriteOps) {
		return true
	}
	if old.planDetailsTitle != next.planDetailsTitle {
		return true
	}
	if len(old.planItems) != len(next.planItems) {
		return true
	}
	oldPP := ""
	if old.data != nil {
		oldPP = old.data["patchPreview"]
	}
	nextPP := ""
	if next.data != nil {
		nextPP = next.data["patchPreview"]
	}
	if oldPP != nextPP {
		return true
	}
	return false
}

// feedEqual returns true when two feed slices are identical in length and every
// entry shares the same identity key, status, and patchPreview.  Used to avoid
// bumping feedGen (and thus invalidating the line cache) when a poll returns
// the same data.
func feedEqual(a, b []feedEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].identityKey != b[i].identityKey {
			return false
		}
		if feedEntryRenderChanged(a[i], b[i]) {
			return false
		}
	}
	return true
}

func (m *Model) feedHeight() int {
	h := m.height - 5 // header + separator + input(2 with border) + footer
	if h < 1 {
		return 1
	}
	return h
}

// ── Agent status derivation ────────────────────────────────────────────

func (m *Model) setAgentStatus(s string) {
	m.agentStatus = s
	m.statusExpiresAt = time.Time{}
}

func (m *Model) setAgentStatusExpiring(s string, d time.Duration) {
	m.agentStatus = s
	m.statusExpiresAt = time.Now().Add(d)
}

func (m *Model) expireAgentStatus() {
	if !m.statusExpiresAt.IsZero() && time.Now().After(m.statusExpiresAt) {
		m.agentStatus = "Idle"
		m.statusExpiresAt = time.Time{}
	}
}

func (m *Model) deriveAgentStatus() {
	// Find the last agent op, last agent text (response), and last thinking entry.
	var lastOp *feedEntry
	var lastAgentText *feedEntry
	var lastThinking *feedEntry
	for i := len(m.feed) - 1; i >= 0; i-- {
		e := &m.feed[i]
		if lastOp == nil && e.kind == feedAgent && !e.isText {
			lastOp = e
		}
		if lastAgentText == nil && e.kind == feedAgent && e.isText {
			lastAgentText = e
		}
		if lastThinking == nil && e.kind == feedThinking {
			lastThinking = e
		}
		if lastOp != nil && lastAgentText != nil && lastThinking != nil {
			break
		}
	}

	// If the most recent agent activity is a text response after thinking, agent has responded -> Idle.
	if lastAgentText != nil && lastThinking != nil && lastAgentText.timestamp.After(lastThinking.timestamp) {
		if lastAgentText.timestamp.After(time.Now().Add(-5 * time.Second)) {
			m.setAgentStatusExpiring("Done", 5*time.Second)
		} else {
			m.setAgentStatus("Idle")
		}
		return
	}
	if lastAgentText != nil && lastThinking == nil && lastOp == nil {
		// Feed is only agent text (no ops yet).
		m.setAgentStatus("Idle")
		return
	}

	if lastOp == nil {
		m.setAgentStatus("Idle")
		return
	}

	s := strings.ToLower(strings.TrimSpace(lastOp.status))
	switch {
	case s == "pending" || s == "running":
		m.setAgentStatus("Processing")
	case s == "error" || s == "failed":
		m.setAgentStatusExpiring("Error", 10*time.Second)
	case s == "done" || s == "completed" || s == "ok" || s == "succeeded":
		// If thinking is more recent than the last op and still live, show Processing.
		// If thinking has ended and there's no agent text after it, don't stay on Processing.
		if lastThinking != nil && lastThinking.timestamp.After(lastOp.timestamp) && lastThinking.live {
			m.setAgentStatus("Processing")
		} else if lastThinking != nil && lastThinking.timestamp.After(lastOp.timestamp) && !lastThinking.live {
			// Thinking ended; no agent text after it in feed order, but we already returned above if there was.
			if lastOp.timestamp.After(time.Now().Add(-5 * time.Second)) {
				m.setAgentStatusExpiring("Done", 5*time.Second)
			} else {
				m.setAgentStatus("Idle")
			}
		} else if lastOp.timestamp.After(time.Now().Add(-5 * time.Second)) {
			m.setAgentStatusExpiring("Done", 5*time.Second)
		} else {
			m.setAgentStatus("Idle")
		}
	default:
		m.setAgentStatus("Idle")
	}
}

func (m *Model) isRunInScope(runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return true
	}
	// In team mode, accept streaming events from all runs (child agents included).
	if strings.TrimSpace(m.teamID) != "" {
		return true
	}
	current := strings.TrimSpace(m.runID)
	if current == "" {
		return true
	}
	return runID == current
}
