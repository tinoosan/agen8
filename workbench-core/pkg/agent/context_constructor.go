package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/debuglog"
	"github.com/tinoosan/workbench-core/pkg/skills"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// ContextConstructor selects, prioritizes, and compresses context from persistent stores.
type ContextConstructor struct {
	FS *vfs.FS

	Cfg config.Config

	RunID     string
	SessionID string

	LoadSession func(sessionID string) (types.Session, error)
	SaveSession func(session types.Session) error
	StateStore  store.ConstructorStateStore

	Trace         *TraceMiddleware
	HistoryStore  store.HistoryReader
	SkillsManager *skills.Manager

	IncludeHistoryOps bool

	MaxProfileBytes int
	MaxMemoryBytes  int
	MaxTraceBytes   int
	MaxHistoryBytes int

	TraceIncludeTypes []string

	Emit func(eventType, message string, data map[string]string)

	LastOp      *types.HostOpRequest
	LastResp    *types.HostOpResponse
	LastToolRun *LastToolRun

	FileAttachments []FileAttachment

	historyCursor store.HistoryCursor

	stateLoaded bool

	cache contextCache

	sessionStateLoaded bool

	sessionCached   types.Session
	sessionCachedOK bool
	sessionCachedID string

	lastPersistedHistoryCursor store.HistoryCursor
}

