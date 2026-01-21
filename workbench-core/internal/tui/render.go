package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/events"
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
	path := strings.TrimSpace(d["path"])
	toolID := strings.TrimSpace(d["toolId"])
	actionID := strings.TrimSpace(d["actionId"])
	input := strings.TrimSpace(d["input"])

	switch op {
	case "fs.list":
		return "List " + path
	case "fs.read":
		return "Read " + path
	case "fs.write":
		return "Write " + path
	case "fs.append":
		return "Append " + path
	case "fs.edit":
		return "Edit " + path
	case "fs.patch":
		return "Patch " + path
	case "shell_exec":
		if cmd := strings.TrimSpace(d["argvPreview"]); cmd != "" {
			return cmd
		}
		if argv0 := strings.TrimSpace(d["argv0"]); argv0 != "" {
			return argv0
		}
		return "shell_exec"
	case "http_fetch":
		u := strings.TrimSpace(d["url"])
		if u != "" {
			m := strings.ToUpper(strings.TrimSpace(d["method"]))
			if m == "" {
				m = "GET"
			}
			return m + " " + u
		}
		return "http_fetch"
	case "trace":
		action := strings.TrimSpace(d["traceAction"])
		key := strings.TrimSpace(d["traceKey"])
		if action != "" {
			if key != "" {
				return fmt.Sprintf("trace.%s %s", action, key)
			}
			return "trace." + action
		}
		return "trace"
	case "tool.run":
		// Chat transcript should read like a narrative: show the effective command rather
		// than the internal toolId/actionId + input payload.
		return renderToolRunTranscript(toolID, actionID, input)
	default:
		return compactKV(d, []string{"op", "path", "toolId", "actionId"})
	}
}

func renderToolRunTranscript(toolID, actionID, input string) string {
	switch toolID {
	case "builtin.shell":
		if actionID != "exec" {
			return fmt.Sprintf("%s/%s", toolID, actionID)
		}
		var in struct {
			Argv []string `json:"argv"`
		}
		if err := json.Unmarshal([]byte(input), &in); err != nil {
			return fmt.Sprintf("%s/%s", toolID, actionID)
		}
		// Show the command the tool will run (argv-joined with quoting).
		if cmd := shellJoin(in.Argv); cmd != "" {
			return cmd
		}
		return fmt.Sprintf("%s/%s", toolID, actionID)

	case "builtin.http":
		if actionID != "fetch" {
			return fmt.Sprintf("%s/%s", toolID, actionID)
		}
		var in struct {
			URL    string `json:"url"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal([]byte(input), &in); err != nil {
			return fmt.Sprintf("%s/%s", toolID, actionID)
		}
		u := strings.TrimSpace(in.URL)
		if u == "" {
			return fmt.Sprintf("%s/%s", toolID, actionID)
		}
		m := strings.ToUpper(strings.TrimSpace(in.Method))
		if m == "" {
			m = "GET"
		}
		return m + " " + u

	case "builtin.trace":
		return fmt.Sprintf("builtin.trace/%s", actionID)
	}

	// Default: don't leak opaque tool inputs into chat.
	return fmt.Sprintf("%s/%s", toolID, actionID)
}

func renderToolRunInspector(toolID, actionID, input string) string {
	base := fmt.Sprintf("Run %s/%s", toolID, actionID)
	if input == "" {
		return base
	}

	switch toolID {
	case "builtin.shell":
		if actionID != "exec" {
			break
		}
		var in struct {
			Argv []string `json:"argv"`
			Cwd  string   `json:"cwd"`
		}
		if err := json.Unmarshal([]byte(input), &in); err != nil {
			break
		}
		cmd := shellJoin(in.Argv)
		cwd := strings.TrimSpace(in.Cwd)
		if cwd == "" {
			cwd = "."
		}
		if cmd != "" {
			return fmt.Sprintf("%s argv=%s cwd=%s cmd=%s", base, listPreview(in.Argv, 6), cwd, truncateRight(cmd, 120))
		}
		return fmt.Sprintf("%s argv=%s cwd=%s", base, listPreview(in.Argv, 6), cwd)

	case "builtin.http":
		if actionID != "fetch" {
			break
		}
		var in struct {
			URL      string `json:"url"`
			Method   string `json:"method"`
			MaxBytes int    `json:"maxBytes"`
		}
		if err := json.Unmarshal([]byte(input), &in); err != nil {
			break
		}
		u := strings.TrimSpace(in.URL)
		m := strings.ToUpper(strings.TrimSpace(in.Method))
		if m == "" {
			m = "GET"
		}
		if u == "" {
			break
		}
		if in.MaxBytes > 0 {
			return fmt.Sprintf("%s %s %s maxBytes=%d", base, m, truncateRight(u, 140), in.MaxBytes)
		}
		return fmt.Sprintf("%s %s %s", base, m, truncateRight(u, 160))
	case "builtin.trace":
		return base
	}

	// Generic fallback: show a compact input preview.
	return fmt.Sprintf("%s input=%s", base, truncateRight(input, 160))
}

func quoteShort(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return `""`
	}
	if maxLen > 0 && len(s) > maxLen {
		s = s[:maxLen-1] + "…"
	}
	return strconv.Quote(s)
}

func shellJoin(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	parts := make([]string, 0, len(argv))
	for _, a := range argv {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return `""`
	}
	// Keep common CLI tokens unquoted for readability.
	if isShellSafeToken(s) {
		return s
	}
	return strconv.Quote(s)
}

func isShellSafeToken(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("._-/=:+@", r):
		default:
			return false
		}
	}
	return true
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
	case "fs.read":
		tr := strings.TrimSpace(d["truncated"])
		if ok == "true" && tr == "true" {
			return prefix + " truncated"
		}
		if ok != "true" && errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " ok"

	case "tool.run":
		callID := strings.TrimSpace(d["callId"])
		short := callID
		if len(short) > 8 {
			short = short[:8]
		}
		if ok != "true" && errStr != "" {
			return prefix + " " + errStr
		}
		if callID != "" {
			return prefix + " call=" + short
		}
		return prefix + " ok"

	default:
		if errStr != "" && ok != "true" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
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
