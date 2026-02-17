package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/opmeta"
	"github.com/tinoosan/workbench-core/pkg/events"
)

// RenderClass is the presentation class for an incoming host event.
//
// This is intentionally small and opinionated: the transcript view should read like a
// narrative, while the details view can show raw events.
type RenderClass int

const (
	RenderIgnore RenderClass = iota
	RenderIntent
	RenderAction
	RenderTelemetry
	RenderOutcome
)

// RenderResult is the TUI-friendly projection of an event.
type RenderResult struct {
	Class RenderClass
	Text  string // for transcript (human-friendly)
	Raw   string // for details panel (raw event JSON)
}

// rawEventJSON returns the same compact JSON shape emitted by events.ConsoleSink,
// but without any timestamp prefix.
func rawEventJSON(ev events.Event) string {
	line := struct {
		Type    string            `json:"type"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data,omitempty"`
	}{
		Type:    ev.Type,
		Message: ev.Message,
		Data:    ev.Data,
	}
	b, err := json.Marshal(line)
	if err != nil {
		return ""
	}
	return string(b)
}

var eventClassMap = map[string]RenderClass{
	"host.mounted":         RenderIgnore,
	"run.started":          RenderIgnore,
	"run.completed":        RenderIgnore,
	"agent.loop.start":     RenderIgnore,
	"llm.usage":            RenderIgnore,
	"user.message":         RenderIgnore,
	"workdir.pwd":          RenderIgnore,
	"agent.op.request":     RenderAction,
	"agent.op.response":    RenderAction,
	"run.warning":          RenderAction,
	"refs.attached":        RenderAction,
	"refs.ambiguous":       RenderAction,
	"refs.unresolved":      RenderAction,
	"artifact.published":   RenderAction,
	"ui.editor.open":       RenderAction,
	"ui.editor.error":      RenderAction,
	"ui.open.ok":           RenderAction,
	"ui.open.error":        RenderAction,
	"workdir.changed":      RenderAction,
	"workdir.error":        RenderAction,
	"context.update":       RenderTelemetry,
	"context.constructor":  RenderTelemetry,
	"llm.usage.total":      RenderTelemetry,
	"llm.cost.total":       RenderTelemetry,
	"memory.evaluate":      RenderOutcome,
	"memory.commit":        RenderOutcome,
	"memory.audit.append":  RenderOutcome,
	"profile.evaluate":     RenderOutcome,
	"profile.commit":       RenderOutcome,
	"profile.audit.append": RenderOutcome,
	"agent.turn.complete":  RenderOutcome,
	"agent.error":          RenderOutcome,
}

var eventTextMap = map[string]func(events.Event) string{
	"agent.op.request":   formatAgentOpRequestEventText,
	"agent.op.response":  formatAgentOpResponseEventText,
	"run.warning":        formatRunWarningEventText,
	"refs.attached":      formatRefsAttachedEventText,
	"refs.ambiguous":     formatRefsAmbiguousEventText,
	"refs.unresolved":    formatRefsUnresolvedEventText,
	"artifact.published": formatArtifactPublishedEventText,
	"ui.editor.open":     formatUIEditorOpenEventText,
	"ui.editor.error":    formatUIEditorErrorEventText,
	"ui.open.ok":         formatUIOpenOKEventText,
	"ui.open.error":      formatUIOpenErrorEventText,
	"workdir.changed":    formatWorkdirChangedEventText,
	"workdir.pwd":        formatWorkdirPWDEventText,
	"workdir.error":      formatWorkdirErrorEventText,
}

// classifyEvent converts a Workbench event into one of the chat-first presentation primitives.
//
// IMPORTANT: This does not expose any chain-of-thought. It only reflects observable activity.
func classifyEvent(ev events.Event) RenderResult {
	res := RenderResult{Raw: rawEventJSON(ev)}

	if (ev.Type == "agent.op.request" || ev.Type == "agent.op.response") &&
		shouldHideInboxOp(strings.TrimSpace(ev.Data["op"]), strings.TrimSpace(ev.Data["path"])) {
		res.Class = RenderIgnore
		return res
	}

	class, ok := eventClassMap[ev.Type]
	if !ok {
		res.Class = RenderIgnore
		return res
	}
	res.Class = class

	if f, ok := eventTextMap[ev.Type]; ok {
		res.Text = f(ev)
	}
	return res
}

func formatAgentOpRequestEventText(ev events.Event) string {
	return renderOpRequest(ev.Data)
}

func formatAgentOpResponseEventText(ev events.Event) string {
	return renderOpResponse(ev.Data)
}

func formatRunWarningEventText(ev events.Event) string {
	if txt := strings.TrimSpace(ev.Data["text"]); txt != "" {
		return "Warning: " + txt
	}
	return "Warning"
}

func formatRefsAttachedEventText(ev events.Event) string {
	files := strings.TrimSpace(ev.Data["files"])
	if files == "" {
		return "Attached referenced files"
	}
	return "Attached " + files
}

