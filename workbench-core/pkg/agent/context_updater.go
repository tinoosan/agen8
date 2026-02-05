package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// PromptUpdater keeps the model's bounded context synchronized with persistent
// context sources (like /memory) and runtime sources (like /log).
//
// This is the "v0" implementation:
//   - it refreshes context once per model step
//   - it tracks a byte offset into /log so it only loads new events
//   - it injects small bounded excerpts into the system prompt
//   - it writes a context manifest for transparency/debugging
type PromptUpdater struct {
	FS *vfs.FS

	// Trace provides shared cursor tracking and trace read fallbacks.
	Trace *TraceMiddleware

	// LastOp/LastResp are the most recent host op request/response observed by the host.
	LastOp   *types.HostOpRequest
	LastResp *types.HostOpResponse

	// MaxMemoryBytes caps how many bytes from today's /memory/<YYYY-MM-DD>-memory.md are injected.
	// If zero, a default is used.
	MaxMemoryBytes int

	// MaxTraceBytes caps how many bytes from /log/events.since/<offset> are injected.
	// If zero, a default is used.
	MaxTraceBytes int

	// ManifestPath is the VFS path where the updater writes its last manifest.
	// If empty, no manifest is written.
	ManifestPath string

	// Emit is an optional hook for recording updater actions (events/telemetry).
	Emit events.EmitFunc

	// TraceIncludeTypes is an allowlist of event types that are relevant to agent reasoning.
	// If empty, a default allowlist is used.
	TraceIncludeTypes []string

	// MaxTraceEvents caps the number of recent trace events held in memory for summarization.
	// If zero, a default is used.
	MaxTraceEvents int
}

// ContextPolicy is the per-step, deterministic decision describing what context to inject.
//
// This is recorded into context_manifest.json for auditability.
type ContextPolicy struct {
	Step int `json:"step"`

	TraceCursorBefore store.TraceCursor `json:"traceCursorBefore"`
	TraceCursorAfter  store.TraceCursor `json:"traceCursorAfter"`

	LastOp    *types.HostOpRequest  `json:"lastOp,omitempty"`
	LastResp  *types.HostOpResponse `json:"lastResp,omitempty"`
	LastError string                `json:"lastError,omitempty"`

	Budgets struct {
		MemoryBytes int `json:"memoryBytes"`
		TraceBytes  int `json:"traceBytes"`
	} `json:"budgets"`

	TraceIncludeTypes []string `json:"traceIncludeTypes"`
	FailureBump       bool     `json:"failureBump"`
}

type PromptManifest struct {
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
		Path          string            `json:"path"`
		ReadMode      string            `json:"readMode"` // "since" or "latest"
		ReadError     string            `json:"readError,omitempty"`
		CursorBefore  store.TraceCursor `json:"cursorBefore"`
		CursorAfter   store.TraceCursor `json:"cursorAfter"`
		BytesRead     int               `json:"bytesRead"`
		BytesIncluded int               `json:"bytesIncluded"`
		Truncated     bool              `json:"truncated"`
		BudgetBytes   int               `json:"budgetBytes"`

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
func (u *PromptUpdater) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
	if u == nil {
		return
	}
	reqCopy := req
	respCopy := resp
	u.LastOp = &reqCopy
	u.LastResp = &respCopy
}

