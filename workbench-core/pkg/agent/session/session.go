package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent"
	agentevents "github.com/tinoosan/workbench-core/pkg/agent/events"
	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/profile"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type ResolveProfileFunc func(ref string) (*profile.Profile, string, error)

type Config struct {
	Agent agent.Agent

	Profile        *profile.Profile
	ProfileDir     string
	ResolveProfile ResolveProfileFunc

	Store  state.Store
	Events *agentevents.Writer

	Memory            agent.MemoryRecallProvider
	MemorySearchLimit int
	Notifier          agent.Notifier

	InboxPath    string
	OutboxPath   string
	PollInterval time.Duration
	MaxReadBytes int

	LeaseTTL    time.Duration
	LeaseExtend time.Duration

	MaxRetries int
	MaxPending int

	InstanceID string
	Logf       func(format string, args ...any)
}

type Session struct {
	cfg Config

	activeProfile    *profile.Profile
	activeProfileDir string
	activePromptText string

	// lastTaskOutcome is a short, single-line summary of the most recently completed task.
	// It is used to provide immediate continuity between sequential tasks in the run loop.
	lastTaskOutcome string

	hbCh   chan profile.HeartbeatJob
	hbStop context.CancelFunc

	queuedEmitted map[string]struct{}
}

func New(cfg Config) (*Session, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("agent is required")
	}
	if cfg.Profile == nil {
		return nil, fmt.Errorf("profile is required")
	}
	if cfg.Store == nil {
		return nil, fmt.Errorf("task state store is required")
	}
	if strings.TrimSpace(cfg.InboxPath) == "" {
		cfg.InboxPath = "/inbox"
	}
	if strings.TrimSpace(cfg.OutboxPath) == "" {
		cfg.OutboxPath = "/outbox"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.MaxReadBytes <= 0 {
		cfg.MaxReadBytes = 256 * 1024
	}
	if cfg.MemorySearchLimit <= 0 {
		cfg.MemorySearchLimit = 3
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 2 * time.Minute
	}
	if cfg.LeaseExtend <= 0 {
		cfg.LeaseExtend = cfg.LeaseTTL / 2
		if cfg.LeaseExtend <= 0 {
			cfg.LeaseExtend = 30 * time.Second
		}
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 50
	}
	if strings.TrimSpace(cfg.InstanceID) == "" {
		cfg.InstanceID = "instance"
	}

	s := &Session{cfg: cfg}
	s.queuedEmitted = make(map[string]struct{})
	if err := s.setProfile(cfg.Profile, cfg.ProfileDir); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Session) Run(ctx context.Context) error {
	if s == nil {
		return fmt.Errorf("session is nil")
	}
	_ = s.cfg.Store.RecoverExpired(ctx, time.Now().UTC())
	s.startHeartbeats(ctx)
	defer s.stopHeartbeats()

	basePoll := s.cfg.PollInterval
	if basePoll <= 0 {
		basePoll = 2 * time.Second
	}
	maxPoll := basePoll * 8
	if maxPoll < 10*time.Second {
		maxPoll = 10 * time.Second
	}
	poll := basePoll
	timer := time.NewTimer(poll)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case job := <-s.hbCh:
			s.handleHeartbeat(ctx, job)
		case <-timer.C:
			hadCandidates, err := s.drainInbox(ctx)
			if err != nil {
				s.logf("inbox drain error: %v", err)
				s.emitBestEffort(ctx, events.Event{Type: "daemon.error", Message: "Inbox drain error", Data: map[string]string{"error": err.Error()}})
			}
			if hadCandidates {
				poll = basePoll
			} else if poll < maxPoll {
				poll *= 2
				if poll > maxPoll {
					poll = maxPoll
				}
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(poll)
		}
	}
}

func (s *Session) emitBestEffort(ctx context.Context, ev events.Event) {
	if s == nil || s.cfg.Events == nil {
		return
	}
	_ = s.cfg.Events.Emit(ctx, ev)
}

func (s *Session) logf(format string, args ...any) {
	if s == nil || s.cfg.Logf == nil {
		return
	}
	s.cfg.Logf(format, args...)
}

func (s *Session) startHeartbeats(ctx context.Context) {
	s.stopHeartbeats()
	ch := make(chan profile.HeartbeatJob, 16)
	s.hbCh = ch
	hbCtx, cancel := context.WithCancel(ctx)
	s.hbStop = cancel

	// One ticker per job; fan-in into hbCh.
	for _, job := range s.activeProfile.Heartbeat {
		j := job
		if j.Interval <= 0 {
			continue
		}
		go func(out chan<- profile.HeartbeatJob) {
			t := time.NewTicker(j.Interval)
			defer t.Stop()
			for {
				select {
				case <-hbCtx.Done():
					return
				case <-t.C:
					select {
					case <-hbCtx.Done():
						return
					case out <- j:
					default:
						// drop if overwhelmed; next tick will retry
					}
				}
			}
		}(ch)
	}
}

