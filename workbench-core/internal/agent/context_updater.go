package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// ContextUpdater keeps the model's bounded context synchronized with persistent
// context sources (like /memory) and runtime sources (like /trace).
//
// This is the "v0" implementation:
//   - it refreshes context once per model step
//   - it tracks a byte offset into /trace so it only loads new events
//   - it injects small bounded excerpts into the system prompt
//   - it writes a context manifest for transparency/debugging
type ContextUpdater struct {
	FS *vfs.FS

	// LastOp/LastResp are the most recent host op request/response observed by the host.
	//
	// These are optional, but enable deterministic, adaptive context policies:
	//   - bump trace budget after failures
	//   - include a "last op" summary so the model doesn't repeat work
	//
	// The host should update these by calling ObserveHostOp after executing an op.
	LastOp   *types.HostOpRequest
	LastResp *types.HostOpResponse

	// LastToolRun captures the most recent tool.run call (if any).
	LastToolRun *LastToolRun

	// TraceOffset is the current byte offset into /trace/events.jsonl used by
	// events.since/<offset>.
	TraceOffset int64

	// MaxMemoryBytes caps how many bytes from /memory/memory.md are injected.
	// If zero, a default is used.
	MaxMemoryBytes int

	// MaxTraceBytes caps how many bytes from /trace/events.since/<offset> are injected.
	// If zero, a default is used.
	MaxTraceBytes int

	// ManifestPath is the VFS path where the updater writes its last manifest.
	// If empty, no manifest is written.
	ManifestPath string

	// Emit is an optional hook for recording updater actions (events/telemetry).
	Emit func(eventType, message string, data map[string]string)

	// TraceIncludeTypes is an allowlist of event types that are relevant to agent reasoning.
	// If empty, a default allowlist is used.
	TraceIncludeTypes []string

	// MaxTraceEvents caps the number of recent trace events held in memory for summarization.
	// If zero, a default is used.
	MaxTraceEvents int

	traceEvents []types.Event
}

type LastToolRun struct {
	ToolID   types.ToolID `json:"toolId"`
	ActionID string       `json:"actionId"`
	CallID   string       `json:"callId"`
}

// ContextPolicy is the per-step, deterministic decision describing what context to inject.
//
// This is recorded into context_manifest.json for auditability.
type ContextPolicy struct {
	Step int `json:"step"`

	TraceOffsetBefore int64 `json:"traceOffsetBefore"`
	TraceOffsetAfter  int64 `json:"traceOffsetAfter"`

	LastOp    *types.HostOpRequest  `json:"lastOp,omitempty"`
	LastResp  *types.HostOpResponse `json:"lastResp,omitempty"`
	LastError string                `json:"lastError,omitempty"`

	LastToolRun *LastToolRun `json:"lastToolRun,omitempty"`

	Budgets struct {
		MemoryBytes int `json:"memoryBytes"`
		TraceBytes  int `json:"traceBytes"`
	} `json:"budgets"`

	TraceIncludeTypes []string `json:"traceIncludeTypes"`
	FailureBump       bool     `json:"failureBump"`
}

type ContextManifest struct {
	UpdatedAt string `json:"updatedAt"`
	Step      int    `json:"step"`

	Policy ContextPolicy `json:"policy"`

	Memory struct {
		Path          string `json:"path"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		BudgetBytes   int    `json:"budgetBytes"`
	} `json:"memory"`

	Trace struct {
		Path          string `json:"path"`
		ReadMode      string `json:"readMode"` // "since" or "latest"
		ReadError     string `json:"readError,omitempty"`
		OffsetBefore  int64  `json:"offsetBefore"`
		OffsetAfter   int64  `json:"offsetAfter"`
		BytesRead     int    `json:"bytesRead"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		BudgetBytes   int    `json:"budgetBytes"`

		Events struct {
			LinesTotal     int `json:"linesTotal"`
			Parsed         int `json:"parsed"`
			ParseErrors    int `json:"parseErrors"`
			Selected       int `json:"selected"`
			SelectedCapped int `json:"selectedCapped"`
			Excluded       int `json:"excluded"`
		} `json:"events"`
	} `json:"trace"`
}

// ObserveHostOp records the most recent host op request/response so the next
// context update can adapt budgets deterministically.
func (u *ContextUpdater) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
	if u == nil {
		return
	}
	reqCopy := req
	respCopy := resp
	u.LastOp = &reqCopy
	u.LastResp = &respCopy

	if req.Op == "tool.run" && resp.ToolResponse != nil {
		u.LastToolRun = &LastToolRun{
			ToolID:   req.ToolID,
			ActionID: req.ActionID,
			CallID:   resp.ToolResponse.CallID,
		}
	}
}