// BuildSystemPrompt returns a base system prompt augmented with bounded context excerpts,
// and a manifest describing what was loaded.
func (u *PromptUpdater) BuildSystemPrompt(ctx context.Context, basePrompt string, step int) (string, PromptManifest, error) {
	if u == nil || u.FS == nil {
		return "", PromptManifest{}, fmt.Errorf("prompt updater FS is required")
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

	manifest := PromptManifest{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Step:      step,
	}

	policy := u.computePolicy(step, maxMem, maxTrace)
	manifest.Policy = policy

	// Memory excerpt (removed for token optimization: rely on tool-based retrieval).
	// manifest.Memory fields kept for struct compatibility but will be empty.

	// Trace excerpt (incremental since offset -> parsed -> filtered -> condensed).
	traceMode := "since"
	tracePath := "/log/events.jsonl"
	batch := store.TraceBatch{}
	cursorBefore := policy.TraceCursorBefore
	cursorAfter := policy.TraceCursorBefore
	var traceErr error
	if u.Trace != nil {
		traceMode, tracePath, batch, cursorBefore, cursorAfter, traceErr = u.Trace.ReadSince(ctx, store.TraceSinceOptions{
			MaxBytes: policy.Budgets.TraceBytes,
			Limit:    200,
		})
		manifest.Policy.TraceCursorAfter = cursorAfter
	} else {
		traceErr = fmt.Errorf("trace middleware not configured")
	}
	manifest.Trace.Path = tracePath
	manifest.Trace.ReadMode = traceMode
	manifest.Trace.CursorBefore = cursorBefore
	manifest.Trace.CursorAfter = cursorAfter
	manifest.Trace.BytesRead = batch.BytesRead
	if traceErr != nil {
		manifest.Trace.ReadError = traceErr.Error()
	}

	manifest.Trace.Events.LinesTotal = batch.LinesTotal
	manifest.Trace.Events.Parsed = batch.Parsed
	manifest.Trace.Events.ParseErrors = batch.ParseErrors

	traceEvents := []types.EventRecord(nil)
	if u.Trace != nil {
		u.Trace.MaxEvents = maxEvents
		traceEvents = u.Trace.ApplyBatch(traceMode, batch)
	}

	traceSummary, selected, capped, excluded, trunc := summarizeTrace(traceEvents, policy.TraceIncludeTypes, policy.Budgets.TraceBytes)
	manifest.Trace.Events.Selected = selected
	manifest.Trace.Events.SelectedCapped = capped
	manifest.Trace.Events.Excluded = excluded
	manifest.Trace.BytesIncluded = len([]byte(traceSummary))
	manifest.Trace.Truncated = trunc
	manifest.Trace.BudgetBytes = policy.Budgets.TraceBytes

	system := strings.TrimSpace(basePrompt)

	if policy.LastOp != nil {
		system += buildXMLBlock("last_host_op", nil, summarizeLastOp(policy))
	}
	if strings.TrimSpace(traceSummary) != "" {
		system += buildXMLBlock("recent_ops", nil, traceSummary)
	}
	system = strings.TrimSpace(system)

	if u.ManifestPath != "" {
		b, err := types.MarshalPretty(manifest)
		if err == nil {
			_ = u.FS.Write(u.ManifestPath, b)
		}
	}

	if u.Emit != nil {
		u.Emit(ctx, events.Event{
			Type:    "context.update",
			Message: "Context updated",
			Origin:  "env",
			Data: map[string]string{
				"step":             strconv.Itoa(step),
				"memoryBytes":      strconv.Itoa(manifest.Memory.BytesIncluded),
				"traceBytes":       strconv.Itoa(manifest.Trace.BytesIncluded),
				"traceOffsetAfter": string(manifest.Trace.CursorAfter),
			},
		})
	}

	return system, manifest, nil
}

// SystemPrompt implements PromptSource by returning an augmented system prompt for a step.
func (u *PromptUpdater) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	s, _, err := u.BuildSystemPrompt(ctx, basePrompt, step)
	return s, err
}

func headUTF8(b []byte, max int) ([]byte, bool) {
	if max <= 0 || len(b) <= max {
		return b, false
	}
	out := b[:max]
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[:len(out)-1]
	}
	if len(out) > 0 && utf8.Valid(out) {
		return out, true
	}

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
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[1:]
	}
	if len(out) > 0 && utf8.Valid(out) {
		return out, true
	}

	out = b[start:]
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[:len(out)-1]
	}
	return out, true
}

// Ensure PromptUpdater is only used with the agent loop types.
var _ = types.HostOpRequest{}

func (u *PromptUpdater) computePolicy(step int, baseMem, baseTrace int) ContextPolicy {
	p := ContextPolicy{Step: step}
	if u.Trace != nil {
		p.TraceCursorBefore = u.Trace.ensureCursor()
	}
	p.TraceIncludeTypes = u.TraceIncludeTypes
	if len(p.TraceIncludeTypes) == 0 {
		p.TraceIncludeTypes = defaultTraceIncludeTypes()
	}

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

func parseTypesEventJSONL(b []byte) (linesTotal, parsed, parseErrors int, events []types.EventRecord) {
	if len(b) == 0 {
		return 0, 0, 0, nil
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		linesTotal++
		var ev types.EventRecord
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			parseErrors++
			continue
		}
		parsed++
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil && !errors.Is(err, io.EOF) {
		parseErrors++
	}
	return linesTotal, parsed, parseErrors, events
}

func toTraceEvents(in []types.EventRecord) []store.TraceEvent {
	out := make([]store.TraceEvent, 0, len(in))
	for _, ev := range in {
		out = append(out, store.TraceEvent{
			Timestamp: ev.Timestamp.UTC().Format(time.RFC3339Nano),
			Type:      ev.Type,
			Message:   ev.Message,
			Data:      ev.Data,
		})
	}
	return out
}

func toTypesEvents(in []store.TraceEvent) []types.EventRecord {
	out := make([]types.EventRecord, 0, len(in))
	for _, ev := range in {
		out = append(out, types.EventRecord{
			Timestamp: parseRFC3339Time(ev.Timestamp),
			Type:      ev.Type,
			Message:   ev.Message,
			Data:      ev.Data,
		})
	}
	return out
}

func parseRFC3339Time(s string) time.Time {
	if strings.TrimSpace(s) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func summarizeTrace(all []types.EventRecord, includeTypes []string, budgetBytes int) (summary string, selected, capped, excluded int, truncated bool) {
	include := make(map[string]bool, len(includeTypes))
	for _, t := range includeTypes {
		include[t] = true
	}

	filtered := make([]types.EventRecord, 0, len(all))
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

	var kept []string
	bytesUsed := 0
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		add := len([]byte(line)) + 1
		if budgetBytes > 0 && bytesUsed+add > budgetBytes {
			truncated = true
			break
		}
		kept = append(kept, line)
		bytesUsed += add
	}
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

func summarizeTraceEvent(ev types.EventRecord) string {
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
	if p.FailureBump {
		b.WriteString("\npolicy: failure bump active")
	}
	return strings.TrimSpace(b.String())
}