func (s *Session) stopHeartbeats() {
	if s == nil {
		return
	}
	if s.hbStop != nil {
		s.hbStop()
		s.hbStop = nil
	}
	s.hbCh = nil
}

func (s *Session) setProfile(p *profile.Profile, dir string) error {
	if p == nil {
		return fmt.Errorf("profile is nil")
	}
	promptText := ""
	switch {
	case strings.TrimSpace(p.Prompts.SystemPrompt) != "":
		promptText = strings.TrimSpace(p.Prompts.SystemPrompt)
	case strings.TrimSpace(p.Prompts.SystemPromptPath) != "" && strings.TrimSpace(dir) != "":
		raw, err := os.ReadFile(filepath.Join(dir, p.Prompts.SystemPromptPath))
		if err != nil {
			return fmt.Errorf("read profile prompt: %w", err)
		}
		promptText = strings.TrimSpace(string(raw))
	}
	s.activeProfile = p
	s.activeProfileDir = strings.TrimSpace(dir)
	s.activePromptText = promptText
	return nil
}

func (s *Session) handleHeartbeat(ctx context.Context, job profile.HeartbeatJob) {
	// Backpressure: if inbox is already large, skip emitting more tasks.
	if count, err := s.inboxCount(ctx); err == nil && count > s.cfg.MaxPending {
		s.emitBestEffort(ctx, events.Event{
			Type:    "task.heartbeat.skipped",
			Message: "Heartbeat skipped due to backpressure",
			Data:    map[string]string{"profile": s.activeProfile.ID, "job": job.Name, "inboxCount": fmt.Sprintf("%d", count)},
		})
		return
	}

	now := time.Now().UTC()
	window := now.Truncate(job.Interval).Unix()
	taskID := fmt.Sprintf("heartbeat-%s-%s-%s-%d", s.activeProfile.ID, s.cfg.InstanceID, job.Name, window)

	if _, ok, _ := s.cfg.Store.Get(ctx, taskID); ok {
		return
	}

	task := types.Task{
		TaskID:    taskID,
		Goal:      strings.TrimSpace(job.Goal),
		Priority:  5,
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Metadata: map[string]any{
			"source":  "heartbeat",
			"profile": s.activeProfile.ID,
			"job":     strings.TrimSpace(job.Name),
			"window":  window,
		},
	}
	filename := taskID + ".json"
	taskPath := path.Join(strings.TrimRight(s.cfg.InboxPath, "/"), filename)
	if err := s.writeJSON(ctx, taskPath, task); err != nil {
		s.logf("heartbeat enqueue failed: %v", err)
		return
	}
	s.emitTaskQueuedOnce(ctx, taskID, task.Goal, "heartbeat")
	s.emitBestEffort(ctx, events.Event{
		Type:    "task.heartbeat.enqueued",
		Message: "Heartbeat task enqueued",
		Data:    map[string]string{"taskId": taskID, "profile": s.activeProfile.ID, "job": job.Name, "goal": truncateText(task.Goal, 200)},
	})
	// Also emit the generic queued event so the monitor queue panel can display it.
	s.emitBestEffort(ctx, events.Event{
		Type:    "task.generated",
		Message: "Task generated",
		Data:    map[string]string{"taskId": taskID, "profile": s.activeProfile.ID, "goal": truncateText(task.Goal, 200)},
	})
}

func (s *Session) inboxCount(ctx context.Context) (int, error) {
	resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSList, Path: s.cfg.InboxPath})
	if !resp.Ok {
		return 0, fmt.Errorf("list inbox: %s", resp.Error)
	}
	return len(resp.Entries), nil
}

