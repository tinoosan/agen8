package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/workbench-core/pkg/types"
)

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

func (m *monitorModel) observeEvent(ev types.EventRecord) {
	if !m.markEventSeen(ev) {
		return
	}
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
	// Team monitor model state is sourced from team manifest; avoid overwriting
	// it with per-run effectiveModel events.
	if strings.TrimSpace(m.teamID) == "" {
		if v := strings.TrimSpace(ev.Data["effectiveModel"]); v != "" {
			m.model = v
		} else if v := strings.TrimSpace(ev.Data["model"]); v != "" {
			if strings.TrimSpace(m.model) == "" || ev.Type == "control.success" {
				m.model = v
			}
		}
	} else if v := strings.TrimSpace(ev.Data["model"]); v != "" {
		if strings.TrimSpace(m.model) == "" || strings.EqualFold(strings.TrimSpace(m.model), "team") {
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
		m.setStatus("Processing…")
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
		m.setStatusExpiring("⚠ LLM Error", 10*time.Second)
		m.stats.lastLLMErrorClass = fallback(strings.TrimSpace(ev.Data["class"]), "unknown")
		m.stats.lastLLMErrorRetryable = parseBool(ev.Data["retryable"])
		m.stats.lastLLMErrorSet = true
	}

	// Lifecycle events that update the status indicator regardless of the main switch.
	switch ev.Type {
	case "agent.turn.complete":
		m.setStatus("Idle")
	case "agent.error":
		m.setStatusExpiring("⚠ Error", 10*time.Second)
	case "llm.retry":
		m.setStatus("Retrying…")
	case "daemon.stop":
		m.setStatus("Stopped")
	case "daemon.start":
		m.setStatusExpiring("Starting…", 5*time.Second)
	case "run.start":
		// Per-run agent started (distinct from daemon process start)
	case "daemon.error", "daemon.runner.error":
		m.setStatusExpiring("⚠ Daemon Error", 10*time.Second)
	}
}

func (m *monitorModel) markEventSeen(ev types.EventRecord) bool {
	if m == nil {
		return true
	}
	if m.seenEventIDs == nil {
		m.seenEventIDs = map[string]time.Time{}
	}
	key := strings.TrimSpace(ev.EventID)
	if key == "" {
		key = strings.TrimSpace(ev.RunID) + "|" + ev.Timestamp.UTC().Format(time.RFC3339Nano) + "|" + strings.TrimSpace(ev.Type) + "|" + strings.TrimSpace(ev.Message)
		if role := strings.TrimSpace(ev.Data["role"]); role != "" {
			key += "|" + role
		}
	}
	if key == "" {
		return true
	}
	if _, ok := m.seenEventIDs[key]; ok {
		return false
	}
	now := time.Now().UTC()
	m.seenEventIDs[key] = now

	// Prune occasionally to keep memory bounded for long-lived sessions.
	if len(m.seenEventIDs) > 10000 {
		cutoff := now.Add(-30 * time.Minute)
		for id, seenAt := range m.seenEventIDs {
			if seenAt.Before(cutoff) {
				delete(m.seenEventIDs, id)
			}
		}
		if len(m.seenEventIDs) > 10000 {
			// Fallback: hard reset when still too large.
			m.seenEventIDs = map[string]time.Time{key: now}
		}
	}
	return true
}

// updateInboxTaskStatus updates both m.inbox[taskID] and the matching entry in
// m.inboxList (if present), then sets m.dirtyInbox so the inbox view re-renders.
// Empty status/goal and zero startedAt are ignored.
func (m *monitorModel) updateInboxTaskStatus(taskID, status, goal string, startedAt time.Time) {
	if taskID == "" {
		return
	}
	if m.inbox == nil {
		m.inbox = make(map[string]taskState)
	}
	ts := m.inbox[taskID]
	ts.TaskID = taskID
	if status != "" {
		ts.Status = status
	}
	if goal != "" {
		ts.Goal = goal
	}
	if !startedAt.IsZero() {
		ts.StartedAt = startedAt
	}
	m.inbox[taskID] = ts
	for i := range m.inboxList {
		if m.inboxList[i].TaskID == taskID {
			if status != "" {
				m.inboxList[i].Status = status
			}
			if goal != "" {
				m.inboxList[i].Goal = goal
			}
			if !startedAt.IsZero() {
				m.inboxList[i].StartedAt = startedAt
			}
			break
		}
	}
	m.dirtyInbox = true
}

func (m *monitorModel) observeTaskEvent(ev types.EventRecord) {
	switch ev.Type {
	case "task.queued":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.updateInboxTaskStatus(taskID, string(types.TaskStatusPending), strings.TrimSpace(ev.Data["goal"]), time.Time{})
	case "webhook.task.queued":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.updateInboxTaskStatus(taskID, string(types.TaskStatusPending), strings.TrimSpace(ev.Data["goal"]), time.Time{})
	case "task.start":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.updateInboxTaskStatus(taskID, "active", strings.TrimSpace(ev.Data["goal"]), ev.Timestamp)
		ts := m.inbox[taskID]
		m.currentTask = &ts
		m.setStatus("Thinking…")
	case "task.delegated":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.updateInboxTaskStatus(taskID, "delegated", "", time.Time{})
		if m.currentTask != nil && m.currentTask.TaskID == taskID {
			m.currentTask = nil
		}
	case "task.done":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		status := strings.TrimSpace(ev.Data["status"])
		if status == "" {
			status = "succeeded"
		}
		m.updateInboxTaskStatus(taskID, status, "", time.Time{})
		m.currentTask = nil
		m.setStatusExpiring("✓ Done", 5*time.Second)
		m.stats.tasksDone++
		if v := getCostUSD(ev.Data); v != "" {
			m.stats.lastTurnCostUSD = v
		}
	case "task.quarantined":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.updateInboxTaskStatus(taskID, "quarantined", "", time.Time{})
		m.currentTask = nil
		m.setStatusExpiring("⚠ Quarantined", 8*time.Second)
		m.sessionTotalsReloadScheduled = true
		m.sessionTotalsReloadDebounce = 200 * time.Millisecond
	}
}

func (m *monitorModel) observeAgentOutput(ev types.EventRecord) {
	runID := strings.TrimSpace(ev.RunID)
	role := strings.TrimSpace(ev.Data["role"])

	item := AgentOutputItem{
		Timestamp: ev.Timestamp,
		RunID:     runID,
		Role:      role,
		Metadata:  make(map[string]string),
	}
	for k, v := range ev.Data {
		item.Metadata[k] = v
	}

	switch ev.Type {
	case "daemon.start", "daemon.stop", "daemon.control", "daemon.warning", "daemon.error", "daemon.runner.error", "run.start":
		item.Type = "system"
		item.Content = formatEventLine(ev)
		m.appendAgentOutputItem(item)

	case "llm.error", "llm.retry":
		item.Type = "error"
		item.Content = formatEventLine(ev)
		m.appendAgentOutputItem(item)

	case "task.queued", "webhook.task.queued", "task.start", "task.delegated", "task.done", "task.quarantined", "task.delivered", "task.heartbeat.enqueued", "task.heartbeat.skipped", "callback.batch.progress", "callback.batch.queued", "callback.batch.item.reviewed", "task.tool.invalid_repeated":
		item.Type = "info"
		for _, line := range formatTaskEventLines(ev) {
			it := item
			it.Content = line
			m.appendAgentOutputItem(it)
		}

	case "control.check", "control.success", "control.error":
		item.Type = "system"
		item.Content = formatEventLine(ev)
		m.appendAgentOutputItem(item)

	case "agent.error", "agent.turn.complete":
		item.Type = "info"
		item.Content = formatEventLine(ev)
		m.appendAgentOutputItem(item)

	case "agent.step":
		item.Type = "thought"
		summary := strings.TrimSpace(ev.Data["reasoningSummary"])
		// Filter out noisy/low-value summaries generated by some providers
		if regexp.MustCompile(`(?i)^Step \d+ (completed|start|finish).*`).MatchString(summary) {
			summary = ""
		}
		// User requested to ONLY see thoughts or nothing.
		// If reasoningSummary is just "Step X completed", we wiped it above.
		// If there is no reasoning, we should logically show nothing (or fall back to message if it's not generic).

		item.Content = summary
		// If summary is empty (or was wiped), check if there's other content?
		// Usually ev.Message might contain the full step text?
		// Actually, for "agent.step", ev.Message is often "Step X completed".

		if item.Content == "" {
			// Check if Message is also generic
			if !regexp.MustCompile(`(?i)^Step \d+ (completed|start|finish).*`).MatchString(ev.Message) {
				item.Content = ev.Message
			}
		}

		// If content is still empty, do NOT append.
		if item.Content != "" {
			m.appendAgentOutputItem(item)
		}

	case "user.message":
		item.Type = "user"
		item.Content = ev.Message
		m.appendAgentOutputItem(item)

	case "agent.op.request":
		m.setStatus("🔧 " + truncateText(strings.TrimSpace(ev.Data["op"]), 40))
		if shouldHideInboxOp(ev.Data["op"], ev.Data["path"]) {
			return
		}
		item.Type = "tool_call"
		txt := strings.TrimSpace(renderOpRequest(ev.Data))
		if txt == "" {
			txt = strings.TrimSpace(ev.Data["op"])
		}
		item.Content = txt
		idx := m.appendAgentOutputItem(item)
		if idx < 0 {
			return
		}
		ts := ev.Timestamp.Local().Format("15:04:05")
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
		m.setStatus("Thinking…")
		if shouldHideInboxOp(ev.Data["op"], ev.Data["path"]) {
			return
		}
		opID := strings.TrimSpace(ev.Data["opId"])
		entry, ok := m.takeAgentOutputPending(opID)
		status := formatAgentOutputStatus(ev)

		op := strings.TrimSpace(ev.Data["op"])
		tag := strings.TrimSpace(ev.Data["tag"])
		if op == "agent_spawn" || op == "task_create" || tag == "task_create" {
			m.sessionTotalsReloadScheduled = true
			m.sessionTotalsReloadDebounce = 500 * time.Millisecond
		}

		if ok {
			if entry.index >= 0 && entry.index < len(m.agentOutput) {
				it := m.agentOutput[entry.index]
				it.Type = "tool_result"
				// Update with response info while keeping the original call description
				it.Content = entry.desc + " — " + status
				m.agentOutput[entry.index] = it
				m.dirtyAgentOutput = true
				return
			}
		}
		// Fallback: append new item if original was lost
		item.Type = "tool_result"
		txt := strings.TrimSpace(renderOpRequest(ev.Data))
		if txt == "" {
			txt = strings.TrimSpace(ev.Data["op"])
		}
		item.Content = txt + " — " + status
		m.appendAgentOutputItem(item)
	}
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
		if kind := strings.TrimSpace(ev.Data["taskKind"]); kind != "" {
			header += " kind=" + kind
		}
		if source := strings.TrimSpace(ev.Data["source"]); source != "" {
			header += " source=" + source
		}
		if job := strings.TrimSpace(ev.Data["job"]); job != "" {
			header += " job=" + strconv.Quote(job)
		}
		if interval := strings.TrimSpace(ev.Data["interval"]); interval != "" {
			header += " interval=" + interval
		}
		lines := []string{header}

		if summary := strings.TrimSpace(ev.Data["summary"]); summary != "" {
			lines = append(lines, agentOutputSummaryMarker+summary)
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
	case "callback.batch.progress":
		parent := shortID(strings.TrimSpace(ev.Data["parentTaskId"]))
		wave := shortID(strings.TrimSpace(ev.Data["batchWaveId"]))
		done := strings.TrimSpace(ev.Data["batchCompletedCount"])
		total := strings.TrimSpace(ev.Data["batchExpectedCount"])
		if done == "" {
			done = "0"
		}
		if total == "" {
			total = "?"
		}
		reviewer := strings.TrimSpace(ev.Data["batchReviewer"])
		line := fmt.Sprintf("[%s] callback.batch.progress: %s %s/%s staged", ts, parent, done, total)
		if wave != "" {
			line += " wave=" + wave
		}
		if reviewer != "" {
			line += " reviewer=" + reviewer
		}
		return []string{line}
	case "callback.batch.queued":
		parent := shortID(strings.TrimSpace(ev.Data["parentTaskId"]))
		wave := shortID(strings.TrimSpace(ev.Data["batchWaveId"]))
		items := strings.TrimSpace(ev.Data["items"])
		if items == "" {
			items = "0"
		}
		reason := strings.TrimSpace(ev.Data["batchFlushReason"])
		if reason == "" {
			reason = "unknown"
		}
		partial := strings.TrimSpace(ev.Data["batchPartial"])
		line := fmt.Sprintf("[%s] callback.batch.queued: %s items=%s reason=%s", ts, parent, items, reason)
		if wave != "" {
			line += " wave=" + wave
		}
		if strings.EqualFold(partial, "true") {
			line += " partial=true"
		}
		return []string{line}
	case "callback.batch.item.reviewed":
		parent := shortID(strings.TrimSpace(ev.Data["parentTaskId"]))
		approved := strings.TrimSpace(ev.Data["approved"])
		retried := strings.TrimSpace(ev.Data["retry"])
		escalated := strings.TrimSpace(ev.Data["escalate"])
		line := fmt.Sprintf("[%s] callback.batch.item.reviewed: %s approved=%s retry=%s escalate=%s", ts, parent, fallback(approved, "0"), fallback(retried, "0"), fallback(escalated, "0"))
		return []string{line}
	case "task.tool.invalid_repeated":
		taskID := shortID(strings.TrimSpace(ev.Data["taskId"]))
		tool := fallback(strings.TrimSpace(ev.Data["tool"]), "unknown")
		reason := strings.TrimSpace(ev.Data["reason"])
		elapsed := strings.TrimSpace(ev.Data["elapsedSeconds"])
		consecutive := fallback(strings.TrimSpace(ev.Data["consecutiveInvalid"]), "0")
		line := fmt.Sprintf("[%s] task.tool.invalid_repeated: %s tool=%s consecutiveInvalid=%s", ts, taskID, tool, consecutive)
		if elapsed != "" {
			line += " elapsed=" + elapsed + "s"
		}
		if reason != "" {
			line += " reason=" + truncateText(reason, 120)
		}
		return []string{line}
	default:
		return []string{formatEventLine(ev)}
	}
}

func parseAgentOutputSummaryLine(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, agentOutputSummaryMarker) {
		return "", false
	}
	summary := strings.TrimSpace(strings.TrimPrefix(raw, agentOutputSummaryMarker))
	if summary == "" {
		return "", false
	}
	return summary, true
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
	if v := strings.TrimSpace(e.Data["taskKind"]); v != "" {
		line += " kind=" + v
	}
	if v := strings.TrimSpace(e.Data["source"]); v != "" {
		line += " source=" + v
	}
	if v := strings.TrimSpace(e.Data["job"]); v != "" {
		line += " job=" + truncateText(v, 40)
	}
	if v := strings.TrimSpace(e.Data["interval"]); v != "" {
		line += " interval=" + v
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
