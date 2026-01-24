package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/tinoosan/workbench-core/internal/events"
)

func (m *Model) observeActivityEvent(ev events.Event) {
	switch ev.Type {
	case "ui.editor.open":
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()
		fin := now

		p := strings.TrimSpace(ev.Data["path"])
		if p == "" {
			p = strings.TrimSpace(ev.Data["vpath"])
		}
		title := "Open editor"
		if p != "" {
			title = "Edit " + p
		}

		act := Activity{
			ID:         id,
			Kind:       "ui.editor.open",
			Title:      title,
			Status:     ActivityOK,
			StartedAt:  now,
			FinishedAt: &fin,
			Duration:   0,
			Path:       p,
			Ok:         "true",
		}
		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()
		return

	case "ui.editor.error":
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()
		fin := now

		act := Activity{
			ID:         id,
			Kind:       "ui.editor.open",
			Title:      "Editor error",
			Status:     ActivityError,
			StartedAt:  now,
			FinishedAt: &fin,
			Duration:   0,
			Ok:         "false",
			Error:      strings.TrimSpace(ev.Data["err"]),
		}
		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()
		return

	case "ui.open.ok":
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()
		fin := now

		p := strings.TrimSpace(ev.Data["path"])
		title := "Opened file"
		if p != "" {
			title = "Open " + p
		}

		act := Activity{
			ID:         id,
			Kind:       "ui.open",
			Title:      title,
			Status:     ActivityOK,
			StartedAt:  now,
			FinishedAt: &fin,
			Duration:   0,
			Path:       p,
			Ok:         "true",
		}
		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()
		return

	case "ui.open.error":
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()
		fin := now

		p := strings.TrimSpace(ev.Data["path"])
		title := "Open failed"
		if p != "" {
			title = "Open " + p
		}

		act := Activity{
			ID:         id,
			Kind:       "ui.open",
			Title:      title,
			Status:     ActivityError,
			StartedAt:  now,
			FinishedAt: &fin,
			Duration:   0,
			Path:       p,
			Ok:         "false",
			Error:      strings.TrimSpace(ev.Data["err"]),
		}
		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()
		return

	case "workdir.changed":
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()
		fin := now

		act := Activity{
			ID:         id,
			Kind:       "workdir.changed",
			Title:      "Workdir changed",
			Status:     ActivityOK,
			StartedAt:  now,
			FinishedAt: &fin,
			Duration:   0,
			From:       strings.TrimSpace(ev.Data["from"]),
			To:         strings.TrimSpace(ev.Data["to"]),
			Ok:         "true",
		}
		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()
		return

	case "workdir.error":
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()
		fin := now

		act := Activity{
			ID:         id,
			Kind:       "workdir.changed",
			Title:      "Workdir change failed",
			Status:     ActivityError,
			StartedAt:  now,
			FinishedAt: &fin,
			Duration:   0,
			From:       strings.TrimSpace(ev.Data["from"]),
			To:         strings.TrimSpace(ev.Data["to"]),
			Ok:         "false",
			Error:      strings.TrimSpace(ev.Data["err"]),
		}
		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()
		return

	case "llm.web.search":
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()
		fin := now

		n := strings.TrimSpace(ev.Data["count"])
		title := "Web search"
		if n != "" {
			title = "Web search (" + n + " sources)"
		}

		act := Activity{
			ID:            id,
			Kind:          "llm.web.search",
			Title:         title,
			Status:        ActivityOK,
			StartedAt:     now,
			FinishedAt:    &fin,
			Duration:      0,
			Ok:            "true",
			OutputPreview: strings.TrimSpace(ev.Data["sources"]),
		}
		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()
		return

	case "agent.op.request":
		op := strings.TrimSpace(ev.Data["op"])
		if op == "" {
			return
		}
		opID := strings.TrimSpace(ev.Data["opId"])
		m.activitySeq++
		id := fmt.Sprintf("act-%d", m.activitySeq)
		now := time.Now()

		act := Activity{
			ID:            id,
			Kind:          op,
			Status:        ActivityPending,
			StartedAt:     now,
			Path:          strings.TrimSpace(ev.Data["path"]),
			MaxBytes:      strings.TrimSpace(ev.Data["maxBytes"]),
			ToolID:        strings.TrimSpace(ev.Data["toolId"]),
			ActionID:      strings.TrimSpace(ev.Data["actionId"]),
			InputJSON:     strings.TrimSpace(ev.Data["input"]),
			TextPreview:   strings.TrimSpace(ev.Data["textPreview"]),
			TextTruncated: strings.TrimSpace(ev.Data["textTruncated"]) == "true",
			TextRedacted:  strings.TrimSpace(ev.Data["textRedacted"]) == "true",
			TextIsJSON:    strings.TrimSpace(ev.Data["textIsJSON"]) == "true",
			TextBytes:     strings.TrimSpace(ev.Data["textBytes"]),
			Data:          ev.Data,
		}
		if op == "tool.run" {
			act.Command = strings.TrimSpace(renderToolRunTranscript(act.ToolID, act.ActionID, act.InputJSON))
			if act.Command != "" {
				act.Title = "Run " + act.Command
			} else {
				act.Title = "Run tool"
			}
		} else {
			act.Title = renderOpRequest(ev.Data)
		}

		m.activities = append(m.activities, act)
		m.activityIndexByID[id] = len(m.activities) - 1
		if opID != "" {
			if m.activityIndexByOpID == nil {
				m.activityIndexByOpID = map[string]int{}
			}
			m.activityIndexByOpID[opID] = len(m.activities) - 1
		} else {
			// Back-compat: older hosts don't emit opId; use the old single-inflight behavior.
			m.pendingActivityID = id
		}
		m.refreshActivityList()
		m.activityList.Select(len(m.activities) - 1)
		m.refreshActivityDetail()

	case "agent.op.response":
		if strings.TrimSpace(ev.Data["op"]) == "" {
			return
		}
		opID := strings.TrimSpace(ev.Data["opId"])
		idx := -1
		ok := false
		if opID != "" && m.activityIndexByOpID != nil {
			idx, ok = m.activityIndexByOpID[opID]
		}
		// Fallback for older hosts / older persisted events: update the last pending activity.
		if !ok {
			idx, ok = m.activityIndexByID[m.pendingActivityID]
		}
		if !ok || idx < 0 || idx >= len(m.activities) {
			return
		}
		act := m.activities[idx]
		now := time.Now()

		act.Ok = strings.TrimSpace(ev.Data["ok"])
		act.Error = strings.TrimSpace(ev.Data["err"])
		act.CallID = strings.TrimSpace(ev.Data["callId"])
		act.OutputPreview = strings.TrimSpace(ev.Data["outputPreview"])
		act.BytesLen = strings.TrimSpace(ev.Data["bytesLen"])
		act.Truncated = strings.TrimSpace(ev.Data["truncated"]) == "true"

		fin := now
		act.FinishedAt = &fin
		act.Duration = fin.Sub(act.StartedAt)
		if act.Ok == "true" {
			act.Status = ActivityOK
		} else {
			act.Status = ActivityError
		}

		// Merge response data into existing activity data
		if act.Data == nil {
			act.Data = make(map[string]string)
		}
		for k, v := range ev.Data {
			act.Data[k] = v
		}

		m.activities[idx] = act
		if opID != "" && m.activityIndexByOpID != nil {
			delete(m.activityIndexByOpID, opID)
		} else {
			m.pendingActivityID = ""
		}
		m.refreshActivityList()
		m.refreshActivityDetail()
	}
}