func (s *Session) drainInbox(ctx context.Context) (bool, error) {
	resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSList, Path: s.cfg.InboxPath})
	if !resp.Ok {
		return false, fmt.Errorf("list inbox: %s", resp.Error)
	}
	paths := normalizeListEntries(s.cfg.InboxPath, resp.Entries)
	sort.Strings(paths)

	var errs error
	hadCandidates := false
	for _, p := range paths {
		if !strings.HasSuffix(strings.ToLower(p), ".json") {
			continue
		}
		if strings.Contains(p, "/poison/") || strings.Contains(p, "/archive/") {
			continue
		}
		hadCandidates = true

		raw, ok := s.readText(ctx, p)
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}

		// Control tasks: first-class and never passed to the LLM.
		if handled, err := s.tryHandleControl(ctx, p, raw); err != nil {
			errs = errors.Join(errs, err)
		} else if handled {
			continue
		}

		var task types.Task
		if err := json.Unmarshal([]byte(raw), &task); err != nil {
			continue
		}
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			taskID = strings.TrimSuffix(path.Base(p), path.Ext(p))
			task.TaskID = taskID
		}
		if strings.TrimSpace(task.Goal) == "" {
			continue
		}

		s.emitTaskQueuedOnce(ctx, taskID, task.Goal, "inbox")

		claim, err := s.cfg.Store.Claim(ctx, taskID, s.cfg.LeaseTTL)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("claim %s: %w", taskID, err))
			continue
		}
		if !claim.Claimed {
			continue
		}

		if claim.Attempts > s.cfg.MaxRetries {
			if err := s.quarantineTask(ctx, p, task); err != nil {
				errs = errors.Join(errs, err)
			}
			continue
		}

		if err := s.runTask(ctx, p, taskID, task); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return hadCandidates, errs
}

func (s *Session) runTask(ctx context.Context, taskPath, taskID string, task types.Task) error {
	now := time.Now()
	task.Status = types.TaskStatusActive
	task.StartedAt = &now
	_ = s.writeJSON(ctx, taskPath, task)

	if s.cfg.Events != nil {
		_ = s.cfg.Events.Emit(ctx, events.Event{
			Type:    "task.start",
			Message: "Task started",
			Data:    map[string]string{"taskId": taskID, "profile": s.activeProfile.ID, "goal": truncateText(task.Goal, 100)},
		})
	}

	leaseCtx, leaseStop := context.WithCancel(ctx)
	defer leaseStop()
	go func() {
		t := time.NewTicker(s.cfg.LeaseExtend)
		defer t.Stop()
		for {
			select {
			case <-leaseCtx.Done():
				return
			case <-t.C:
				_ = s.cfg.Store.Extend(leaseCtx, taskID, s.cfg.LeaseTTL)
			}
		}
	}()

	runAgent := s.cfg.Agent
	if s.cfg.Memory != nil || s.activeProfile != nil {
		memSnips := []agent.MemorySnippet{}
		if s.cfg.Memory != nil {
			if ms, err := s.cfg.Memory.Search(ctx, strings.TrimSpace(task.Goal), s.cfg.MemorySearchLimit); err == nil {
				memSnips = ms
			}
		}
		aug := buildSystemPrompt(runAgent.GetSystemPrompt(), *s.activeProfile, s.activePromptText, memSnips, s.lastTaskOutcome)
		if strings.TrimSpace(aug) != "" {
			cfg := runAgent.Config()
			cfg.SystemPrompt = aug
			if cloned, err := runAgent.CloneWithConfig(cfg); err == nil && cloned != nil {
				runAgent = cloned
			}
		}
	}

	runRes, err := runAgent.Run(ctx, strings.TrimSpace(task.Goal))
	doneAt := time.Now()
	tr := types.TaskResult{
		TaskID:      taskID,
		Status:      types.TaskStatusSucceeded,
		Summary:     strings.TrimSpace(runRes.Text),
		CompletedAt: &doneAt,
	}
	if err != nil {
		tr.Status = types.TaskStatusFailed
		tr.Error = err.Error()
	}

	// Preserve a tiny, immediate continuity signal for Task N+1 without relying on async memory indexing.
	// Stored value is strictly truncated and later re-sanitized at prompt construction.
	{
		outcome := strings.TrimSpace(tr.Summary)
		if outcome == "" {
			outcome = strings.TrimSpace(tr.Error)
		} else if tr.Status == types.TaskStatusFailed && strings.TrimSpace(tr.Error) != "" {
			outcome = strings.TrimSpace(outcome) + " | " + strings.TrimSpace(tr.Error)
		}
		s.lastTaskOutcome = truncateText(outcome, 150)
	}

	base := deliverablesBase(doneAt, taskID)
	copied := []string(nil)
	if len(runRes.Artifacts) != 0 {
		copied = s.materializeDeliverables(ctx, base, runRes.Artifacts)
	}
	if summaryPath := s.writeTaskSummary(ctx, base, taskID, task.Goal, tr, copied); summaryPath != "" {
		tr.Artifacts = append([]string{summaryPath}, copied...)
	} else {
		tr.Artifacts = copied
	}

	// Emit completion events before updating task state (ordering).
	if s.cfg.Events != nil {
		data := map[string]string{"taskId": taskID, "status": string(tr.Status), "profile": s.activeProfile.ID}
		if g := truncateText(task.Goal, 100); g != "" {
			data["goal"] = g
		}
		if sum := truncateText(tr.Summary, 900); sum != "" {
			data["summary"] = sum
		}
		if tr.Error != "" {
			data["error"] = tr.Error
		}
		if len(tr.Artifacts) != 0 {
			data["artifacts"] = fmt.Sprintf("%d", len(tr.Artifacts))
			// Always include the first artifact path (typically SUMMARY.md).
			data["artifact0"] = tr.Artifacts[0]
		}
		_ = s.cfg.Events.Emit(ctx, events.Event{Type: "task.done", Message: "Task finished", Data: data})
		if len(tr.Artifacts) != 0 {
			_ = s.cfg.Events.Emit(ctx, events.Event{Type: "task.delivered", Message: "Task deliverables recorded", Data: map[string]string{"taskId": taskID, "count": fmt.Sprintf("%d", len(tr.Artifacts)), "summaryPath": tr.Artifacts[0]}})
		}
	}

	if err := s.cfg.Store.Complete(ctx, taskID, tr); err != nil {
		return err
	}

	if err := s.writeResult(ctx, taskID, tr); err != nil {
		return err
	}
	task.Status = tr.Status
	task.CompletedAt = tr.CompletedAt
	task.Error = tr.Error
	_ = s.writeJSON(ctx, taskPath, task)

	if s.cfg.Notifier != nil {
		if err := s.cfg.Notifier.Notify(ctx, task, tr); err != nil {
			s.emitBestEffort(ctx, events.Event{Type: "task.notify.error", Message: "Task notification failed", Data: map[string]string{"taskId": taskID, "error": err.Error()}})
		}
	}
	return nil
}