// BuildSystemPrompt returns a base system prompt augmented with bounded context excerpts,
// and a manifest describing what was loaded.
func (u *ContextUpdater) BuildSystemPrompt(ctx context.Context, basePrompt string, step int) (string, ContextManifest, error) {
	if u == nil || u.FS == nil {
		return "", ContextManifest{}, fmt.Errorf("context updater FS is required")
	}

	maxMem := u.MaxMemoryBytes
	if maxMem == 0 {
		maxMem = 8 * 1024
	}
	maxTrace := u.MaxTraceBytes
	if maxTrace == 0 {
		maxTrace = 8 * 1024
	}
	maxEvents := u.MaxTraceEvents
	if maxEvents == 0 {
		maxEvents = 500
	}

	manifest := ContextManifest{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Step:      step,
	}

	policy := u.computePolicy(step, maxMem, maxTrace)
	manifest.Policy = policy

	// Memory excerpt (tail-biased).
	memPath := "/memory/memory.md"
	memBytes, memErr := u.FS.Read(memPath)
	if memErr != nil {
		memBytes = []byte{}
	}
	memIncl, memTrunc := tailUTF8(memBytes, policy.Budgets.MemoryBytes)
	manifest.Memory.Path = memPath
	manifest.Memory.BytesTotal = len(memBytes)
	manifest.Memory.BytesIncluded = len(memIncl)
	manifest.Memory.Truncated = memTrunc
	manifest.Memory.BudgetBytes = policy.Budgets.MemoryBytes

	// Trace excerpt (incremental since offset -> parsed -> filtered -> condensed).
	traceMode, tracePath, traceBytes, traceRead, offsetBefore, offsetAfter, traceErr := u.readTraceSince(ctx, policy.TraceOffsetBefore)
	u.TraceOffset = offsetAfter
	manifest.Policy.TraceOffsetAfter = offsetAfter
	manifest.Trace.Path = tracePath
	manifest.Trace.ReadMode = traceMode
	manifest.Trace.OffsetBefore = offsetBefore
	manifest.Trace.OffsetAfter = offsetAfter
	manifest.Trace.BytesRead = traceRead
	if traceErr != nil {
		manifest.Trace.ReadError = traceErr.Error()
	}

	linesTotal, parsed, parseErrors, newEvents := parseEventJSONL(traceBytes)
	manifest.Trace.Events.LinesTotal = linesTotal
	manifest.Trace.Events.Parsed = parsed
	manifest.Trace.Events.ParseErrors = parseErrors

	if traceMode == "since" {
		u.traceEvents = append(u.traceEvents, newEvents...)
		if len(u.traceEvents) > maxEvents {
			u.traceEvents = u.traceEvents[len(u.traceEvents)-maxEvents:]
		}
	} else if traceMode == "latest" {
		u.traceEvents = newEvents
		if len(u.traceEvents) > maxEvents {
			u.traceEvents = u.traceEvents[len(u.traceEvents)-maxEvents:]
		}
	}

	traceSummary, selected, capped, excluded, trunc := summarizeTrace(u.traceEvents, policy.TraceIncludeTypes, policy.Budgets.TraceBytes)
	manifest.Trace.Events.Selected = selected
	manifest.Trace.Events.SelectedCapped = capped
	manifest.Trace.Events.Excluded = excluded
	manifest.Trace.BytesIncluded = len([]byte(traceSummary))
	manifest.Trace.Truncated = trunc
	manifest.Trace.BudgetBytes = policy.Budgets.TraceBytes

	system := strings.TrimSpace(basePrompt)
	if len(memIncl) > 0 {
		system = system + "\n\n" + "## Persistent Memory (/memory/memory.md)\n\n" + string(memIncl) + "\n"
	}
	if policy.LastOp != nil {
		system = system + "\n\n" + "## Last Host Op\n\n" + summarizeLastOp(policy) + "\n"
	}
	if strings.TrimSpace(traceSummary) != "" {
		system = system + "\n\n" + "## Recent Ops (from /trace)\n\n" + traceSummary + "\n"
	}
	system = strings.TrimSpace(system)

	if u.ManifestPath != "" {
		b, err := json.MarshalIndent(manifest, "", "  ")
		if err == nil {
			_ = u.FS.Write(u.ManifestPath, b)
		}
	}

	if u.Emit != nil {
		u.Emit("context.update", "Context updated", map[string]string{
			"step":             strconv.Itoa(step),
			"memoryBytes":      strconv.Itoa(manifest.Memory.BytesIncluded),
			"traceBytes":       strconv.Itoa(manifest.Trace.BytesIncluded),
			"traceOffsetAfter": strconv.FormatInt(manifest.Trace.OffsetAfter, 10),
		})
	}

	_ = ctx
	return system, manifest, nil
}

