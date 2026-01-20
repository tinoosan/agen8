package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// ContextConstructor selects, prioritizes, and compresses context from persistent stores
// (profile/memory/history) and runtime streams (trace) to produce a bounded context block.
//
// This is the "Context Constructor" adapted to Workbench:
//   - deterministic (no LLM calls)
//   - auditable (writes a manifest)
//   - scope-aware (run vs session)
//   - budgeted (hard caps in bytes)
//
// The Constructor is a ContextSource, so the Agent can call it once per step to build
// an augmented system prompt.
type ContextConstructor struct {
	FS *vfs.FS

	// Cfg is used for disk-backed host state (e.g. session.json history cursor).
	Cfg config.Config

	// RunID/SessionID identify the current execution scope.
	RunID     string
	SessionID string

	TraceStore   store.TraceStore
	HistoryStore store.HistoryReader

	// IncludeHistoryOps controls whether the constructor includes environment/host ops
	// from history in addition to user/agent messages.
	IncludeHistoryOps bool

	// Budgets.
	MaxProfileBytes int
	MaxMemoryBytes  int
	MaxTraceBytes   int
	MaxHistoryBytes int

	// TraceIncludeTypes filters which trace event types are considered "reasoning relevant".
	// If empty, the constructor uses a conservative default allowlist.
	TraceIncludeTypes []string

	// StatePath is the VFS path used to persist constructor state (run-scoped).
	// Example: "/workspace/context_constructor_state.json".
	StatePath string

	// ManifestPath is the VFS path where the constructor writes its last manifest.
	// Example: "/workspace/context_constructor_manifest.json".
	ManifestPath string

	// Emit is an optional hook for recording constructor actions (events/telemetry).
	Emit func(eventType, message string, data map[string]string)

	// Observations from the host loop (used to adapt budgets deterministically).
	LastOp      *types.HostOpRequest
	LastResp    *types.HostOpResponse
	LastToolRun *LastToolRun

	// FileAttachments are bounded file snapshots resolved by the host for the current
	// user turn (e.g. @go.mod, @cmd/workbench/main.go).
	//
	// The host is responsible for:
	// - resolving tokens to VFS paths
	// - bounding bytes per file / total bytes
	// - setting/clearing this slice per user turn
	FileAttachments []FileAttachment

	traceCursor   store.TraceCursor
	historyCursor store.HistoryCursor

	stateLoaded bool
	traceEvents []types.Event

	// cache holds per-turn cached prompt sections to avoid re-reading/re-formatting
	// stable inputs (profile/memory/attachments) on every model step.
	cache contextCache

	// sessionStateLoaded indicates whether we've loaded session-scoped state from disk
	// (session.json) into in-memory fields like historyCursor.
	sessionStateLoaded bool

	// sessionCached holds the last session.json we loaded, so we can update/persist
	// HistoryCursor without re-loading the session every step.
	sessionCached   types.Session
	sessionCachedOK bool
	sessionCachedID string

	// lastPersistedHistoryCursor tracks the last value we successfully persisted
	// into session.json, so we can avoid redundant writes.
	lastPersistedHistoryCursor store.HistoryCursor
}

type contextCache struct {
	// basePromptHash invalidates cached sections when the caller changes basePrompt.
	basePromptHash uint64

	// profile section cache
	profileReady       bool
	profileSection     string
	profileBudgetBytes int
	profileBytesTotal  int
	profileBytesIncl   int
	profileBytesTrunc  bool

	// memory section cache
	memoryReady       bool
	memorySection     string
	memoryBudgetBytes int
	memoryBytesTotal  int
	memoryBytesIncl   int
	memoryBytesTrunc  bool

	// attachments section cache
	attachReady   bool
	attachSection string
	attachHash    uint64
}

