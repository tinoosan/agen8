package tui

import (
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/opcatalog"
	"github.com/tinoosan/agen8/pkg/opformat"
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

type eventRenderer struct {
	class     RenderClass
	formatter func(events.Event) string
}

var eventRenderers = map[string]eventRenderer{}
var eventRenderersMu sync.RWMutex

func RegisterEventRenderer(eventType string, class RenderClass, formatter func(events.Event) string) {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return
	}
	eventRenderersMu.Lock()
	defer eventRenderersMu.Unlock()
	eventRenderers[eventType] = eventRenderer{
		class:     class,
		formatter: formatter,
	}
}

func registeredEventRenderers() map[string]eventRenderer {
	eventRenderersMu.RLock()
	defer eventRenderersMu.RUnlock()
	out := make(map[string]eventRenderer, len(eventRenderers))
	for k, v := range eventRenderers {
		out[k] = v
	}
	return out
}

func init() {
	RegisterEventRenderer("host.mounted", RenderIgnore, nil)
	RegisterEventRenderer("run.started", RenderIgnore, nil)
	RegisterEventRenderer("run.completed", RenderIgnore, nil)
	RegisterEventRenderer("agent.loop.start", RenderIgnore, nil)
	RegisterEventRenderer("llm.usage", RenderIgnore, nil)
	RegisterEventRenderer("user.message", RenderIgnore, nil)
	RegisterEventRenderer("workdir.pwd", RenderIgnore, formatWorkdirPWDEventText)
	RegisterEventRenderer("agent.op.request", RenderAction, formatAgentOpRequestEventText)
	RegisterEventRenderer("agent.op.response", RenderAction, formatAgentOpResponseEventText)
	RegisterEventRenderer("run.warning", RenderAction, formatRunWarningEventText)
	RegisterEventRenderer("refs.attached", RenderAction, formatRefsAttachedEventText)
	RegisterEventRenderer("refs.ambiguous", RenderAction, formatRefsAmbiguousEventText)
	RegisterEventRenderer("refs.unresolved", RenderAction, formatRefsUnresolvedEventText)
	RegisterEventRenderer("artifact.published", RenderAction, formatArtifactPublishedEventText)
	RegisterEventRenderer("ui.editor.open", RenderAction, formatUIEditorOpenEventText)
	RegisterEventRenderer("ui.editor.error", RenderAction, formatUIEditorErrorEventText)
	RegisterEventRenderer("ui.open.ok", RenderAction, formatUIOpenOKEventText)
	RegisterEventRenderer("ui.open.error", RenderAction, formatUIOpenErrorEventText)
	RegisterEventRenderer("workdir.changed", RenderAction, formatWorkdirChangedEventText)
	RegisterEventRenderer("workdir.error", RenderAction, formatWorkdirErrorEventText)
	RegisterEventRenderer("context.update", RenderTelemetry, nil)
	RegisterEventRenderer("context.constructor", RenderTelemetry, nil)
	RegisterEventRenderer("llm.usage.total", RenderTelemetry, nil)
	RegisterEventRenderer("llm.cost.total", RenderTelemetry, nil)
	RegisterEventRenderer("memory.evaluate", RenderOutcome, nil)
	RegisterEventRenderer("memory.commit", RenderOutcome, nil)
	RegisterEventRenderer("memory.audit.append", RenderOutcome, nil)
	RegisterEventRenderer("profile.evaluate", RenderOutcome, nil)
	RegisterEventRenderer("profile.commit", RenderOutcome, nil)
	RegisterEventRenderer("profile.audit.append", RenderOutcome, nil)
	RegisterEventRenderer("agent.turn.complete", RenderOutcome, nil)
	RegisterEventRenderer("agent.error", RenderOutcome, nil)
}

// classifyEvent converts a Agen8 event into one of the chat-first presentation primitives.
//
// IMPORTANT: This does not expose any chain-of-thought. It only reflects observable activity.
func classifyEvent(ev events.Event) RenderResult {
	res := RenderResult{Raw: rawEventJSON(ev)}

	if (ev.Type == "agent.op.request" || ev.Type == "agent.op.response") &&
		shouldHideInboxOp(strings.TrimSpace(ev.Data["op"]), strings.TrimSpace(ev.Data["path"])) {
		res.Class = RenderIgnore
		return res
	}

	eventRenderersMu.RLock()
	renderer, ok := eventRenderers[ev.Type]
	eventRenderersMu.RUnlock()
	if !ok {
		res.Class = RenderIgnore
		return res
	}
	res.Class = renderer.class

	if renderer.formatter != nil {
		res.Text = renderer.formatter(ev)
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
	return opformat.FormatRequestText(d)
}

func renderOpResponse(d map[string]string) string {
	if v := strings.TrimSpace(d["responseText"]); v != "" {
		return v
	}
	return opformat.FormatResponseText(d)
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
	case "fs_list", "fs_stat", "fs_read":
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
	if category, ok := opcatalog.Category(trimmed); ok {
		return category
	}
	return "Action"
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
