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

	switch ev.Type {
	// Noisy or non-user-facing events (still visible in details).
	case "host.mounted", "run.started", "run.completed", "agent.loop.start":
		res.Class = RenderIgnore
		return res
	case "llm.usage":
		res.Class = RenderIgnore
		return res
	case "user.message":
		res.Class = RenderIgnore
		return res

	// Actions: these are the main "what is the agent doing" bullets.
	case "agent.op.request":
		res.Class = RenderAction
		res.Text = renderOpRequest(ev.Data)
		return res
	case "agent.op.response":
		res.Class = RenderAction
		res.Text = renderOpResponse(ev.Data)
		return res

	// Host-side helpers (user-facing, compact).
	case "run.warning":
		res.Class = RenderAction
		if txt := strings.TrimSpace(ev.Data["text"]); txt != "" {
			res.Text = "Warning: " + txt
		} else {
			res.Text = "Warning"
		}
		return res
	case "refs.attached":
		res.Class = RenderAction
		files := strings.TrimSpace(ev.Data["files"])
		if files == "" {
			res.Text = "Attached referenced files"
		} else {
			res.Text = "Attached " + files
		}
		return res
	case "refs.ambiguous":
		res.Class = RenderAction
		tok := strings.TrimSpace(ev.Data["token"])
		cands := strings.TrimSpace(ev.Data["candidates"])
		if tok != "" && cands != "" {
			res.Text = "Ambiguous @" + tok + " (candidates: " + cands + ")"
		} else {
			res.Text = "Ambiguous @reference"
		}
		return res
	case "refs.unresolved":
		res.Class = RenderAction
		toks := strings.TrimSpace(ev.Data["tokens"])
		if toks != "" {
			res.Text = "Unresolved @references: " + toks
		} else {
			res.Text = "Unresolved @references"
		}
		return res
	case "artifact.published":
		res.Class = RenderAction
		src := strings.TrimSpace(ev.Data["source"])
		dst := strings.TrimSpace(ev.Data["dest"])
		if src != "" && dst != "" {
			res.Text = "Published " + src + " → " + dst
		} else {
			res.Text = "Published artifact to workdir"
		}
		return res

	case "ui.editor.open":
		res.Class = RenderAction
		p := strings.TrimSpace(ev.Data["path"])
		if p == "" {
			p = strings.TrimSpace(ev.Data["vpath"])
		}
		if p != "" {
			res.Text = "Edit " + p
		} else {
			res.Text = "Open editor"
		}
		return res
	case "ui.editor.error":
		res.Class = RenderAction
		if e := strings.TrimSpace(ev.Data["err"]); e != "" {
			res.Text = "Editor error: " + e
		} else {
			res.Text = "Editor error"
		}
		return res
	case "ui.open.ok":
		res.Class = RenderAction
		p := strings.TrimSpace(ev.Data["path"])
		if p != "" {
			res.Text = "Opened " + p
		} else {
			res.Text = "Opened file"
		}
		return res
	case "ui.open.error":
		res.Class = RenderAction
		p := strings.TrimSpace(ev.Data["path"])
		e := strings.TrimSpace(ev.Data["err"])
		if p != "" && e != "" {
			res.Text = "Open failed: " + p + " (" + e + ")"
		} else if e != "" {
			res.Text = "Open failed: " + e
		} else {
			res.Text = "Open failed"
		}
		return res

	case "workdir.changed":
		res.Class = RenderAction
		from := strings.TrimSpace(ev.Data["from"])
		to := strings.TrimSpace(ev.Data["to"])
		if from != "" && to != "" {
			res.Text = "Workdir changed: " + from + " → " + to
		} else if to != "" {
			res.Text = "Workdir: " + to
		} else {
			res.Text = "Workdir changed"
		}
		return res
	case "workdir.pwd":
		// Do not render workdir into the transcript. Workdir is displayed in the header,
		// and /pwd is often invoked automatically during pre-init.
		res.Class = RenderIgnore
		wd := strings.TrimSpace(ev.Data["workdir"])
		if wd == "" {
			res.Text = "Workdir"
		} else {
			res.Text = "Workdir: " + wd
		}
		return res
	case "workdir.error":
		res.Class = RenderAction
		if e := strings.TrimSpace(ev.Data["err"]); e != "" {
			res.Text = "Workdir change failed: " + e
		} else {
			res.Text = "Workdir change failed"
		}
		return res

	// Telemetry/Outcome are never rendered into the chat transcript.
	// The inspector/details view still receives the raw JSON lines.
	case "context.update", "context.constructor":
		res.Class = RenderTelemetry
		return res
	case "llm.usage.total":
		res.Class = RenderTelemetry
		return res
	case "llm.cost.total":
		res.Class = RenderTelemetry
		return res

	// Outcomes: commit/eval/audit summaries.
	case "memory.evaluate", "memory.commit", "memory.audit.append":
		res.Class = RenderOutcome
		return res
	case "profile.evaluate", "profile.commit", "profile.audit.append":
		res.Class = RenderOutcome
		return res
	case "agent.turn.complete":
		res.Class = RenderOutcome
		return res
	case "agent.error":
		res.Class = RenderOutcome
		return res
	}

	// Default: show only in details.
	res.Class = RenderIgnore
	return res
}