func formatRefsAmbiguousEventText(ev events.Event) string {
	tok := strings.TrimSpace(ev.Data["token"])
	cands := strings.TrimSpace(ev.Data["candidates"])
	if tok != "" && cands != "" {
		return "Ambiguous @" + tok + " (candidates: " + cands + ")"
	}
	return "Ambiguous @reference"
}

func formatRefsUnresolvedEventText(ev events.Event) string {
	toks := strings.TrimSpace(ev.Data["tokens"])
	if toks != "" {
		return "Unresolved @references: " + toks
	}
	return "Unresolved @references"
}

func formatArtifactPublishedEventText(ev events.Event) string {
	src := strings.TrimSpace(ev.Data["source"])
	dst := strings.TrimSpace(ev.Data["dest"])
	if src != "" && dst != "" {
		return "Published " + src + " → " + dst
	}
	return "Published artifact to workdir"
}

func formatUIEditorOpenEventText(ev events.Event) string {
	p := strings.TrimSpace(ev.Data["path"])
	if p == "" {
		p = strings.TrimSpace(ev.Data["vpath"])
	}
	if p != "" {
		return "Edit " + p
	}
	return "Open editor"
}

func formatUIEditorErrorEventText(ev events.Event) string {
	if e := strings.TrimSpace(ev.Data["err"]); e != "" {
		return "Editor error: " + e
	}
	return "Editor error"
}

func formatUIOpenOKEventText(ev events.Event) string {
	p := strings.TrimSpace(ev.Data["path"])
	if p != "" {
		return "Opened " + p
	}
	return "Opened file"
}

func formatUIOpenErrorEventText(ev events.Event) string {
	p := strings.TrimSpace(ev.Data["path"])
	e := strings.TrimSpace(ev.Data["err"])
	if p != "" && e != "" {
		return "Open failed: " + p + " (" + e + ")"
	}
	if e != "" {
		return "Open failed: " + e
	}
	return "Open failed"
}

func formatWorkdirChangedEventText(ev events.Event) string {
	from := strings.TrimSpace(ev.Data["from"])
	to := strings.TrimSpace(ev.Data["to"])
	if from != "" && to != "" {
		return "Workdir changed: " + from + " → " + to
	}
	if to != "" {
		return "Workdir: " + to
	}
	return "Workdir changed"
}

func formatWorkdirPWDEventText(ev events.Event) string {
	wd := strings.TrimSpace(ev.Data["workdir"])
	if wd == "" {
		return "Workdir"
	}
	return "Workdir: " + wd
}

func formatWorkdirErrorEventText(ev events.Event) string {
	if e := strings.TrimSpace(ev.Data["err"]); e != "" {
		return "Workdir change failed: " + e
	}
	return "Workdir change failed"
}

func renderOpRequest(d map[string]string) string {
	if v := strings.TrimSpace(d["requestText"]); v != "" {
		return v
	}
	op := strings.TrimSpace(d["op"])
	tag := strings.TrimSpace(d["tag"])

	if tag == "task_create" || op == "task_create" {
		return "Create task" // Simple description for the request
	}

	switch op {
	case "browser":
		action := strings.TrimSpace(d["action"])
		if action == "" {
			return "browse"
		}
		desc := "browse." + action
		target := ""
		switch action {
		case "navigate":
			target = strings.TrimSpace(d["url"])
		case "click", "type", "hover", "check", "uncheck", "upload", "download", "select":
			target = strings.TrimSpace(d["selector"])
		case "extract", "extract_links":
			target = strings.TrimSpace(d["selector"])
			if target == "" {
				target = "page"
			}
		default:
			target = firstNonEmpty(
				strings.TrimSpace(d["url"]),
				strings.TrimSpace(d["selector"]),
			)
		}
		if target != "" {
			return desc + " " + singleLinePreview(target, 120)
		}
		return desc
	case "email":
		to := singleLinePreview(strings.TrimSpace(d["to"]), 48)
		subject := singleLinePreview(strings.TrimSpace(d["subject"]), 72)
		if to != "" && subject != "" {
			return "Email " + to + ": " + subject
		}
		if to != "" {
			return "Email " + to
		}
		return "Email"
	case "agent_spawn":
		return opmeta.FormatRequestTitle(d)
	default:
		if isSharedOpRequestTitleOp(op) {
			return opmeta.FormatRequestTitle(d)
		}
		return compactKV(d, []string{"op", "path"})
	}
}

func isSharedOpRequestTitleOp(op string) bool {
	switch strings.TrimSpace(op) {
	case "fs_list", "fs_read", "fs_search", "fs_write", "fs_append", "fs_edit", "fs_patch", "shell_exec", "http_fetch", "trace_run", "agent_spawn", "task_create":
		return true
	default:
		return false
	}
}