type constructorState struct {
	UpdatedAt     string              `json:"updatedAt"`
	RunID         string              `json:"runId,omitempty"`
	SessionID     string              `json:"sessionId,omitempty"`
	TraceCursor   store.TraceCursor   `json:"traceCursor"`
	HistoryCursor store.HistoryCursor `json:"historyCursor"`
}

// ConstructorManifest records what the constructor selected/excluded and why.
type ConstructorManifest struct {
	UpdatedAt string `json:"updatedAt"`
	Step      int    `json:"step"`

	RunID     string `json:"runId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`

	Policy struct {
		IncludeHistoryOps bool `json:"includeHistoryOps"`
		Budgets           struct {
			ProfileBytes int `json:"profileBytes"`
			MemoryBytes  int `json:"memoryBytes"`
			TraceBytes   int `json:"traceBytes"`
			HistoryBytes int `json:"historyBytes"`
		} `json:"budgets"`
		TraceIncludeTypes []string `json:"traceIncludeTypes"`
	} `json:"policy"`

	Cursors struct {
		TraceBefore   store.TraceCursor   `json:"traceBefore"`
		TraceAfter    store.TraceCursor   `json:"traceAfter"`
		HistoryBefore store.HistoryCursor `json:"historyBefore"`
		HistoryAfter  store.HistoryCursor `json:"historyAfter"`
	} `json:"cursors"`

	Sources []struct {
		Source        string `json:"source"`
		Path          string `json:"path"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		Reason        string `json:"reason"`
	} `json:"sources"`

	References []struct {
		Token         string `json:"token"`
		Path          string `json:"path"`
		DisplayName   string `json:"displayName"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		Reason        string `json:"reason"`
	} `json:"references,omitempty"`
}

// historyLine is the minimal schema written to /history/history.jsonl by HistorySink.
type historyLine struct {
	Timestamp string            `json:"ts"`
	RunID     string            `json:"runId"`
	Origin    string            `json:"origin"`
	Kind      string            `json:"kind"`
	Message   string            `json:"message"`
	Model     string            `json:"model,omitempty"`
	Data      map[string]string `json:"data,omitempty"`
}

// ObserveHostOp records the most recent host op request/response for adaptive decisions.
func (c *ContextConstructor) ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse) {
	if c == nil {
		return
	}
	reqCopy := req
	respCopy := resp
	c.LastOp = &reqCopy
	c.LastResp = &respCopy
	if req.Op == types.HostOpToolRun && resp.ToolResponse != nil {
		c.LastToolRun = &LastToolRun{
			ToolID:   req.ToolID,
			ActionID: req.ActionID,
			CallID:   resp.ToolResponse.CallID,
		}
	}
}

// SetFileAttachments sets the current turn's referenced file attachments.
//
// Callers should treat these as ephemeral: set them before running the agent loop
// for a user turn, and clear them after the turn completes.
func (c *ContextConstructor) SetFileAttachments(atts []FileAttachment) {
	if c == nil {
		return
	}
	if len(atts) == 0 {
		c.FileAttachments = nil
		return
	}
	c.FileAttachments = append([]FileAttachment(nil), atts...)
}

// ClearFileAttachments removes any current turn file attachments.
func (c *ContextConstructor) ClearFileAttachments() {
	if c == nil {
		return
	}
	c.FileAttachments = nil
}