func renderOpRequest(d map[string]string) string {
	op := strings.TrimSpace(d["op"])

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
	default:
		if isSharedOpRequestTitleOp(op) {
			return opmeta.FormatRequestTitle(d)
		}
		return compactKV(d, []string{"op", "path"})
	}
}

func isSharedOpRequestTitleOp(op string) bool {
	switch strings.TrimSpace(op) {
	case "fs_list", "fs_read", "fs_search", "fs_write", "fs_append", "fs_edit", "fs_patch", "shell_exec", "http_fetch", "trace_run":
		return true
	default:
		return false
	}
}

func listPreview(items []string, maxItems int) string {
	if len(items) == 0 {
		return "[]"
	}
	if maxItems <= 0 || len(items) <= maxItems {
		b, _ := json.Marshal(items)
		return string(b)
	}
	trim := append([]string{}, items[:maxItems]...)
	trim = append(trim, "…")
	b, _ := json.Marshal(trim)
	return string(b)
}

func renderOpResponse(d map[string]string) string {
	op := strings.TrimSpace(d["op"])
	ok := strings.TrimSpace(d["ok"])
	errStr := strings.TrimSpace(d["err"])

	prefix := "✓"
	if ok != "true" {
		prefix = "✗"
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
	default:
		return "Action"
	}
}

func renderTurnComplete(d map[string]string) string {
	turn := strings.TrimSpace(d["turn"])
	steps := strings.TrimSpace(d["steps"])
	duration := strings.TrimSpace(d["duration"])
	if duration == "" {
		duration = strings.TrimSpace(d["durationMs"]) + "ms"
	}
	out := []string{}
	if turn != "" {
		out = append(out, "Turn "+turn)
	}
	if steps != "" {
		out = append(out, steps+" steps")
	}
	if duration != "" && duration != "ms" {
		out = append(out, duration)
	}
	if len(out) == 0 {
		return "Done"
	}
	return "Outcome: " + strings.Join(out, " • ")
}

func renderMemoryOutcome(label string, d map[string]string) string {
	accepted := strings.TrimSpace(d["accepted"])
	reason := strings.TrimSpace(d["reason"])
	b := strings.TrimSpace(d["bytes"])

	switch accepted {
	case "true":
		if b != "" {
			return fmt.Sprintf("%s: accepted (%sB)", label, b)
		}
		return label + ": accepted"
	case "false":
		if reason == "" {
			reason = "rejected"
		}
		if b != "" && b != "0" {
			return fmt.Sprintf("%s: %s (%sB)", label, reason, b)
		}
		return fmt.Sprintf("%s: %s", label, reason)
	default:
		return label + ": " + compactKV(d, nil)
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
