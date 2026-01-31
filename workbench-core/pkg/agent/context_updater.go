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

	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// ContextUpdater keeps the model's bounded context synchronized with persistent
// context sources (like /memory) and runtime sources (like /log).
//
// This is the "v0" implementation:
//   - it refreshes context once per model step
//   - it tracks a byte offset into /log so it only loads new events
//   - it injects small bounded excerpts into the system prompt
//   - it writes a context manifest for transparency/debugging
type ContextUpdater struct {
	FS *vfs.FS

	// Trace provides shared cursor tracking and trace read fallbacks.
	Trace *TraceMiddleware

	// LastOp/LastResp are the most recent host op request/response observed by the host.
	LastOp   *types.HostOpRequest
	LastResp *types.HostOpResponse

	// LastToolRun captures the most recent tool.run call (if any).
	LastToolRun *LastToolRun

	// MaxMemoryBytes caps how many bytes from /memory/memory.md are injected.
	// If zero, a default is used.
	MaxMemoryBytes int

	// MaxProfileBytes caps how many bytes from /profile/profile.md are injected.
	// If zero, a default is used.
	MaxProfileBytes int

	// MaxTraceBytes caps how many bytes from /log/events.since/<offset> are injected.
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
}

type LastToolRun struct {
	ToolID   tools.ToolID `json:"toolId"`
	ActionID string       `json:"actionId"`
	CallID   string       `json:"callId"`
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

	LastToolRun *LastToolRun `json:"lastToolRun,omitempty"`

	Budgets struct {
		ProfileBytes int `json:"profileBytes"`
		MemoryBytes  int `json:"memoryBytes"`
		TraceBytes   int `json:"traceBytes"`
	} `json:"budgets"`

	TraceIncludeTypes []string `json:"traceIncludeTypes"`
	FailureBump       bool     `json:"failureBump"`
}

type ContextManifest struct {
	UpdatedAt string `json:"updatedAt"`
	Step      int    `json:"step"`

	Policy ContextPolicy `json:"policy"`

	Profile struct {
		Path          string `json:"path"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		BudgetBytes   int    `json:"budgetBytes"`
	} `json:"profile"`

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
func (u *ContextUpdater) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
	if u == nil {
		return
	}
	reqCopy := req
	respCopy := resp
	u.LastOp = &reqCopy
	u.LastResp = &respCopy

	if req.Op == types.HostOpToolRun && resp.ToolResponse != nil {
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
	maxProfile := u.MaxProfileBytes
	if maxProfile == 0 {
		maxProfile = 4 * 1024
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

	policy := u.computePolicy(step, maxProfile, maxMem, maxTrace)
	manifest.Policy = policy

	// Profile excerpt (tail-biased; global user preferences/facts).
	profilePath := "/profile/profile.md"
	profileBytes, profileErr := u.FS.Read(profilePath)
	if profileErr != nil {
		profileBytes = []byte{}
	}
	profileIncl, profileTrunc := tailUTF8(profileBytes, policy.Budgets.ProfileBytes)
	manifest.Profile.Path = profilePath
	manifest.Profile.BytesTotal = len(profileBytes)
	manifest.Profile.BytesIncluded = len(profileIncl)
	manifest.Profile.Truncated = profileTrunc
	manifest.Profile.BudgetBytes = policy.Budgets.ProfileBytes

	// Memory excerpt (tail-biased).
	todayName := time.Now().Format("2006-01-02") + "-memory.md"
	memPath := "/memory/" + todayName
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

	events := []types.Event(nil)
	if u.Trace != nil {
		u.Trace.MaxEvents = maxEvents
		events = u.Trace.ApplyBatch(traceMode, batch)
	}

	traceSummary, selected, capped, excluded, trunc := summarizeTrace(events, policy.TraceIncludeTypes, policy.Budgets.TraceBytes)
	manifest.Trace.Events.Selected = selected
	manifest.Trace.Events.SelectedCapped = capped
	manifest.Trace.Events.Excluded = excluded
	manifest.Trace.BytesIncluded = len([]byte(traceSummary))
	manifest.Trace.Truncated = trunc
	manifest.Trace.BudgetBytes = policy.Budgets.TraceBytes

	system := strings.TrimSpace(basePrompt)
	if len(profileIncl) > 0 {
		system += buildXMLBlock("user_profile", []xmlAttribute{
			{key: "path", value: "/profile/profile.md"},
			{key: "bytes_included", value: strconv.Itoa(len(profileIncl))},
			{key: "bytes_total", value: strconv.Itoa(len(profileBytes))},
		}, string(profileIncl))
	}
	if len(memIncl) > 0 {
		system += buildXMLBlock("run_memory", []xmlAttribute{
			{key: "path", value: memPath},
			{key: "bytes_included", value: strconv.Itoa(len(memIncl))},
			{key: "bytes_total", value: strconv.Itoa(len(memBytes))},
		}, string(memIncl))
	}
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
		u.Emit("context.update", "Context updated", map[string]string{
			"step":             strconv.Itoa(step),
			"profileBytes":     strconv.Itoa(manifest.Profile.BytesIncluded),
			"memoryBytes":      strconv.Itoa(manifest.Memory.BytesIncluded),
			"traceBytes":       strconv.Itoa(manifest.Trace.BytesIncluded),
			"traceOffsetAfter": string(manifest.Trace.CursorAfter),
		})
	}

	return system, manifest, nil
}

// SystemPrompt implements ContextSource by returning an augmented system prompt for a step.
func (u *ContextUpdater) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
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

// Ensure ContextUpdater is only used with the agent loop types.
var _ = types.HostOpRequest{}

func (u *ContextUpdater) computePolicy(step int, baseProfile, baseMem, baseTrace int) ContextPolicy {
	p := ContextPolicy{Step: step}
	if u.Trace != nil {
		p.TraceCursorBefore = u.Trace.ensureCursor()
	}
	p.TraceIncludeTypes = u.TraceIncludeTypes
	if len(p.TraceIncludeTypes) == 0 {
		p.TraceIncludeTypes = defaultTraceIncludeTypes()
	}

	profileBudget := baseProfile
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

	p.Budgets.ProfileBytes = clampBudget(profileBudget, 0, baseProfile*2)
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

func parseTypesEventJSONL(b []byte) (linesTotal, parsed, parseErrors int, events []types.Event) {
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
		var ev types.Event
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

func toTraceEvents(in []types.Event) []store.TraceEvent {
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

func toTypesEvents(in []store.TraceEvent) []types.Event {
	out := make([]types.Event, 0, len(in))
	for _, ev := range in {
		out = append(out, types.Event{
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
