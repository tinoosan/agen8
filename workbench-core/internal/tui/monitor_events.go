package tui

import (
	"fmt"
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
		m.agentStatusLine = "⏳ Thinking…"
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
		m.agentStatusLine = "⏳ Working…"
	case "task.done":
		taskID := strings.TrimSpace(ev.Data["taskId"])
		if taskID == "" {
			return
		}
		m.currentTask = nil
		m.agentStatusLine = "✓ Done"
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
		m.agentStatusLine = ""
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
		m.agentStatusLine = "🔧 " + truncateText(strings.TrimSpace(ev.Data["op"]), 40)
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
		m.agentStatusLine = ""
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