func (m *Model) refreshActivityList() {
	items := make([]list.Item, 0, len(m.activities))
	for _, a := range m.activities {
		items = append(items, activityItem{act: a})
	}
	cur := m.activityList.Index()
	m.activityList.SetItems(items)
	if cur >= 0 && cur < len(items) {
		m.activityList.Select(cur)
	}
}

func (m *Model) refreshActivityDetail() {
	if !m.showDetails {
		return
	}
	if len(m.activities) == 0 || m.activityList.Index() < 0 || m.activityList.Index() >= len(m.activities) {
		m.activityDetail.SetContent("")
		return
	}
	w := max(24, m.activityDetail.Width-4)

	telemetryBadge := ""
	if m.showTelemetry {
		telemetryBadge = " _(telemetry on)_"
	}
	header := "### Details" + telemetryBadge + "\n\n"
	help := "_PgUp/PgDn scroll · e/enter expand · o open file · Ctrl+T telemetry_\n\n"

	if m.fileViewOpen {
		md := renderFilePreviewMarkdown(m.fileViewPath, m.fileViewContent, m.fileViewTruncated, m.fileViewErr)
		m.activityDetail.SetContent(strings.TrimRight(m.renderer.RenderMarkdown(header+help+md, w), "\n"))
		return
	}

	act := m.activities[m.activityList.Index()]
	md := renderActivityDetailMarkdown(act, m.showTelemetry, m.expandOutput)
	m.activityDetail.SetContent(strings.TrimRight(m.renderer.RenderMarkdown(header+help+md, w), "\n"))
}