func (s *Session) emitTaskQueuedOnce(ctx context.Context, taskID, goal, source string) {
	if s == nil || s.cfg.Events == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}
	if s.queuedEmitted == nil {
		s.queuedEmitted = make(map[string]struct{})
	}
	if _, ok := s.queuedEmitted[taskID]; ok {
		return
	}
	// Prevent unbounded growth for long-lived daemons.
	if len(s.queuedEmitted) > 5000 {
		s.queuedEmitted = make(map[string]struct{})
	}
	s.queuedEmitted[taskID] = struct{}{}
	payload := map[string]string{"taskId": taskID, "profile": s.activeProfile.ID, "goal": truncateText(goal, 200)}
	if strings.TrimSpace(source) != "" {
		payload["source"] = strings.TrimSpace(source)
	}
	_ = s.cfg.Events.Emit(ctx, events.Event{Type: "task.queued", Message: "Task queued", Data: payload})
}

func (s *Session) quarantineTask(ctx context.Context, taskPath string, task types.Task) error {
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		taskID = strings.TrimSuffix(path.Base(taskPath), path.Ext(taskPath))
	}
	poisonPath := path.Join(strings.TrimRight(s.cfg.InboxPath, "/"), "poison", time.Now().UTC().Format("20060102T150405Z")+"-"+taskID+".json")
	raw, ok := s.readText(ctx, taskPath)
	if ok && strings.TrimSpace(raw) != "" {
		_ = s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSWrite, Path: poisonPath, Text: raw})
	}
	lastErr := strings.TrimSpace(task.Error)
	if lastErr == "" {
		if rec, ok, err := s.cfg.Store.Get(ctx, taskID); err == nil && ok {
			if rec.Result != nil && strings.TrimSpace(rec.Result.Error) != "" {
				lastErr = strings.TrimSpace(rec.Result.Error)
			} else if strings.TrimSpace(rec.Error) != "" {
				lastErr = strings.TrimSpace(rec.Error)
			}
		}
	}
	_ = s.cfg.Store.Quarantine(ctx, taskID, "max retries exceeded")
	s.emitBestEffort(ctx, events.Event{
		Type:    "task.quarantined",
		Message: "Task quarantined",
		Data:    map[string]string{"taskId": taskID, "poisonPath": poisonPath, "error": fallback(lastErr, "max retries exceeded")},
	})

	// Mark original task file terminal-ish to discourage manual re-run without intervention.
	now := time.Now()
	task.Status = types.TaskStatusFailed
	task.Error = "quarantined: max retries exceeded"
	task.CompletedAt = &now
	_ = s.writeJSON(ctx, taskPath, task)
	return nil
}