func headUTF8(b []byte, max int) ([]byte, bool) {
	if max <= 0 || len(b) <= max {
		return b, false
	}
	out := b[:max]
	// First try trimming from the end (most common when the cut splits a rune).
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[:len(out)-1]
	}
	if len(out) > 0 && utf8.Valid(out) {
		return out, true
	}

	// If the prefix begins with invalid bytes, trimming the end won't help.
	// Fall back to trimming from the start until the remaining slice is valid UTF-8.
	out = b[:max]
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[1:]
	}
	return out, true
}

func tailUTF8(b []byte, max int) ([]byte, bool) {
	if max <= 0 || len(b) <= max {
		return b, false
	}
	start := len(b) - max
	out := b[start:]
	// First try trimming from the start (most common when the cut splits a rune).
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[1:]
	}
	if len(out) > 0 && utf8.Valid(out) {
		return out, true
	}

	// If the suffix ends with invalid bytes, trimming the start won't help.
	// Fall back to trimming from the end until valid.
	out = b[start:]
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[:len(out)-1]
	}
	return out, true
}

// Ensure ContextUpdater is only used with the agent loop types.
var _ = types.HostOpRequest{}

func (u *ContextUpdater) computePolicy(step int, baseMem, baseTrace int) ContextPolicy {
	p := ContextPolicy{Step: step}
	p.TraceOffsetBefore = u.TraceOffset
	p.TraceIncludeTypes = u.TraceIncludeTypes
	if len(p.TraceIncludeTypes) == 0 {
		p.TraceIncludeTypes = defaultTraceIncludeTypes()
	}

	// Budgets:
	// - step 1: trace 2x, memory 1x
	// - steps 2+: trace 1x, memory 0.5x
	memBudget := baseMem
	traceBudget := baseTrace
	if step == 1 {
		traceBudget = baseTrace * 2
	} else {
		memBudget = baseMem / 2
	}

	lastFailed := false
	if u.LastResp != nil && (!u.LastResp.Ok || u.LastResp.Truncated) {
		lastFailed = true
		traceBudget = baseTrace * 2
	}
	p.FailureBump = lastFailed

	p.Budgets.MemoryBytes = clampBudget(memBudget, 0, baseMem*2)
	p.Budgets.TraceBytes = clampBudget(traceBudget, 0, baseTrace*4)

	p.LastOp = u.LastOp
	p.LastResp = u.LastResp
	p.LastToolRun = u.LastToolRun
	if u.LastResp != nil && !u.LastResp.Ok {
		p.LastError = strings.TrimSpace(u.LastResp.Error)
	}
	return p
}

func clampBudget(v, min, max int) int {
	if v < min {
		return min
	}
	if max > 0 && v > max {
		return max
	}
	return v
}

func defaultTraceIncludeTypes() []string {
	return []string{
		"agent.op.request",
		"agent.op.response",
		"agent.error",
		"memory.evaluate",
		"memory.commit",
		"context.update",
	}
}

type traceSinceReader interface {
	ReadEventsSince(offset int64) ([]byte, int64, error)
}

type traceLatestReader interface {
	ReadLastEvents(count int) ([]byte, error)
}

func (u *ContextUpdater) readTraceSince(ctx context.Context, offset int64) (mode, path string, b []byte, bytesRead int, offsetBefore, offsetAfter int64, err error) {
	_ = ctx
	offsetBefore = offset

	// Prefer calling into the underlying trace resource directly so we can get nextOffset
	// even in error cases (e.g. "requested exceeds max").
	_, r, _, resErr := u.FS.Resolve("/trace")
	if resErr == nil {
		if tr, ok := r.(traceSinceReader); ok {
			path = "/trace/events.since/" + strconv.FormatInt(offset, 10)
			mode = "since"
			out, next, readErr := tr.ReadEventsSince(offset)
			if readErr == nil {
				return mode, path, out, len(out), offsetBefore, next, nil
			}
			// Fall back to latest snapshot when the since window is too large.
			if lr, ok := r.(traceLatestReader); ok {
				mode = "latest"
				path = "/trace/events.latest/200"
				out2, err2 := lr.ReadLastEvents(200)
				if err2 != nil {
					return mode, path, []byte{}, 0, offsetBefore, next, fmt.Errorf("events.latest fallback failed: %w", err2)
				}
				return mode, path, out2, len(out2), offsetBefore, next, fmt.Errorf("events.since failed: %w", readErr)
			}
			// No latest fallback available; still advance offset to next.
			return mode, path, []byte{}, 0, offsetBefore, next, fmt.Errorf("events.since failed: %w", readErr)
		}
	}

	// Fallback: read via VFS only (no nextOffset available).
	mode = "since"
	path = "/trace/events.since/" + strconv.FormatInt(offset, 10)
	out, readErr := u.FS.Read(path)
	if readErr != nil {
		return mode, path, []byte{}, 0, offsetBefore, offsetBefore, readErr
	}
	return mode, path, out, len(out), offsetBefore, offsetBefore + int64(len(out)), nil
}