type contextCache struct {
	basePromptHash uint64

	profileReady       bool
	profileSection     string
	profileBudgetBytes int
	profileBytesTotal  int
	profileBytesIncl   int
	profileBytesTrunc  bool

	memoryReady       bool
	memorySection     string
	memoryBudgetBytes int
	memoryBytesTotal  int
	memoryBytesIncl   int
	memoryBytesTrunc  bool

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

// historyLine is the minimal schema emitted by HistorySink and exposed at /history/history.jsonl.
type historyLine struct {
	Timestamp string            `json:"ts"`
	RunID     string            `json:"runId"`
	Origin    string            `json:"origin"`
	Kind      string            `json:"kind"`
	Message   string            `json:"message"`
	Model     string            `json:"model,omitempty"`
	Data      map[string]string `json:"data,omitempty"`
}

// SystemPrompt renders the combined context block for a given agent step.
func (c *ContextConstructor) SystemPrompt(ctx context.Context, basePrompt string, step int) (string, error) {
	if c == nil {
		return basePrompt, nil
	}
	if c.FS == nil {
		return basePrompt, fmt.Errorf("context constructor missing FS")
	}
	if strings.TrimSpace(c.RunID) == "" {
		return basePrompt, fmt.Errorf("context constructor missing RunID")
	}
	if strings.TrimSpace(c.SessionID) == "" {
		return basePrompt, fmt.Errorf("context constructor missing SessionID")
	}

	basePrompt = strings.TrimSpace(basePrompt)
	if basePrompt == "" {
		basePrompt = agentLoopV0SystemPrompt()
	}

	if err := c.ensureStateLoaded(ctx); err != nil {
		return basePrompt, err
	}

	traceSummary, traceSelected, traceCapped, traceExcluded, traceTrunc := c.traceSummary()
	historySummary, historySelected, historyCapped, historyExcluded, historyTrunc, err := c.historySummary(ctx)
	if err != nil {
		return basePrompt, err
	}

	profileSection, profileTotals := c.profileSection()
	memorySection, memoryTotals := c.memorySection()
	attachSection, attachTotals := c.attachmentsSection()
	skillsSection, skillsTotals := c.skillsSection()
	activeSkillSection, activeSkillTotals := c.activeSkillSection(ctx)

	sections := []string{
		basePrompt,
		activeSkillSection,
		profileSection,
		memorySection,
		attachSection,
		skillsSection,
		traceSummary,
		historySummary,
	}

	out := strings.TrimSpace(strings.Join(nonEmpty(sections), "\n\n")) + "\n"
	if c.Emit != nil {
		c.Emit("context.constructor", "Context constructor updated", map[string]string{
			"step":              strconv.Itoa(step),
			"profile.bytes":     profileTotals,
			"memory.bytes":      memoryTotals,
			"attachments.bytes": attachTotals,
			"skills.bytes":      skillsTotals,
			"active_skill.bytes": activeSkillTotals,
			"trace.selected":    strconv.Itoa(traceSelected),
			"trace.capped":      strconv.Itoa(traceCapped),
			"trace.excluded":    strconv.Itoa(traceExcluded),
			"trace.truncated":   strconv.FormatBool(traceTrunc),
			"history.selected":  strconv.Itoa(historySelected),
			"history.capped":    strconv.Itoa(historyCapped),
			"history.excluded":  strconv.Itoa(historyExcluded),
			"history.truncated": strconv.FormatBool(historyTrunc),
		})
	}

	if c.StateStore != nil {
		if err := c.writeManifest(step, traceSelected, traceCapped, traceExcluded, traceTrunc, historySelected, historyCapped, historyExcluded, historyTrunc); err != nil {
			return out, err
		}
	}
	return out, nil
}

func (c *ContextConstructor) activeSkillSection(ctx context.Context) (section string, totals string) {
	if c == nil || c.SkillsManager == nil {
		return "", "0"
	}
	if !c.sessionCachedOK || c.sessionCachedID != c.SessionID {
		if err := c.loadSessionState(ctx); err != nil {
			return "", "0"
		}
	}
	skill := strings.TrimSpace(c.sessionCached.SelectedSkill)
	if skill == "" {
		return "", "0"
	}
	entry, ok := c.SkillsManager.Get(skill)
	if !ok || entry == nil || strings.TrimSpace(entry.Path) == "" {
		return "", "0"
	}
	b, err := os.ReadFile(filepath.Join(entry.Path, "SKILL.md"))
	if err != nil {
		return "", "0"
	}
	content := strings.TrimSpace(string(b))
	if content == "" {
		return "", "0"
	}
	section = strings.TrimSpace(strings.Join([]string{
		"<active_skill>",
		"### " + skill,
		content,
		"</active_skill>",
	}, "\n"))
	return section, fmt.Sprintf("%d", len(content))
}

// ObserveHostOp records the most recent host op request/response for adaptive context.
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

func (c *ContextConstructor) traceSummary() (summary string, selected, capped, excluded int, truncated bool) {
	if c.Trace == nil {
		return "", 0, 0, 0, false
	}
	return c.Trace.Summary(c.TraceIncludeTypes, c.MaxTraceBytes)
}

func (c *ContextConstructor) historySummary(ctx context.Context) (summary string, selected, capped, excluded int, truncated bool, err error) {
	if c.HistoryStore == nil {
		return "", 0, 0, 0, false, nil
	}
	if !c.sessionStateLoaded {
		if err := c.loadSessionState(ctx); err != nil {
			return "", 0, 0, 0, false, err
		}
	}
	maxBytes := c.MaxHistoryBytes
	if maxBytes <= 0 {
		maxBytes = 10 * 1024
	}
	opts := store.HistorySinceOptions{
		MaxBytes: maxBytes,
		Limit:    0,
	}
	batch, err := c.HistoryStore.LinesSince(ctx, c.historyCursor, opts)
	if err != nil {
		return "", 0, 0, 0, false, err
	}
	lines, selected, capped, excluded, truncated := summarizeHistory(batch, c.IncludeHistoryOps)
	c.historyCursor = batch.CursorAfter

	if err := c.persistHistoryCursor(ctx); err != nil {
		return "", 0, 0, 0, false, err
	}

	return lines, selected, capped, excluded, truncated, nil
}

func (c *ContextConstructor) profileSection() (section string, totals string) {
	if c.MaxProfileBytes == 0 {
		return "", "0"
	}
	if c.cache.profileReady && c.cache.profileBudgetBytes == c.MaxProfileBytes {
		return c.cache.profileSection, fmt.Sprintf("%d/%d", c.cache.profileBytesIncl, c.cache.profileBytesTotal)
	}
	b, total, incl, trunc := c.readCap("/profile/profile.md", c.MaxProfileBytes)
	c.cache.profileReady = true
	c.cache.profileSection = fmt.Sprintf("## Profile\n\n%s", b)
	c.cache.profileBudgetBytes = c.MaxProfileBytes
	c.cache.profileBytesTotal = total
	c.cache.profileBytesIncl = incl
	c.cache.profileBytesTrunc = trunc
	return c.cache.profileSection, fmt.Sprintf("%d/%d", incl, total)
}

func (c *ContextConstructor) memorySection() (section string, totals string) {
	if c.MaxMemoryBytes == 0 {
		return "", "0"
	}
	if c.cache.memoryReady && c.cache.memoryBudgetBytes == c.MaxMemoryBytes {
		return c.cache.memorySection, fmt.Sprintf("%d/%d", c.cache.memoryBytesIncl, c.cache.memoryBytesTotal)
	}
	b, total, incl, trunc := c.readCap("/memory/memory.md", c.MaxMemoryBytes)
	c.cache.memoryReady = true
	c.cache.memorySection = fmt.Sprintf("## Memory\n\n%s", b)
	c.cache.memoryBudgetBytes = c.MaxMemoryBytes
	c.cache.memoryBytesTotal = total
	c.cache.memoryBytesIncl = incl
	c.cache.memoryBytesTrunc = trunc
	return c.cache.memorySection, fmt.Sprintf("%d/%d", incl, total)
}

func (c *ContextConstructor) attachmentsSection() (section string, totals string) {
	if len(c.FileAttachments) == 0 {
		return "", "0"
	}
	hash := attachmentsHash(c.FileAttachments)
	if c.cache.attachReady && c.cache.attachHash == hash {
		return c.cache.attachSection, totalsBytes(c.FileAttachments)
	}
	parts := []string{"## Attachments"}
	for _, a := range c.FileAttachments {
		parts = append(parts, fmt.Sprintf("### %s\n\n%s", a.DisplayName, a.Content))
	}
	section = strings.TrimSpace(strings.Join(parts, "\n\n"))
	c.cache.attachReady = true
	c.cache.attachHash = hash
	c.cache.attachSection = section
	return section, totalsBytes(c.FileAttachments)
}

// SetFileAttachments replaces the current attachment set for this turn.
func (c *ContextConstructor) SetFileAttachments(attachments []FileAttachment) {
	if c == nil {
		return
	}
	c.FileAttachments = attachments
	c.cache.attachReady = false
	c.cache.attachHash = 0
}

// ClearFileAttachments clears any cached attachments for the current turn.
func (c *ContextConstructor) ClearFileAttachments() {
	if c == nil {
		return
	}
	c.FileAttachments = nil
	c.cache.attachReady = false
	c.cache.attachHash = 0
}

func (c *ContextConstructor) skillsSection() (section string, totals string) {
	if c.SkillsManager == nil {
		return "", "0"
	}
	entries := c.SkillsManager.Entries()
	if len(entries) == 0 {
		return "", "0"
	}
	var total int
	out := []string{"## Skills"}
	for _, e := range entries {
		skillPath := ""
		if e.Skill != nil {
			skillPath = strings.TrimSpace(e.Skill.Path)
		}
		if skillPath == "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(skillPath, "SKILL.md"))
		if err != nil {
			continue
		}
		total += len(b)
		out = append(out, fmt.Sprintf("### %s\n\n%s", e.Dir, string(b)))
	}
	section = strings.TrimSpace(strings.Join(out, "\n\n"))
	return section, fmt.Sprintf("%d", total)
}

func (c *ContextConstructor) readCap(path string, maxBytes int) (string, int, int, bool) {
	b, err := c.FS.Read(path)
	if err != nil {
		return "", 0, 0, false
	}
	total := len(b)
	if maxBytes <= 0 || total <= maxBytes {
		return string(b), total, total, false
	}
	return string(b[:maxBytes]), total, maxBytes, true
}

func (c *ContextConstructor) ensureStateLoaded(ctx context.Context) error {
	if c.stateLoaded {
		return nil
	}
	c.stateLoaded = true
	if c.StateStore == nil {
		return nil
	}
	raw, err := c.StateStore.GetState(ctx, c.RunID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil
		}
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var st constructorState
	if err := json.Unmarshal(raw, &st); err != nil {
		return err
	}
	c.Trace.Cursor = st.TraceCursor
	c.historyCursor = st.HistoryCursor
	return nil
}

func (c *ContextConstructor) saveState() error {
	if c.StateStore == nil {
		return nil
	}
	st := constructorState{
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		RunID:         c.RunID,
		SessionID:     c.SessionID,
		TraceCursor:   c.Trace.Cursor,
		HistoryCursor: c.historyCursor,
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return c.StateStore.SetState(context.Background(), c.RunID, b)
}

func (c *ContextConstructor) writeManifest(step int, traceSelected, traceCapped, traceExcluded int, traceTrunc bool, historySelected, historyCapped, historyExcluded int, historyTrunc bool) error {
	if c.StateStore == nil {
		return nil
	}
	manifest := ConstructorManifest{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Step:      step,
		RunID:     c.RunID,
		SessionID: c.SessionID,
	}
	manifest.Policy.IncludeHistoryOps = c.IncludeHistoryOps
	manifest.Policy.Budgets.ProfileBytes = c.MaxProfileBytes
	manifest.Policy.Budgets.MemoryBytes = c.MaxMemoryBytes
	manifest.Policy.Budgets.TraceBytes = c.MaxTraceBytes
	manifest.Policy.Budgets.HistoryBytes = c.MaxHistoryBytes
	manifest.Policy.TraceIncludeTypes = c.TraceIncludeTypes

	manifest.Cursors.TraceBefore = c.Trace.Cursor
	manifest.Cursors.TraceAfter = c.Trace.Cursor
	manifest.Cursors.HistoryBefore = c.historyCursor
	manifest.Cursors.HistoryAfter = c.historyCursor

	manifest.Sources = append(manifest.Sources, buildSource("profile", "/profile/profile.md", c.cache.profileBytesTotal, c.cache.profileBytesIncl, c.cache.profileBytesTrunc, "limit"))
	manifest.Sources = append(manifest.Sources, buildSource("memory", "/memory/memory.md", c.cache.memoryBytesTotal, c.cache.memoryBytesIncl, c.cache.memoryBytesTrunc, "limit"))
	manifest.Sources = append(manifest.Sources, buildSource("trace", "/log/events.jsonl", traceSelected+traceExcluded, traceSelected, traceTrunc, "summary"))
	manifest.Sources = append(manifest.Sources, buildSource("history", "/history/history.jsonl", historySelected+historyExcluded, historySelected, historyTrunc, "summary"))

	manifest.References = buildRefs(c.FileAttachments)
	b, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return c.StateStore.SetManifest(context.Background(), c.RunID, b)
}

func buildSource(source, path string, total, incl int, trunc bool, reason string) struct {
	Source        string `json:"source"`
	Path          string `json:"path"`
	BytesTotal    int    `json:"bytesTotal"`
	BytesIncluded int    `json:"bytesIncluded"`
	Truncated     bool   `json:"truncated"`
	Reason        string `json:"reason"`
} {
	return struct {
		Source        string `json:"source"`
		Path          string `json:"path"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		Reason        string `json:"reason"`
	}{
		Source:        source,
		Path:          path,
		BytesTotal:    total,
		BytesIncluded: incl,
		Truncated:     trunc,
		Reason:        reason,
	}
}

func buildRefs(refs []FileAttachment) []struct {
	Token         string `json:"token"`
	Path          string `json:"path"`
	DisplayName   string `json:"displayName"`
	BytesTotal    int    `json:"bytesTotal"`
	BytesIncluded int    `json:"bytesIncluded"`
	Truncated     bool   `json:"truncated"`
	Reason        string `json:"reason"`
} {
	out := make([]struct {
		Token         string `json:"token"`
		Path          string `json:"path"`
		DisplayName   string `json:"displayName"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
		Reason        string `json:"reason"`
	}, 0, len(refs))
	for _, r := range refs {
		out = append(out, struct {
			Token         string `json:"token"`
			Path          string `json:"path"`
			DisplayName   string `json:"displayName"`
			BytesTotal    int    `json:"bytesTotal"`
			BytesIncluded int    `json:"bytesIncluded"`
			Truncated     bool   `json:"truncated"`
			Reason        string `json:"reason"`
		}{
			Token:         r.Token,
			Path:          r.VPath,
			DisplayName:   r.DisplayName,
			BytesTotal:    r.BytesTotal,
			BytesIncluded: r.BytesIncluded,
			Truncated:     r.Truncated,
			Reason:        "attachment",
		})
	}
	return out
}

func summarizeHistory(batch store.HistoryBatch, includeOps bool) (summary string, selected, capped, excluded int, truncated bool) {
	if len(batch.Lines) == 0 {
		return "", 0, 0, 0, false
	}
	lines := make([]string, 0, len(batch.Lines))
	for _, raw := range batch.Lines {
		var h historyLine
		if err := json.Unmarshal(raw, &h); err != nil {
			continue
		}
		if !includeOps && h.Origin == "host" {
			excluded++
			continue
		}
		parts := []string{h.Origin, h.Kind, h.Message}
		for k, v := range h.Data {
			parts = append(parts, k+": "+v)
		}
		lines = append(lines, strings.Join(parts, " | "))
		selected++
		if batch.ReturnedCapped && selected >= batch.Returned {
			capped++
			break
		}
	}
	truncated = batch.Truncated || batch.ReturnedCapped
	if len(lines) == 0 {
		return "", selected, capped, excluded, truncated
	}
	return "## Recent History\n\n" + strings.Join(lines, "\n"), selected, capped, excluded, truncated
}

func (c *ContextConstructor) loadSessionState(ctx context.Context) error {
	if c.LoadSession == nil {
		return fmt.Errorf("context constructor: LoadSession is required")
	}
	sess, err := c.LoadSession(c.SessionID)
	if err != nil {
		return err
	}
	c.sessionCached = sess
	c.sessionCachedOK = true
	c.sessionCachedID = sess.SessionID
	c.historyCursor = store.HistoryCursor(sess.HistoryCursor)
	c.sessionStateLoaded = true
	return nil
}

func (c *ContextConstructor) persistHistoryCursor(ctx context.Context) error {
	if !c.sessionCachedOK || c.sessionCachedID != c.SessionID {
		if err := c.loadSessionState(ctx); err != nil {
			return err
		}
	}
	if c.historyCursor == c.lastPersistedHistoryCursor {
		return nil
	}
	c.sessionCached.HistoryCursor = string(c.historyCursor)
	c.sessionCached.UpdatedAt = ptr(time.Now().UTC())
	if c.SaveSession == nil {
		return fmt.Errorf("context constructor: SaveSession is required")
	}
	if err := c.SaveSession(c.sessionCached); err != nil {
		return err
	}
	c.lastPersistedHistoryCursor = c.historyCursor
	return nil
}

func nonEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

func totalsBytes(refs []FileAttachment) string {
	var incl int
	var total int
	for _, r := range refs {
		incl += r.BytesIncluded
		total += r.BytesTotal
	}
	return fmt.Sprintf("%d/%d", incl, total)
}

func attachmentsHash(refs []FileAttachment) uint64 {
	var h uint64
	for _, r := range refs {
		h ^= uint64(len(r.VPath)) * 1099511628211
		h ^= uint64(len(r.Content)) * 1469598103934665603
	}
	return h
}

func ptr[T any](v T) *T { return &v }

func init() {
	if os.Getenv("WORKBENCH_CTX_DEBUG") != "" {
		debuglog.Log("context", "H12", "constructor", "debug_enabled", map[string]any{
			"stateStore":    "configured",
			"manifestStore": "configured",
		})
	}
}