// SystemPrompt implements ContextSource.
func (c *ContextConstructor) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	if c == nil || c.FS == nil {
		return "", fmt.Errorf("context constructor FS is required")
	}
	if step < 1 {
		step = 1
	}
	if err := c.loadStateIfNeeded(ctx); err != nil {
		return "", err
	}
	c.loadSessionStateIfNeeded()

	// Budgets (deterministic defaults).
	profileBudget := clampBudget(orDefault(c.MaxProfileBytes, 4*1024), 0, 8*1024)
	memBudget := clampBudget(orDefault(c.MaxMemoryBytes, 8*1024), 0, 16*1024)
	traceBudget := clampBudget(orDefault(c.MaxTraceBytes, 4*1024), 0, 64*1024)
	histBudget := clampBudget(orDefault(c.MaxHistoryBytes, 4*1024), 0, 64*1024)

	// Failure bump: if last host op failed or was truncated, prefer more trace/history.
	failureBump := false
	if c.LastResp != nil && (!c.LastResp.Ok || c.LastResp.Truncated) {
		failureBump = true
		traceBudget = clampBudget(traceBudget*2, 0, 64*1024)
		histBudget = clampBudget(histBudget*2, 0, 64*1024)
	}

	includeTypes := c.TraceIncludeTypes
	if len(includeTypes) == 0 {
		includeTypes = defaultTraceIncludeTypes()
	}

	manifest := ConstructorManifest{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Step:      step,
		RunID:     c.RunID,
		SessionID: c.SessionID,
	}
	manifest.Policy.IncludeHistoryOps = c.IncludeHistoryOps
	manifest.Policy.Budgets.ProfileBytes = profileBudget
	manifest.Policy.Budgets.MemoryBytes = memBudget
	manifest.Policy.Budgets.TraceBytes = traceBudget
	manifest.Policy.Budgets.HistoryBytes = histBudget
	manifest.Policy.TraceIncludeTypes = includeTypes
	manifest.Cursors.TraceBefore = c.traceCursor
	manifest.Cursors.HistoryBefore = c.historyCursor

	basePrompt = strings.TrimSpace(basePrompt)
	baseHash := hashString(basePrompt)

	// New turn (step==1) or base prompt changed => invalidate per-turn caches.
	if step == 1 || (c.cache.basePromptHash != 0 && c.cache.basePromptHash != baseHash) {
		c.cache = contextCache{}
		c.cache.basePromptHash = baseHash
	} else if c.cache.basePromptHash == 0 {
		c.cache.basePromptHash = baseHash
	}

	// Profile (global). Cache per turn to avoid re-reading/re-formatting on every step.
	profilePath := "/profile/profile.md"
	if step == 1 || !c.cache.profileReady || c.cache.profileBudgetBytes != profileBudget {
		profileBytes, _ := c.FS.Read(profilePath)
		profileIncl, profileTrunc := tailUTF8(profileBytes, profileBudget)
		c.cache.profileSection = ""
		if len(profileIncl) > 0 {
			c.cache.profileSection = "\n\n## User Profile (/profile/profile.md)\n\n" + string(profileIncl) + "\n"
		}
		c.cache.profileReady = true
		c.cache.profileBudgetBytes = profileBudget
		c.cache.profileBytesTotal = len(profileBytes)
		c.cache.profileBytesIncl = len(profileIncl)
		c.cache.profileBytesTrunc = profileTrunc
	}
	manifest.Sources = append(manifest.Sources, struct {
		Source        string `json:"source"`
		Path          string `json:"path"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		Reason        string `json:"reason"`
	}{
		Source:        "profile",
		Path:          profilePath,
		BytesTotal:    c.cache.profileBytesTotal,
		BytesIncluded: c.cache.profileBytesIncl,
		Truncated:     c.cache.profileBytesTrunc,
		Reason:        "global user facts/preferences",
	})

	// Memory (run-scoped). Cache per turn (commits happen after a turn completes).
	memPath := "/memory/memory.md"
	if step == 1 || !c.cache.memoryReady || c.cache.memoryBudgetBytes != memBudget {
		memBytes, _ := c.FS.Read(memPath)
		memIncl, memTrunc := tailUTF8(memBytes, memBudget)
		c.cache.memorySection = ""
		if len(memIncl) > 0 {
			c.cache.memorySection = "\n\n## Run Memory (/memory/memory.md)\n\n" + string(memIncl) + "\n"
		}
		c.cache.memoryReady = true
		c.cache.memoryBudgetBytes = memBudget
		c.cache.memoryBytesTotal = len(memBytes)
		c.cache.memoryBytesIncl = len(memIncl)
		c.cache.memoryBytesTrunc = memTrunc
	}
	manifest.Sources = append(manifest.Sources, struct {
		Source        string `json:"source"`
		Path          string `json:"path"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		Reason        string `json:"reason"`
	}{
		Source:        "memory",
		Path:          memPath,
		BytesTotal:    c.cache.memoryBytesTotal,
		BytesIncluded: c.cache.memoryBytesIncl,
		Truncated:     c.cache.memoryBytesTrunc,
		Reason:        "run-scoped working memory",
	})

	// Turn-scoped file attachments (resolved from @references in the user message).
	attachHash := hashAttachments(c.FileAttachments)
	if step == 1 || !c.cache.attachReady || attachHash != c.cache.attachHash {
		c.cache.attachHash = attachHash
		c.cache.attachSection = ""
		if len(c.FileAttachments) != 0 {
			var b strings.Builder
			b.WriteString("\n\n## Referenced Files\n\n")
			for _, att := range c.FileAttachments {
				name := strings.TrimSpace(att.DisplayName)
				if name == "" {
					name = strings.TrimSpace(att.Token)
				}
				if name == "" {
					name = "<file>"
				}
				b.WriteString("### ")
				b.WriteString(name)
				b.WriteString("\n\n")
				b.WriteString("- path: `")
				b.WriteString(strings.TrimSpace(att.VPath))
				b.WriteString("`\n")
				if att.BytesTotal != 0 {
					b.WriteString("- bytes: ")
					b.WriteString(strconv.Itoa(att.BytesIncluded))
					b.WriteString(" (of ")
					b.WriteString(strconv.Itoa(att.BytesTotal))
					b.WriteString(")\n")
				}
				if att.Truncated {
					b.WriteString("- truncated: true\n")
				}
				b.WriteString("\n")
				b.WriteString(fencedCodeForPath(att.VPath, att.Content))
				b.WriteString("\n")
			}
			c.cache.attachSection = b.String()
		}
		c.cache.attachReady = true
	}

	var systemB strings.Builder
	systemB.WriteString(basePrompt)
	systemB.WriteString(c.cache.profileSection)
	systemB.WriteString(c.cache.memorySection)
	systemB.WriteString(c.cache.attachSection)

	// Rebuild attachment references for the manifest (cheap, bounded slice).
	for _, att := range c.FileAttachments {
		manifest.References = append(manifest.References, struct {
			Token         string `json:"token"`
			Path          string `json:"path"`
			DisplayName   string `json:"displayName"`
			BytesTotal    int    `json:"bytesTotal"`
			BytesIncluded int    `json:"bytesIncluded"`
			Truncated     bool   `json:"truncated"`
			Reason        string `json:"reason"`
		}{
			Token:         strings.TrimSpace(att.Token),
			Path:          strings.TrimSpace(att.VPath),
			DisplayName:   strings.TrimSpace(att.DisplayName),
			BytesTotal:    att.BytesTotal,
			BytesIncluded: att.BytesIncluded,
			Truncated:     att.Truncated,
			Reason:        "user @reference attachment",
		})
	}

	// Last op snapshot (cheap, high-signal).
	if c.LastOp != nil {
		systemB.WriteString("\n\n## Last Host Op\n\n")
		systemB.WriteString(c.summarizeLastOp(failureBump))
		systemB.WriteString("\n")
	}

	// Trace (run-scoped).
	if c.TraceStore != nil {
		if strings.TrimSpace(string(c.traceCursor)) == "" {
			c.traceCursor = store.TraceCursorFromInt64(0)
		}
		batch, err := c.TraceStore.EventsSince(ctx, c.traceCursor, store.TraceSinceOptions{MaxBytes: traceBudget, Limit: 200})
		if err == nil {
			c.traceCursor = batch.CursorAfter
			manifest.Cursors.TraceAfter = batch.CursorAfter
			evs := toTypesEvents(batch.Events)
			c.traceEvents = append(c.traceEvents, evs...)
			if len(c.traceEvents) > 500 {
				c.traceEvents = c.traceEvents[len(c.traceEvents)-500:]
			}
			summary, _, _, _, _ := summarizeTrace(c.traceEvents, includeTypes, traceBudget)
			if strings.TrimSpace(summary) != "" {
				systemB.WriteString("\n\n## Recent Ops (from /trace)\n\n")
				systemB.WriteString(summary)
				systemB.WriteString("\n")
			}
		}
	}

	// History (session-scoped).
	if c.HistoryStore != nil {
		var hb store.HistoryBatch
		var err error
		if strings.TrimSpace(string(c.historyCursor)) == "" {
			hb, err = c.HistoryStore.LinesLatest(ctx, store.HistoryLatestOptions{MaxBytes: histBudget, Limit: 200})
		} else {
			hb, err = c.HistoryStore.LinesSince(ctx, c.historyCursor, store.HistorySinceOptions{MaxBytes: histBudget, Limit: 200})
		}
		if err == nil {
			c.historyCursor = hb.CursorAfter
			manifest.Cursors.HistoryAfter = hb.CursorAfter
			blk := c.recentHistoryBlock(hb.Lines, 8)
			if strings.TrimSpace(blk) != "" {
				systemB.WriteString("\n\n")
				systemB.WriteString(blk)
				systemB.WriteString("\n")
			}
			// Persist session-level history cursor (avoid redundant LoadSession per step).
			c.persistSessionHistoryCursorIfNeeded()
		}
	}

	system := strings.TrimSpace(systemB.String())

	_ = c.saveState(ctx)
	_ = c.writeManifest(ctx, manifest)

	if c.Emit != nil {
		c.Emit("context.constructor", "Constructed context", map[string]string{
			"step":          strconv.Itoa(step),
			"profileBytes":  strconv.Itoa(c.cache.profileBytesIncl),
			"memoryBytes":   strconv.Itoa(c.cache.memoryBytesIncl),
			"traceCursor":   string(c.traceCursor),
			"historyCursor": string(c.historyCursor),
		})
	}

	return system, nil
}