func parseEventJSONL(b []byte) (linesTotal, parsed, parseErrors int, events []types.Event) {
	if len(b) == 0 {
		return 0, 0, 0, nil
	}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	// Scanner default token limit is 64K; our trace is capped at 64K for since reads,
	// and latest is bounded by count, so this is safe.
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		linesTotal++
		var ev types.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			parseErrors++
			continue
		}
		parsed++
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		// Treat scanner errors as parse errors for accounting.
		parseErrors++
	}
	return linesTotal, parsed, parseErrors, events
}

func summarizeTrace(all []types.Event, includeTypes []string, budgetBytes int) (summary string, selected, capped, excluded int, truncated bool) {
	include := make(map[string]bool, len(includeTypes))
	for _, t := range includeTypes {
		include[t] = true
	}

	filtered := make([]types.Event, 0, len(all))
	for _, ev := range all {
		if include[ev.Type] {
			filtered = append(filtered, ev)
		} else {
			excluded++
		}
	}
	selected = len(filtered)

	lines := make([]string, 0, len(filtered))
	for _, ev := range filtered {
		lines = append(lines, summarizeTraceEvent(ev))
	}

	// Build from the end to keep the most recent events within budget.
	var kept []string
	bytesUsed := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		add := len([]byte(line)) + 1 // + newline
		if budgetBytes > 0 && bytesUsed+add > budgetBytes {
			truncated = true
			break
		}
		kept = append(kept, line)
		bytesUsed += add
	}
	// Reverse to chronological order (most recent last).
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	capped = len(kept)
	if len(kept) == 0 {
		return "", selected, capped, excluded, truncated
	}

	var b strings.Builder
	b.WriteString("Recent Ops (most recent last):\n")
	for _, line := range kept {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), selected, capped, excluded, truncated
}

func summarizeTraceEvent(ev types.Event) string {
	switch ev.Type {
	case "agent.op.request":
		return "op.request " + kv(ev.Data, "op", "path", "toolId", "actionId")
	case "agent.op.response":
		return "op.response " + kv(ev.Data, "op", "ok", "err", "bytesLen", "truncated", "callId")
	case "agent.error":
		return "agent.error " + kv(ev.Data, "err")
	case "memory.evaluate":
		return "memory.evaluate " + kv(ev.Data, "accepted", "reason", "bytes", "sha256", "turn")
	case "memory.commit":
		return "memory.commit " + kv(ev.Data, "bytes", "sha256", "turn")
	case "context.update":
		return "context.update " + kv(ev.Data, "step", "memoryBytes", "traceBytes", "traceOffsetAfter")
	default:
		// Generic fallback.
		if ev.Message != "" {
			return ev.Type + " " + ev.Message
		}
		return ev.Type
	}
}

func kv(m map[string]string, keys ...string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	return strings.Join(parts, " ")
}

func summarizeLastOp(p ContextPolicy) string {
	if p.LastOp == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("request: op=")
	b.WriteString(p.LastOp.Op)
	if p.LastOp.Path != "" {
		b.WriteString(" path=")
		b.WriteString(p.LastOp.Path)
	}
	if p.LastOp.ToolID.String() != "" {
		b.WriteString(" toolId=")
		b.WriteString(p.LastOp.ToolID.String())
	}
	if p.LastOp.ActionID != "" {
		b.WriteString(" actionId=")
		b.WriteString(p.LastOp.ActionID)
	}
	if p.LastResp != nil {
		b.WriteString("\nresponse: ok=")
		b.WriteString(strconv.FormatBool(p.LastResp.Ok))
		if p.LastResp.Error != "" {
			b.WriteString(" err=")
			b.WriteString(p.LastResp.Error)
		}
		if p.LastResp.Truncated {
			b.WriteString(" truncated=true")
		}
	}
	if p.LastToolRun != nil && p.LastToolRun.CallID != "" {
		b.WriteString("\nlastToolRun: toolId=")
		b.WriteString(p.LastToolRun.ToolID.String())
		b.WriteString(" actionId=")
		b.WriteString(p.LastToolRun.ActionID)
		b.WriteString(" callId=")
		b.WriteString(p.LastToolRun.CallID)
	}
	if p.FailureBump {
		b.WriteString("\npolicy: failure bump active")
	}
	return strings.TrimSpace(b.String())
}