func (s *Session) writeResult(ctx context.Context, taskID string, result types.TaskResult) error {
	outbox := strings.TrimRight(s.cfg.OutboxPath, "/")
	if outbox == "" {
		outbox = "/outbox"
	}
	filename := "result-" + taskID + ".json"
	resultPath := path.Join(outbox, filename)
	return s.writeJSON(ctx, resultPath, result)
}

func (s *Session) materializeDeliverables(ctx context.Context, base string, artifacts []string) []string {
	uniq := make([]string, 0, len(artifacts))
	seen := map[string]struct{}{}
	for _, p := range artifacts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		uniq = append(uniq, p)
	}
	if len(uniq) == 0 {
		return nil
	}

	out := make([]string, 0, len(uniq))

	for _, src := range uniq {
		srcBase := path.Base(src)
		if srcBase == "" || srcBase == "." || srcBase == "/" {
			srcBase = "artifact"
		}
		dst := path.Join(base, srcBase)
		// Best-effort copy.
		resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSRead, Path: src, MaxBytes: 4 * 1024 * 1024})
		if resp.Ok && strings.TrimSpace(resp.Text) != "" {
			_ = s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSWrite, Path: dst, Text: resp.Text})
			out = append(out, dst)
			continue
		}
		// Fall back to recording the original path.
		out = append(out, src)
	}
	return out
}

func deliverablesBase(when time.Time, taskID string) string {
	date := when.UTC().Format("2006-01-02")
	return path.Join("/workspace", "deliverables", date, taskID)
}

func (s *Session) writeTaskSummary(ctx context.Context, base, taskID, goal string, tr types.TaskResult, copiedArtifacts []string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	p := path.Join(base, "SUMMARY.md")
	var b strings.Builder
	b.WriteString("# Task Summary\n\n")
	b.WriteString("- TaskID: `")
	b.WriteString(taskID)
	b.WriteString("`\n")
	if strings.TrimSpace(goal) != "" {
		b.WriteString("- Goal: ")
		b.WriteString(strings.TrimSpace(goal))
		b.WriteString("\n")
	}
	b.WriteString("- Status: `")
	b.WriteString(string(tr.Status))
	b.WriteString("`\n")
	if tr.CompletedAt != nil && !tr.CompletedAt.IsZero() {
		b.WriteString("- CompletedAt: `")
		b.WriteString(tr.CompletedAt.UTC().Format(time.RFC3339Nano))
		b.WriteString("`\n")
	}
	if strings.TrimSpace(s.activeProfile.ID) != "" {
		b.WriteString("- Profile: `")
		b.WriteString(s.activeProfile.ID)
		b.WriteString("`\n")
	}
	if strings.TrimSpace(tr.Error) != "" {
		b.WriteString("\n## Error\n\n")
		b.WriteString("```\n")
		b.WriteString(strings.TrimSpace(tr.Error))
		b.WriteString("\n```\n")
	}

	b.WriteString("\n## Summary\n\n")
	if strings.TrimSpace(tr.Summary) == "" {
		b.WriteString("_No summary returned._\n")
	} else {
		b.WriteString(strings.TrimSpace(tr.Summary))
		b.WriteString("\n")
	}

	b.WriteString("\n## Deliverables\n\n")
	if len(copiedArtifacts) == 0 {
		b.WriteString("_No deliverables were recorded._\n")
	} else {
		for _, a := range copiedArtifacts {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			b.WriteString("- `")
			b.WriteString(a)
			b.WriteString("`\n")
		}
	}

	resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSWrite, Path: p, Text: b.String()})
	if !resp.Ok {
		return ""
	}
	return p
}

func (s *Session) readText(ctx context.Context, p string) (string, bool) {
	resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSRead, Path: p, MaxBytes: s.cfg.MaxReadBytes})
	if !resp.Ok {
		return "", false
	}
	return resp.Text, true
}

func (s *Session) writeJSON(ctx context.Context, p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	resp := s.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSWrite, Path: p, Text: string(b)})
	if !resp.Ok {
		return fmt.Errorf("write %s: %s", p, resp.Error)
	}
	return nil
}

func normalizeListEntries(inboxPath string, entries []string) []string {
	out := make([]string, 0, len(entries))
	for _, p := range entries {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, inboxPath) {
			out = append(out, p)
			continue
		}
		if strings.HasPrefix(p, "/") {
			out = append(out, p)
			continue
		}
		out = append(out, path.Join(inboxPath, p))
	}
	return out
}

func truncateText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	s = strings.TrimSpace(s)
	if s == "" || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func fallback(v string, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}