func fencedCodeForPath(vpath string, content string) string {
	lang := guessFenceLang(vpath)
	content = strings.TrimRight(content, "\n")
	// Avoid accidental fence termination in content; fall back to plain fence.
	if strings.Contains(content, "```") {
		lang = ""
	}
	if lang != "" {
		return "```" + lang + "\n" + content + "\n```"
	}
	return "```\n" + content + "\n```"
}

func guessFenceLang(vpath string) string {
	low := strings.ToLower(strings.TrimSpace(vpath))
	switch {
	case strings.HasSuffix(low, ".go"):
		return "go"
	case strings.HasSuffix(low, ".mod"):
		return "go"
	case strings.HasSuffix(low, ".sum"):
		return "txt"
	case strings.HasSuffix(low, ".json"):
		return "json"
	case strings.HasSuffix(low, ".yaml"), strings.HasSuffix(low, ".yml"):
		return "yaml"
	case strings.HasSuffix(low, ".md"):
		return "md"
	case strings.HasSuffix(low, ".sh"):
		return "sh"
	case strings.HasSuffix(low, ".ts"):
		return "ts"
	case strings.HasSuffix(low, ".js"):
		return "js"
	case strings.HasSuffix(low, ".html"), strings.HasSuffix(low, ".htm"):
		return "html"
	default:
		return ""
	}
}