func (m *Model) refreshPlanView() {
	if !m.showDetails {
		return
	}
	w := max(24, m.planViewport.Width-4)
	currentStep := ""
	body := ""
	planText := strings.TrimSpace(m.planMarkdown)
	if planText == "" {
		if strings.TrimSpace(m.planLoadErr) != "" {
			body = fmt.Sprintf("_Failed to load plan: %s_", m.planLoadErr)
		} else {
			body = "_No plan has been created yet._"
		}
	} else {
		highlighted, active := highlightPlanChecklist(m.planMarkdown)
		if active != "" {
			currentStep = fmt.Sprintf("_Current step: %s_\n\n", active)
		}
		if strings.TrimSpace(m.planLoadErr) != "" {
			body = fmt.Sprintf("_Failed to load plan: %s_\n\n%s", m.planLoadErr, highlighted)
		} else {
			body = highlighted
		}
	}
	help := "_Ctrl+Alt+P toggles tabs · Ctrl+A toggles sidebar_\n\n"
		content := currentStep + body + "\n\n" + help
	if strings.TrimSpace(content) == "" {
		content = "_Plan view is preparing…_"
	}
	m.planViewport.SetContent(strings.TrimRight(m.renderer.RenderMarkdown(content, w), "\n"))
}

func highlightPlanChecklist(markdown string) (string, string) {
	text := strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	active := ""
	found := false
	for i, line := range lines {
		if ok, checked := parsePlanChecklist(line); ok && !checked && !found {
			active = planChecklistLabel(line)
			lines[i] = highlightPlanLine(line)
			found = true
		}
	}
	return strings.Join(lines, "\n"), active
}

func parsePlanChecklist(line string) (bool, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 5 {
		return false, false
	}
	bullet := trimmed[0]
	if bullet != '-' && bullet != '*' && bullet != '+' {
		return false, false
	}
	if len(trimmed) < 5 || trimmed[1] != ' ' || trimmed[2] != '[' {
		return false, false
	}
	status := trimmed[3]
	if trimmed[4] != ']' {
		return false, false
	}
	switch status {
	case 'x', 'X':
		return true, true
	case ' ':
		return true, false
	}
	return false, false
}

func planChecklistLabel(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if idx := strings.Index(trimmed, "]"); idx >= 0 {
		return strings.TrimSpace(trimmed[idx+1:])
	}
	return strings.TrimSpace(trimmed)
}

func highlightPlanLine(line string) string {
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	body := strings.TrimLeft(line, " \t")
	body = strings.TrimSpace(body)
	return indent + "**" + body + "** _(current step)_"
}