func renderOpResponse(d map[string]string) string {
	if v := strings.TrimSpace(d["responseText"]); v != "" {
		return v
	}
	op := strings.TrimSpace(d["op"])
	tag := strings.TrimSpace(d["tag"])
	ok := strings.TrimSpace(d["ok"])
	errStr := strings.TrimSpace(d["err"])

	prefix := "✓"
	if ok != "true" {
		prefix = "✗"
	}

	if tag == "task_create" {
		if ok == "true" {
			return prefix + " " + strings.TrimSpace(d["text"])
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " task creation failed"
	}

	switch op {
	case "fs_read":
		tr := strings.TrimSpace(d["truncated"])
		if ok == "true" && tr == "true" {
			return prefix + " truncated"
		}
		if ok != "true" && errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	case "shell_exec":
		exitCode := strings.TrimSpace(d["exitCode"])
		if ok == "true" {
			if exitCode != "" {
				return prefix + " exit " + exitCode
			}
			return prefix + " ok"
		}
		if exitCode != "" && errStr != "" {
			return prefix + " exit " + exitCode + ": " + errStr
		}
		if exitCode != "" {
			return prefix + " exit " + exitCode
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " failed"
	case "http_fetch":
		status := strings.TrimSpace(d["status"])
		if ok == "true" {
			if status != "" {
				return prefix + " " + status
			}
			return prefix + " ok"
		}
		if status != "" && errStr != "" {
			return prefix + " " + status + ": " + errStr
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		if status != "" {
			return prefix + " " + status
		}
		return prefix + " failed"
	case "fs_search":
		results := strings.TrimSpace(d["results"])
		if ok == "true" && results != "" {
			if results == "1" {
				return prefix + " 1 result"
			}
			return prefix + " " + results + " results"
		}
		if ok != "true" && errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	case "email":
		if ok == "true" {
			return prefix + " sent"
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " failed"
	case "agent_spawn":
		if ok == "true" {
			return prefix + " child completed"
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " child failed"

	default:
		if strings.HasPrefix(op, "browser.") || op == "browser" {
			if ok != "true" {
				if errStr != "" {
					return prefix + " " + errStr
				}
				return prefix + " browser failed"
			}
			browserOp := strings.TrimPrefix(firstNonEmpty(strings.TrimSpace(d["browserOp"]), op), "browser.")
			title := strings.TrimSpace(d["title"])
			count := firstNonEmpty(strings.TrimSpace(d["items"]), strings.TrimSpace(d["count"]))
			switch browserOp {
			case "navigate", "back", "forward", "reload":
				if title != "" {
					return prefix + " navigated " + strconv.Quote(singleLinePreview(title, 80))
				}
				return prefix + " navigated"
			case "extract", "extract_links", "tab_list":
				if count != "" {
					return prefix + " extracted " + count + " items"
				}
				return prefix + " extracted"
			case "click":
				return prefix + " clicked"
			case "type":
				return prefix + " typed"
			case "screenshot", "pdf":
				return prefix + " captured"
			case "start":
				return prefix + " browser started"
			case "close":
				return prefix + " browser closed"
			default:
				if browserOp != "" {
					return prefix + " " + strings.ReplaceAll(browserOp, "_", " ")
				}
				return prefix + " browser ok"
			}
		}
		if errStr != "" && ok != "true" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	}
}

func singleLinePreview(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max < 2 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func shouldHideInboxOp(op, path string) bool {
	op = strings.TrimSpace(op)
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if !strings.HasPrefix(path, "/inbox") {
		return false
	}
	switch op {
	case "fs_list", "fs_read":
		return true
	default:
		return false
	}
}

func actionStatusIcon(d map[string]string) (string, bool) {
	ok := strings.TrimSpace(d["ok"])
	if ok == "" || ok == "true" {
		return "✓", false
	}
	return "✗", true
}

func actionCategory(op string) string {
	trimmed := strings.TrimSpace(op)
	if strings.HasPrefix(trimmed, "browser.") {
		return "Browsed"
	}
	switch trimmed {
	case "fs_list", "fs_read", "fs_search":
		return "Explored"
	case "fs_write", "fs_edit", "fs_patch", "fs_append":
		return "Updated"
	case "shell_exec":
		return "Ran"
	case "http_fetch":
		return "Called"
	case "browser":
		return "Browsed"
	case "email":
		return "Sent"
	case "trace_run":
		return "Traced"
	case "agent_spawn":
		return "Delegated"
	case "task_create":
		return "Created"
	default:
		return "Action"
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func compactKV(m map[string]string, ordered []string) string {
	if len(m) == 0 {
		return ""
	}
	var keys []string
	if len(ordered) != 0 {
		keys = ordered
	} else {
		keys = make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	parts := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, k := range keys {
		seen[k] = true
		if v := strings.TrimSpace(m[k]); v != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
	}
	if len(ordered) != 0 {
		rest := make([]string, 0, len(m))
		for k := range m {
			if seen[k] {
				continue
			}
			rest = append(rest, k)
		}
		sort.Strings(rest)
		for _, k := range rest {
			if v := strings.TrimSpace(m[k]); v != "" {
				parts = append(parts, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	return strings.Join(parts, " ")
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

func parseBool(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	b, _ := strconv.ParseBool(s)
	return b
}