func (c *ContextConstructor) summarizeLastOp(failureBump bool) string {
	op := "<none>"
	if c.LastOp != nil {
		op = c.LastOp.Op
		if strings.TrimSpace(c.LastOp.Path) != "" {
			op += " path=" + c.LastOp.Path
		}
		if c.LastOp.ToolID.String() != "" {
			op += " toolId=" + c.LastOp.ToolID.String()
		}
		if strings.TrimSpace(c.LastOp.ActionID) != "" {
			op += " actionId=" + c.LastOp.ActionID
		}
	}
	resp := "<none>"
	if c.LastResp != nil {
		resp = "ok=" + fmtBool(c.LastResp.Ok)
		if strings.TrimSpace(c.LastResp.Error) != "" {
			resp += " err=" + oneLineClamp(c.LastResp.Error, 120)
		}
		if c.LastResp.Truncated {
			resp += " truncated=true"
		}
	}
	out := "- request: " + op + "\n" + "- response: " + resp
	if failureBump {
		out += "\n" + "- policy: failure bump active"
	}
	return out
}

func (c *ContextConstructor) recentHistoryBlock(lines [][]byte, maxPairs int) string {
	var msgs []string
	for _, raw := range lines {
		var hl historyLine
		if err := json.Unmarshal(bytes.TrimSpace(raw), &hl); err != nil {
			continue
		}
		kind := strings.TrimSpace(hl.Kind)
		switch kind {
		case "user.message":
			text := ""
			if hl.Data != nil {
				text = strings.TrimSpace(hl.Data["text"])
			}
			if text == "" {
				text = strings.TrimSpace(hl.Message)
			}
			if text != "" {
				msgs = append(msgs, "user: "+oneLineClamp(text, 160))
			}
		case "agent.final":
			text := ""
			if hl.Data != nil {
				text = strings.TrimSpace(hl.Data["text"])
			}
			if text == "" {
				text = strings.TrimSpace(hl.Message)
			}
			if text != "" {
				msgs = append(msgs, "agent: "+oneLineClamp(text, 160))
			}
		case "agent.op.request", "agent.op.response":
			if !c.IncludeHistoryOps {
				continue
			}
			if hl.Data != nil {
				msgs = append(msgs, "env: "+kind+" "+oneLineClamp(mapSummary(hl.Data), 160))
			} else {
				msgs = append(msgs, "env: "+kind)
			}
		}
	}
	if len(msgs) == 0 {
		return ""
	}
	if maxPairs <= 0 {
		maxPairs = 8
	}
	maxLines := maxPairs * 2
	if c.IncludeHistoryOps {
		// Allow ops to show up without displacing all conversation pairs.
		maxLines = maxPairs*2 + 10
	}
	if len(msgs) > maxLines {
		msgs = msgs[len(msgs)-maxLines:]
	}
	return "## Recent Conversation (from /history)\n\n" + strings.Join(msgs, "\n")
}