func renderActivityDetailMarkdown(a Activity, telemetry bool, expanded bool) string {
	var b strings.Builder

	b.WriteString("**Fields**\n\n")
	if strings.TrimSpace(a.Kind) != "" {
		b.WriteString("- Operation: `")
		b.WriteString(a.Kind)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.From) != "" {
		b.WriteString("- From: `")
		b.WriteString(a.From)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.To) != "" {
		b.WriteString("- To: `")
		b.WriteString(a.To)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.Path) != "" {
		b.WriteString("- Path: `")
		b.WriteString(a.Path)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.ToolID) != "" {
		b.WriteString("- Tool: `")
		b.WriteString(a.ToolID)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.ActionID) != "" {
		b.WriteString("- Action: `")
		b.WriteString(a.ActionID)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(a.CallID) != "" {
		b.WriteString("- CallID: `")
		b.WriteString(a.CallID)
		b.WriteString("`\n")
	}
	b.WriteString("- Status: ")
	b.WriteString(string(a.Status))
	b.WriteString(" ")
	b.WriteString(a.ShortStatus())
	if a.Duration > 0 {
		b.WriteString(" · duration=")
		b.WriteString(a.Duration.Truncate(time.Millisecond).String())
	}
	b.WriteString("\n\n")

	b.WriteString("**Arguments**\n\n")
	renderedArgs := renderActivityArgumentsMarkdown(a, telemetry)
	if strings.TrimSpace(renderedArgs) != "" {
		b.WriteString(renderedArgs)
		if !strings.HasSuffix(renderedArgs, "\n") {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("_No arguments._\n")
	}
	b.WriteString("\n")

	b.WriteString("**Output**\n\n")
	if strings.TrimSpace(a.Error) != "" {
		b.WriteString("- error: ")
		b.WriteString(a.Error)
		b.WriteString("\n")
	}
	openable := openablePathsForActivity(a)
	if len(openable) != 0 {
		for _, p := range openable {
			b.WriteString("- file: `")
			b.WriteString(p)
			b.WriteString("` _(press `o` to open)_\n")
		}
	}

	if (a.Kind == "fs.write" || a.Kind == "fs.append") && !a.TextRedacted && strings.TrimSpace(a.TextPreview) != "" {
		lang := guessCodeFenceLang(a.Path, a.TextIsJSON)
		b.WriteString("\n**Written content preview**")
		if a.TextTruncated {
			b.WriteString(" _(truncated)_")
		}
		b.WriteString("\n\n")
		if strings.EqualFold(lang, "json") {
			b.WriteString(FormatJSON(a.TextPreview))
		} else {
			b.WriteString(FormatCode(lang, a.TextPreview))
		}
		b.WriteString("\n")
	} else if (a.Kind == "fs.write" || a.Kind == "fs.append") && a.TextRedacted {
		b.WriteString("\n**Written content preview**\n\n_(redacted)_\n")
	}

	if strings.TrimSpace(a.Kind) == "llm.web.search" && strings.TrimSpace(a.OutputPreview) != "" {
		b.WriteString("\n**Sources**\n\n")
		b.WriteString(a.OutputPreview)
		if !strings.HasSuffix(a.OutputPreview, "\n") {
			b.WriteString("\n")
		}
	}

	if strings.TrimSpace(a.OutputPreview) != "" && strings.TrimSpace(a.Kind) != "llm.web.search" {
		txt := a.OutputPreview
		if !expanded && len(txt) > 600 {
			txt = txt[:599] + "…"
		}
		b.WriteString("\n**Tool output preview** _(press `e` to expand)_\n\n")
		b.WriteString(FormatCode("text", txt))
		b.WriteString("\n")
	} else if a.Kind == "shell_exec" {
		// Specific handling for shell_exec components
		exitCode := strings.TrimSpace(a.Data["exitCode"])
		stdout := strings.TrimSpace(a.Data["stdout"])
		stderr := strings.TrimSpace(a.Data["stderr"])
		if exitCode != "" {
			b.WriteString("- exitCode: `")
			b.WriteString(exitCode)
			b.WriteString("`\n")
		}
		if stdout != "" {
			b.WriteString("\n**stdout**\n\n")
			b.WriteString(FormatCode("text", stdout))
			b.WriteString("\n")
		}
		if stderr != "" {
			b.WriteString("\n**stderr**\n\n")
			b.WriteString(FormatCode("text", stderr))
			b.WriteString("\n")
		}
	} else if a.Kind == "http_fetch" {
		status := strings.TrimSpace(a.Data["status"])
		body := strings.TrimSpace(a.Data["body"])
		if status != "" {
			b.WriteString("- status: `")
			b.WriteString(status)
			b.WriteString("`\n")
		}
		if body != "" {
			b.WriteString("\n**Body**\n\n")
			b.WriteString(FormatCode("html", body))
			b.WriteString("\n")
		}
	} else if a.Kind == "trace" {
		output := strings.TrimSpace(a.Data["output"])
		if output != "" {
			b.WriteString("\n**Output**\n\n")
			b.WriteString(FormatCode("text", output))
			b.WriteString("\n")
		}
	}

	if telemetry {
		b.WriteString("\n**Telemetry**\n\n")
		if strings.TrimSpace(a.MaxBytes) != "" && a.Kind == "fs.read" {
			b.WriteString("- maxBytes: ")
			b.WriteString(a.MaxBytes)
			b.WriteString("\n")
		}
		if strings.TrimSpace(a.TextBytes) != "" && (a.Kind == "fs.write" || a.Kind == "fs.append") {
			b.WriteString("- textBytes: ")
			b.WriteString(a.TextBytes)
			b.WriteString("\n")
		}
		if strings.TrimSpace(a.BytesLen) != "" {
			b.WriteString("- bytesLen: ")
			b.WriteString(a.BytesLen)
			b.WriteString("\n")
		}
		if a.Truncated {
			b.WriteString("- truncated: true\n")
		}
	}

	return b.String()
}

func renderActivityArgumentsMarkdown(a Activity, telemetry bool) string {
	var b strings.Builder

	switch a.Kind {
	case "workdir.changed":
		if strings.TrimSpace(a.From) != "" || strings.TrimSpace(a.To) != "" {
			b.WriteString("- from: `")
			b.WriteString(strings.TrimSpace(a.From))
			b.WriteString("`\n")
			b.WriteString("- to: `")
			b.WriteString(strings.TrimSpace(a.To))
			b.WriteString("`\n")
		}
	case "tool.run":
		if strings.TrimSpace(a.Command) != "" {
			b.WriteString("- command:\n\n")
			b.WriteString(FormatCode("bash", a.Command))
			b.WriteString("\n")
		}
		if strings.TrimSpace(a.InputJSON) != "" {
			b.WriteString("\n- input:\n\n")
			b.WriteString(FormatJSON(a.InputJSON))
			b.WriteString("\n")
		}
	default:
		if strings.TrimSpace(a.Path) != "" {
			b.WriteString("- path: `")
			b.WriteString(a.Path)
			b.WriteString("`\n")
		}
		if telemetry && strings.TrimSpace(a.MaxBytes) != "" && a.Kind == "fs.read" {
			b.WriteString("- maxBytes: ")
			b.WriteString(a.MaxBytes)
			b.WriteString("\n")
		}

		// Handle host operations with new telemetry fields
		if a.Kind == "shell_exec" {
			if v := strings.TrimSpace(a.Data["argvPreview"]); v != "" {
				b.WriteString("- command:\n\n")
				b.WriteString(FormatCode("bash", v))
				b.WriteString("\n")
			}
			if v := strings.TrimSpace(a.Data["cwd"]); v != "" {
				b.WriteString("- cwd: `")
				b.WriteString(v)
				b.WriteString("`\n")
			}
		} else if a.Kind == "http_fetch" {
			if v := strings.TrimSpace(a.Data["url"]); v != "" {
				b.WriteString("- url: `")
				b.WriteString(v)
				b.WriteString("`\n")
			}
			if v := strings.TrimSpace(a.Data["method"]); v != "" {
				b.WriteString("- method: `")
				b.WriteString(v)
				b.WriteString("`\n")
			}
		} else if a.Kind == "trace" {
			if v := strings.TrimSpace(a.Data["traceAction"]); v != "" {
				b.WriteString("- action: `")
				b.WriteString(v)
				b.WriteString("`\n")
			}
			if v := strings.TrimSpace(a.Data["traceKey"]); v != "" {
				b.WriteString("- key: `")
				b.WriteString(v)
				b.WriteString("`\n")
			}
			if v := strings.TrimSpace(a.Data["traceInput"]); v != "" {
				b.WriteString("- input: `")
				b.WriteString(v)
				b.WriteString("`\n")
			}
		}
	}

	return b.String()
}

func openablePathsForActivity(a Activity) []string {
	paths := make([]string, 0, 2)
	if strings.TrimSpace(a.Kind) == "tool.run" && strings.TrimSpace(a.CallID) != "" {
		paths = append(paths, "/results/"+a.CallID+"/response.json")
	}
	if strings.HasPrefix(strings.TrimSpace(a.Kind), "fs.") && strings.TrimSpace(a.Path) != "" {
		paths = append(paths, a.Path)
	}
	return paths
}

func renderFilePreviewMarkdown(path string, content string, truncated bool, errStr string) string {
	var b strings.Builder
	b.WriteString("## File preview\n\n")
	if strings.TrimSpace(path) != "" {
		b.WriteString("- path: `")
		b.WriteString(strings.TrimSpace(path))
		b.WriteString("`\n")
	}
	if strings.TrimSpace(errStr) != "" {
		b.WriteString("\n**Error**\n\n")
		b.WriteString(errStr)
		b.WriteString("\n")
		return b.String()
	}
	if strings.TrimSpace(content) == "" {
		b.WriteString("\n_(empty)_\n")
		return b.String()
	}

	lang := guessCodeFenceLang(path, strings.HasSuffix(strings.ToLower(path), ".json"))
	b.WriteString("\n**Content**")
	if truncated {
		b.WriteString(" _(truncated)_")
	}
	b.WriteString("\n\n")

	if strings.EqualFold(lang, "json") {
		b.WriteString(FormatJSON(content))
	} else {
		b.WriteString(FormatCode(lang, content))
	}
	b.WriteString("\n")
	return b.String()
}

func (m *Model) openSelectedActivityFile() tea.Cmd {
	if !m.showDetails || len(m.activities) == 0 {
		return nil
	}
	idx := m.activityList.Index()
	if idx < 0 || idx >= len(m.activities) {
		return nil
	}
	act := m.activities[idx]
	paths := openablePathsForActivity(act)
	if len(paths) == 0 {
		return nil
	}

	path := paths[0]
	m.fileViewOpen = true
	m.fileViewPath = path
	m.fileViewContent = "_Loading…_"
	m.fileViewTruncated = false
	m.fileViewErr = ""
	m.refreshActivityDetail()

	type vfsReader interface {
		ReadVFS(ctx context.Context, path string, maxBytes int) (text string, bytesLen int, truncated bool, err error)
	}
	vr, ok := m.runner.(vfsReader)
	if !ok {
		return func() tea.Msg {
			return fileViewMsg{path: path, err: fmt.Errorf("file preview not supported by runner")}
		}
	}

	const maxPreviewBytes = 16 * 1024
	return func() tea.Msg {
		txt, _, tr, err := vr.ReadVFS(m.ctx, path, maxPreviewBytes)
		return fileViewMsg{path: path, content: txt, truncated: tr, err: err}
	}
}

func guessCodeFenceLang(path string, isJSON bool) string {
	if isJSON {
		return "json"
	}
	low := strings.ToLower(strings.TrimSpace(path))
	switch {
	case strings.HasSuffix(low, ".md"):
		return "md"
	case strings.HasSuffix(low, ".go"):
		return "go"
	case strings.HasSuffix(low, ".sh"):
		return "sh"
	case strings.HasSuffix(low, ".js"):
		return "js"
	case strings.HasSuffix(low, ".ts"):
		return "ts"
	case strings.HasSuffix(low, ".html"), strings.HasSuffix(low, ".htm"):
		return "html"
	default:
		return "text"
	}
}
