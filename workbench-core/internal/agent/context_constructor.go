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

	"github.com/tinoosan/workbench-core/internal/jsonutil"
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

	// RunID/SessionID identify the current execution scope.
	RunID     string
	SessionID string

	TraceStore   store.TraceStore
	HistoryStore store.HistoryStore

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

	traceCursor   store.TraceCursor
	historyCursor store.HistoryCursor

	stateLoaded bool
	traceEvents []types.Event
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

	// Pull session-scoped history cursor from session.json (persisted across runs).
	if c.SessionID != "" {
		if sess, err := store.LoadSession(c.SessionID); err == nil {
			if strings.TrimSpace(sess.HistoryCursor) != "" {
				c.historyCursor = store.HistoryCursor(strings.TrimSpace(sess.HistoryCursor))
			}
		}
	}

	// Budgets (deterministic defaults).
	profileBudget := clampBudget(orDefault(c.MaxProfileBytes, 4*1024), 0, 8*1024)
	memBudget := clampBudget(orDefault(c.MaxMemoryBytes, 8*1024), 0, 16*1024)
	traceBudget := clampBudget(orDefault(c.MaxTraceBytes, 8*1024), 0, 64*1024)
	histBudget := clampBudget(orDefault(c.MaxHistoryBytes, 8*1024), 0, 64*1024)

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

	system := strings.TrimSpace(basePrompt)

	// Profile (global).
	profilePath := "/profile/profile.md"
	profileBytes, _ := c.FS.Read(profilePath)
	profileIncl, profileTrunc := tailUTF8(profileBytes, profileBudget)
	if len(profileIncl) > 0 {
		system += "\n\n## User Profile (/profile/profile.md)\n\n" + string(profileIncl) + "\n"
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
		BytesTotal:    len(profileBytes),
		BytesIncluded: len(profileIncl),
		Truncated:     profileTrunc,
		Reason:        "global user facts/preferences",
	})

	// Memory (run-scoped).
	memPath := "/memory/memory.md"
	memBytes, _ := c.FS.Read(memPath)
	memIncl, memTrunc := tailUTF8(memBytes, memBudget)
	if len(memIncl) > 0 {
		system += "\n\n## Run Memory (/memory/memory.md)\n\n" + string(memIncl) + "\n"
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
		BytesTotal:    len(memBytes),
		BytesIncluded: len(memIncl),
		Truncated:     memTrunc,
		Reason:        "run-scoped working memory",
	})

	// Last op snapshot (cheap, high-signal).
	if c.LastOp != nil {
		system += "\n\n## Last Host Op\n\n" + c.summarizeLastOp(failureBump) + "\n"
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
				system += "\n\n## Recent Ops (from /trace)\n\n" + summary + "\n"
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
				system += "\n\n" + blk + "\n"
			}
			// Persist session-level history cursor.
			if c.SessionID != "" {
				if sess, err := store.LoadSession(c.SessionID); err == nil {
					sess.HistoryCursor = string(c.historyCursor)
					_ = store.SaveSession(sess)
				}
			}
		}
	}

	system = strings.TrimSpace(system)

	_ = c.saveState(ctx)
	_ = c.writeManifest(ctx, manifest)

	if c.Emit != nil {
		c.Emit("context.constructor", "Constructed context", map[string]string{
			"step":          strconv.Itoa(step),
			"profileBytes":  strconv.Itoa(len(profileIncl)),
			"memoryBytes":   strconv.Itoa(len(memIncl)),
			"traceCursor":   string(c.traceCursor),
			"historyCursor": string(c.historyCursor),
		})
	}

	return system, nil
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
	b, err := jsonutil.MarshalPretty(st)
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
	b, err := jsonutil.MarshalPretty(m)
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