func mapSummary(m map[string]string) string {
	// Stable-ish ordering is not required here; this is a tiny best-effort summary.
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

const (
	fnv64Offset = 14695981039346656037
	fnv64Prime  = 1099511628211
)

type fnv64a struct {
	h uint64
}

func newFNV64a() fnv64a {
	return fnv64a{h: fnv64Offset}
}

func (f *fnv64a) addByte(b byte) {
	f.h ^= uint64(b)
	f.h *= fnv64Prime
}

func (f *fnv64a) addString(s string) {
	for i := 0; i < len(s); i++ {
		f.addByte(s[i])
	}
}

func (f *fnv64a) addUint64(v uint64) {
	// Little-endian bytes to make the encoding stable.
	for i := 0; i < 8; i++ {
		f.addByte(byte(v))
		v >>= 8
	}
}

func hashString(s string) uint64 {
	f := newFNV64a()
	f.addString(s)
	return f.h
}

func hashAttachments(atts []FileAttachment) uint64 {
	f := newFNV64a()
	f.addUint64(uint64(len(atts)))
	for _, a := range atts {
		f.addString(a.Token)
		f.addByte(0)
		f.addString(a.VPath)
		f.addByte(0)
		f.addString(a.DisplayName)
		f.addByte(0)
		f.addUint64(uint64(a.BytesTotal))
		f.addUint64(uint64(a.BytesIncluded))
		if a.Truncated {
			f.addByte(1)
		} else {
			f.addByte(0)
		}
		f.addByte(0)
		// Content is bounded by the host (e.g. 12KB/file); include it so we detect
		// any mid-turn attachment refresh without relying on pointers.
		f.addString(a.Content)
		f.addByte(0)
	}
	return f.h
}

func (c *ContextConstructor) loadSessionStateIfNeeded() {
	if c == nil {
		return
	}
	sid := strings.TrimSpace(c.SessionID)
	if sid == "" {
		c.sessionStateLoaded = true
		c.sessionCachedOK = false
		c.sessionCachedID = ""
		return
	}
	if c.sessionStateLoaded && c.sessionCachedID == sid {
		return
	}
	c.sessionStateLoaded = true
	c.sessionCachedID = sid

	sess, err := store.LoadSession(c.Cfg, sid)
	if err != nil {
		c.sessionCachedOK = false
		return
	}
	c.sessionCached = sess
	c.sessionCachedOK = true

	// Prefer the session-scoped cursor when present (persisted across runs).
	if strings.TrimSpace(sess.HistoryCursor) != "" {
		c.historyCursor = store.HistoryCursor(strings.TrimSpace(sess.HistoryCursor))
	}
	c.lastPersistedHistoryCursor = store.HistoryCursor(strings.TrimSpace(sess.HistoryCursor))
}

func (c *ContextConstructor) persistSessionHistoryCursorIfNeeded() {
	if c == nil {
		return
	}
	sid := strings.TrimSpace(c.SessionID)
	if sid == "" {
		return
	}
	cur := strings.TrimSpace(string(c.historyCursor))
	last := strings.TrimSpace(string(c.lastPersistedHistoryCursor))
	if cur == last {
		return
	}
	// Ensure we have a session object to update and persist.
	if !c.sessionCachedOK || c.sessionCachedID != sid {
		c.sessionStateLoaded = false
		c.loadSessionStateIfNeeded()
	}
	if !c.sessionCachedOK {
		return
	}

	c.sessionCached.HistoryCursor = string(c.historyCursor)
	if err := store.SaveSession(c.Cfg, c.sessionCached); err == nil {
		c.lastPersistedHistoryCursor = c.historyCursor
	}
}

func (c *ContextConstructor) loadStateIfNeeded(ctx context.Context) error {
	if c.stateLoaded {
		return nil
	}
	c.stateLoaded = true
	if strings.TrimSpace(c.StatePath) == "" {
		c.StatePath = "/workspace/context_constructor_state.json"
	}
	b, err := c.FS.Read(c.StatePath)
	if err != nil || len(b) == 0 {
		return nil
	}
	var st constructorState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil
	}
	c.traceCursor = st.TraceCursor
	c.historyCursor = st.HistoryCursor
	return nil
}

func (c *ContextConstructor) saveState(ctx context.Context) error {
	_ = ctx
	if strings.TrimSpace(c.StatePath) == "" {
		return nil
	}
	st := constructorState{
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		RunID:         c.RunID,
		SessionID:     c.SessionID,
		TraceCursor:   c.traceCursor,
		HistoryCursor: c.historyCursor,
	}
	b, err := types.MarshalPretty(st)
	if err != nil {
		return err
	}
	return c.FS.Write(c.StatePath, b)
}

func (c *ContextConstructor) writeManifest(ctx context.Context, m ConstructorManifest) error {
	_ = ctx
	if strings.TrimSpace(c.ManifestPath) == "" {
		c.ManifestPath = "/workspace/context_constructor_manifest.json"
	}
	b, err := types.MarshalPretty(m)
	if err != nil {
		return err
	}
	return c.FS.Write(c.ManifestPath, b)
}

func orDefault(v int, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func oneLineClamp(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if max > 0 && len(s) > max {
		return s[:max] + "…"
	}
	return s
}
