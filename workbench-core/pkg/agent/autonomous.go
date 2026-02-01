package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/queue"
	"github.com/tinoosan/workbench-core/pkg/role"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type AutonomousRunnerConfig struct {
	Agent Agent

	Role role.Role

	// Memory, if set, is used to retrieve relevant memories for the current task
	// and to persist new memories after task completion.
	Memory            MemoryRecallProvider
	MemorySearchLimit int
	Notifier          Notifier

	InboxPath         string
	OutboxPath        string
	PollInterval      time.Duration
	ProactiveInterval time.Duration
	InitialGoal       string
	MaxReadBytes      int
	Logf              func(format string, args ...any)
	Emit              func(ctx context.Context, ev events.Event)
}

func (cfg AutonomousRunnerConfig) Validate() error {
	if cfg.Agent == nil {
		return fmt.Errorf("agent is required")
	}
	if _, err := cfg.Role.Normalize(); err != nil {
		return err
	}
	return nil
}

func (cfg AutonomousRunnerConfig) WithDefaults() (AutonomousRunnerConfig, error) {
	normalizedRole, err := cfg.Role.Normalize()
	if err != nil {
		return cfg, err
	}
	cfg.Role = normalizedRole
	if strings.TrimSpace(cfg.InboxPath) == "" {
		cfg.InboxPath = "/inbox"
	}
	if strings.TrimSpace(cfg.OutboxPath) == "" {
		cfg.OutboxPath = "/outbox"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 5 * time.Second
	}
	if cfg.ProactiveInterval <= 0 {
		cfg.ProactiveInterval = 30 * time.Second
	}
	if cfg.MemorySearchLimit <= 0 {
		cfg.MemorySearchLimit = 3
	}
	if cfg.MaxReadBytes <= 0 {
		cfg.MaxReadBytes = 96 * 1024
	}
	return cfg, nil
}

type MemorySnippet struct {
	Title    string
	Filename string
	Content  string
	Score    float64
}

// MemoryRecallProvider provides best-effort semantic recall and persistence for the agent.
// This is distinct from the daily memory file store (`store.DailyMemoryStore`).
type MemoryRecallProvider interface {
	Search(ctx context.Context, query string, limit int) ([]MemorySnippet, error)
	Save(ctx context.Context, title, content string) error
}

// MemoryProvider is kept for backwards compatibility.
//
// Deprecated: use MemoryRecallProvider.
type MemoryProvider = MemoryRecallProvider

type Notifier interface {
	Notify(ctx context.Context, task types.Task, result types.TaskResult) error
}

// AutonomousRunner is an always-on control loop:
// - pulls tasks from /inbox (manual/external)
// - generates bounded autonomous tasks only when role obligations need attention
// - executes tasks via the underlying Agent
// - writes results to /outbox
type AutonomousRunner struct {
	cfg AutonomousRunnerConfig

	q *queue.TaskQueue

	mu sync.RWMutex

	seenTaskIDs          map[string]bool
	lastSatisfied        map[string]time.Time
	pendingObligationRun map[string]int
}

func NewAutonomousRunner(cfg AutonomousRunnerConfig) (*AutonomousRunner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	var err error
	cfg, err = cfg.WithDefaults()
	if err != nil {
		return nil, err
	}
	return &AutonomousRunner{
		cfg:                  cfg,
		q:                    queue.New(),
		seenTaskIDs:          map[string]bool{},
		lastSatisfied:        map[string]time.Time{},
		pendingObligationRun: map[string]int{},
	}, nil
}

func (r *AutonomousRunner) Run(ctx context.Context) error {
	if strings.TrimSpace(r.cfg.InitialGoal) != "" {
		r.enqueueSelfGoal(r.cfg.InitialGoal, 0, "startup")
	}

	inboxTicker := time.NewTicker(r.cfg.PollInterval)
	defer inboxTicker.Stop()
	roleTicker := time.NewTicker(r.cfg.ProactiveInterval)
	defer roleTicker.Stop()

	for {
		// Execute next task if available.
		if item := r.q.Next(); item != nil {
			if err := r.executeQueuedTask(ctx, item); err != nil {
				r.reportError(ctx, "task.execute", err)
			}
			// Drain inbox immediately so tasks that arrived during execution are picked up without waiting for the ticker.
			if err := r.drainInbox(ctx); err != nil {
				r.reportError(ctx, "inbox.drain", err)
			}
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-inboxTicker.C:
			if err := r.drainInbox(ctx); err != nil {
				r.reportError(ctx, "inbox.drain", err)
			}

		case <-roleTicker.C:
			r.maybeEnqueueObligationTasks(ctx)
		}
	}
}

func (r *AutonomousRunner) reportError(ctx context.Context, kind string, err error) {
	if err == nil {
		return
	}
	if r.cfg.Logf != nil {
		r.cfg.Logf("autonomous runner error (%s): %v", strings.TrimSpace(kind), err)
	}
	if r.cfg.Emit != nil {
		data := map[string]string{"error": err.Error()}
		if strings.TrimSpace(kind) != "" {
			data["kind"] = strings.TrimSpace(kind)
		}
		r.cfg.Emit(ctx, events.Event{
			Type:    "daemon.error",
			Message: "Autonomous runner error",
			Data:    data,
		})
	}
}

func (r *AutonomousRunner) drainInbox(ctx context.Context) error {
	a := r.cfg.Agent
	resp := a.ExecHostOp(ctx, types.HostOpRequest{Op: types.HostOpFSList, Path: r.cfg.InboxPath})
	if !resp.Ok {
		return fmt.Errorf("list inbox: %s", resp.Error)
	}
	paths := make([]string, 0, len(resp.Entries))
	for _, p := range resp.Entries {
		if strings.HasSuffix(strings.ToLower(p), ".json") {
			paths = append(paths, path.Join(r.cfg.InboxPath, p))
		}
	}
	sort.Strings(paths)

	var errs error
	for _, p := range paths {
		// Control plane: allow the monitor to update role/model asynchronously.
		if path.Base(p) == "control.json" {
			if err := r.processControlFile(ctx, p); err != nil {
				errs = errors.Join(errs, err)
				r.reportError(ctx, "control.process", err)
			}
			continue
		}

		task, ok := r.readTask(ctx, p)
		if !ok {
			continue
		}
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			taskID = strings.TrimSuffix(path.Base(p), path.Ext(p))
			task.TaskID = taskID
		}

		task.NormalizeStatus()
		status := string(task.Status)
		switch status {
		case string(types.TaskStatusActive), string(types.TaskStatusSucceeded), string(types.TaskStatusFailed), string(types.TaskStatusCanceled):
			continue
		default:
			// allow enqueue
		}

		r.mu.Lock()
		if r.seenTaskIDs[taskID] {
			r.mu.Unlock()
			continue
		}
		r.seenTaskIDs[taskID] = true
		r.mu.Unlock()

		if task.CreatedAt == nil {
			now := time.Now()
			task.CreatedAt = &now
		}
		r.q.Enqueue(&queue.Item{Task: task, Path: p})
		if r.cfg.Emit != nil {
			r.cfg.Emit(ctx, events.Event{
				Type:    "task.queued",
				Message: "Task queued",
				Data: map[string]string{
					"taskId": taskID,
					"source": "inbox",
					"goal":   truncateText(task.Goal, 100),
				},
			})
		}
	}
	return errs
}

func (r *AutonomousRunner) processControlFile(ctx context.Context, controlPath string) error {
	type control struct {
		Model       string `json:"model,omitempty"`
		Role        string `json:"role,omitempty"`
		Processed   bool   `json:"processed,omitempty"`
		ProcessedAt string `json:"processedAt,omitempty"`
		Error       string `json:"error,omitempty"`
	}
	resp := r.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{
		Op:       types.HostOpFSRead,
		Path:     controlPath,
		MaxBytes: r.cfg.MaxReadBytes,
	})
	if !resp.Ok {
		return fmt.Errorf("read control file: %s", resp.Error)
	}
	if strings.TrimSpace(resp.Text) == "" {
		return nil
	}
	var c control
	if err := json.Unmarshal([]byte(resp.Text), &c); err != nil {
		return fmt.Errorf("parse control file: %w", err)
	}
	if c.Processed {
		return nil
	}

	changed := false
	if strings.TrimSpace(c.Role) != "" {
		role.ReloadDefaultManager()
		if newRole, ok := role.GetDefault(c.Role); ok {
			normalized, err := newRole.Normalize()
			if err != nil {
				c.Error = fmt.Sprintf("role invalid: %v", err)
			} else {
				r.cfg.Role = normalized
				changed = true
			}
		} else {
			c.Error = "role not found"
		}
	}
	if strings.TrimSpace(c.Model) != "" {
		r.cfg.Agent.SetModel(strings.TrimSpace(c.Model))
		changed = true
	}

	c.Processed = true
	c.ProcessedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if !changed {
		c.Error = "no changes applied"
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal control response: %w", err)
	}
	wresp := r.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: controlPath,
		Text: string(b),
	})
	if !wresp.Ok {
		return fmt.Errorf("write control response: %s", wresp.Error)
	}
	if r.cfg.Emit != nil && changed {
		r.cfg.Emit(ctx, events.Event{
			Type:    "daemon.control",
			Message: "Control update applied",
			Data:    map[string]string{"role": r.cfg.Role.ID, "model": r.cfg.Agent.GetModel()},
		})
	}
	return nil
}

func (r *AutonomousRunner) maybeEnqueueObligationTasks(ctx context.Context) {
	now := time.Now()
	roleCfg := r.cfg.Role
	maxPerCycle := roleCfg.TaskPolicy.MaxTasksPerCycle
	if maxPerCycle <= 0 {
		maxPerCycle = 1
	}

	// Build status for each obligation.
	unsatisfied := []role.Obligation{}
	expiring := []role.Obligation{}

	r.mu.RLock()
	for _, ob := range roleCfg.Obligations {
		last := r.lastSatisfied[ob.ID]
		if last.IsZero() {
			unsatisfied = append(unsatisfied, ob)
			continue
		}
		if ob.ValidityDuration <= 0 {
			// symbolic validity: treat as satisfied until explicit refresh needed
			continue
		}
		elapsed := now.Sub(last)
		if elapsed >= ob.ValidityDuration {
			unsatisfied = append(unsatisfied, ob)
			continue
		}
		if elapsed >= ob.ValidityDuration-r.cfg.ProactiveInterval {
			expiring = append(expiring, ob)
		}
	}
	r.mu.RUnlock()

	createRules := roleCfg.TaskPolicy.CreateTasksOnlyIf
	if len(createRules) == 0 {
		return
	}

	created := 0
	enqueue := func(ob role.Obligation, reason string) {
		if created >= maxPerCycle {
			return
		}
		if r.hasPendingObligationTask(ob.ID) {
			return
		}
		goal := fmt.Sprintf("Refresh evidence for obligation %s (%s)", ob.ID, ob.Evidence)
		r.enqueueSelfGoal(goal, 5, reason, ob.ID)
		created++
	}

	for _, rule := range createRules {
		switch strings.ToLower(strings.TrimSpace(rule)) {
		case "obligation_unsatisfied":
			for _, ob := range unsatisfied {
				enqueue(ob, "obligation")
				if created >= maxPerCycle {
					return
				}
			}
		case "obligation_expiring":
			for _, ob := range expiring {
				enqueue(ob, "obligation")
				if created >= maxPerCycle {
					return
				}
			}
		}
		if created >= maxPerCycle {
			return
		}
	}
}

func (r *AutonomousRunner) enqueueSelfGoal(goal string, priority int, source string, obligationID ...string) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return
	}
	now := time.Now()
	taskID := "self-" + uuid.NewString()
	metadata := map[string]any{"source": source, "role": r.cfg.Role.ID}
	if len(obligationID) > 0 && strings.TrimSpace(obligationID[0]) != "" {
		metadata["obligation"] = strings.TrimSpace(obligationID[0])
	}
	task := types.Task{
		TaskID:    taskID,
		Goal:      goal,
		Priority:  priority,
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  metadata,
	}
	r.q.Enqueue(&queue.Item{Task: task})
	if obID, ok := metadata["obligation"].(string); ok && obID != "" {
		r.mu.Lock()
		r.pendingObligationRun[obID]++
		r.mu.Unlock()
	}
	if r.cfg.Emit != nil {
		r.cfg.Emit(context.Background(), events.Event{
			Type:    "task.generated",
			Message: "Generated task",
			Data: map[string]string{
				"taskId": taskID,
				"source": source,
				"role":   r.cfg.Role.ID,
				"goal":   truncateText(goal, 100),
			},
		})
	}
}

func (r *AutonomousRunner) executeQueuedTask(ctx context.Context, item *queue.Item) error {
	if item == nil {
		return nil
	}
	a := r.cfg.Agent
	task := item.Task
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		taskID = "task-" + uuid.NewString()
		task.TaskID = taskID
	}
	goal := strings.TrimSpace(task.Goal)
	if goal == "" {
		return nil
	}

	now := time.Now()
	task.Status = types.TaskStatusActive
	task.StartedAt = &now
	var persistErr error
	if err := r.writeTask(ctx, item.Path, task); err != nil {
		persistErr = errors.Join(persistErr, err)
	}

	if r.cfg.Emit != nil {
		r.cfg.Emit(ctx, events.Event{
			Type:    "task.start",
			Message: "Task started",
			Data: map[string]string{
				"taskId": taskID,
				"role":   r.cfg.Role.ID,
				"goal":   truncateText(goal, 100),
			},
		})
	}

	// Inject role + relevant memories by cloning the agent with an augmented system prompt.
	runAgent := a
	if r.cfg.Memory != nil || r.cfg.Role.ID != "" || r.cfg.Role.Description != "" {
		memSnips := []MemorySnippet{}
		if r.cfg.Memory != nil {
			if ms, err := r.cfg.Memory.Search(ctx, goal, r.cfg.MemorySearchLimit); err == nil {
				memSnips = ms
			}
		}
		aug := buildAugmentedSystemPrompt(a.GetSystemPrompt(), r.cfg.Role, memSnips)
		if strings.TrimSpace(aug) != "" {
			cfg := a.Config()
			cfg.SystemPrompt = aug
			if cloned, err := a.CloneWithConfig(cfg); err == nil && cloned != nil {
				runAgent = cloned
			}
		}
	}

	final, err := runAgent.Run(ctx, goal)
	doneAt := time.Now()

	result := types.TaskResult{
		TaskID:      taskID,
		Status:      types.TaskStatusSucceeded,
		Summary:     strings.TrimSpace(final),
		CompletedAt: &doneAt,
	}
	if err != nil {
		result.Status = types.TaskStatusFailed
		result.Error = err.Error()
	}
	if err := r.writeResult(ctx, task, result); err != nil {
		persistErr = errors.Join(persistErr, err)
	}

	task.Status = result.Status
	task.CompletedAt = result.CompletedAt
	task.Error = result.Error
	if err := r.writeTask(ctx, item.Path, task); err != nil {
		persistErr = errors.Join(persistErr, err)
	}

	r.onTaskFinished(task, result)

	if r.cfg.Emit != nil {
		data := map[string]string{
			"taskId": taskID,
			"status": string(result.Status),
			"role":   r.cfg.Role.ID,
		}
		if goal := truncateText(task.Goal, 100); goal != "" {
			data["goal"] = goal
		}
		if summary := truncateText(result.Summary, 200); summary != "" {
			data["summary"] = summary
		}
		if result.Error != "" {
			data["error"] = result.Error
		}
		r.cfg.Emit(ctx, events.Event{
			Type:    "task.done",
			Message: "Task finished",
			Data:    data,
		})
	}
	if r.cfg.Notifier != nil {
		if err := r.cfg.Notifier.Notify(ctx, task, result); err != nil && r.cfg.Emit != nil {
			r.cfg.Emit(ctx, events.Event{
				Type:    "task.notify.error",
				Message: "Task notification failed",
				Data:    map[string]string{"taskId": taskID, "error": err.Error()},
			})
		}
	}
	return persistErr
}

func buildAugmentedSystemPrompt(base string, r role.Role, memories []MemorySnippet) string {
	base = strings.TrimSpace(base)
	var b strings.Builder
	if base != "" {
		b.WriteString(base)
	}

	rr := r
	if normalized, err := r.Normalize(); err == nil {
		rr = normalized
	}
	if rr.ID != "" || rr.Description != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("<role>\n")
		if rr.ID != "" {
			b.WriteString("ID: ")
			b.WriteString(rr.ID)
			b.WriteString("\n")
		}
		if rr.Description != "" {
			b.WriteString("Description: ")
			b.WriteString(rr.Description)
			b.WriteString("\n")
		}
		if len(rr.SkillBias) > 0 {
			b.WriteString("SkillBias:\n")
			for _, s := range rr.SkillBias {
				if strings.TrimSpace(s) == "" {
					continue
				}
				b.WriteString("- ")
				b.WriteString(strings.TrimSpace(s))
				b.WriteString("\n")
			}
		}
		if len(rr.Obligations) > 0 {
			b.WriteString("Obligations:\n")
			for _, ob := range rr.Obligations {
				if strings.TrimSpace(ob.ID) == "" {
					continue
				}
				b.WriteString("- id: ")
				b.WriteString(strings.TrimSpace(ob.ID))
				if strings.TrimSpace(ob.ValidityRaw) != "" {
					b.WriteString("  validity: ")
					b.WriteString(strings.TrimSpace(ob.ValidityRaw))
				}
				if strings.TrimSpace(ob.Evidence) != "" {
					b.WriteString("  evidence: ")
					b.WriteString(strings.TrimSpace(ob.Evidence))
				}
				b.WriteString("\n")
			}
		}
		if strings.TrimSpace(rr.Guidance) != "" {
			b.WriteString("Guidance:\n")
			b.WriteString(strings.TrimSpace(rr.Guidance))
			if !strings.HasSuffix(rr.Guidance, "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("</role>")
	}

	if len(memories) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("<memories>\n")
		for i, m := range memories {
			if i >= 6 {
				break
			}
			title := strings.TrimSpace(m.Title)
			content := strings.TrimSpace(m.Content)
			if title == "" && content == "" {
				continue
			}
			if title != "" {
				b.WriteString("Title: ")
				b.WriteString(title)
				b.WriteString("\n")
			}
			if content != "" {
				// Keep excerpts bounded.
				if len(content) > 1200 {
					content = content[:1200] + "…"
				}
				b.WriteString(content)
				b.WriteString("\n")
			}
			b.WriteString("---\n")
		}
		b.WriteString("</memories>")
	}

	return strings.TrimSpace(b.String())
}

func (r *AutonomousRunner) readTask(ctx context.Context, taskPath string) (types.Task, bool) {
	if strings.TrimSpace(taskPath) == "" {
		return types.Task{}, false
	}
	resp := r.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{
		Op:       types.HostOpFSRead,
		Path:     taskPath,
		MaxBytes: r.cfg.MaxReadBytes,
	})
	if !resp.Ok || strings.TrimSpace(resp.Text) == "" {
		return types.Task{}, false
	}
	var task types.Task
	if err := json.Unmarshal([]byte(resp.Text), &task); err != nil {
		return types.Task{}, false
	}
	return task, true
}

func (r *AutonomousRunner) writeTask(ctx context.Context, taskPath string, task types.Task) error {
	if strings.TrimSpace(taskPath) == "" {
		// Self-generated tasks are not persisted to inbox by default.
		return nil
	}
	b, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	resp := r.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: taskPath,
		Text: string(b),
	})
	if !resp.Ok {
		return fmt.Errorf("update task: %s", resp.Error)
	}
	return nil
}

func (r *AutonomousRunner) writeResult(ctx context.Context, task types.Task, result types.TaskResult) error {
	outbox := strings.TrimRight(r.cfg.OutboxPath, "/")
	if outbox == "" {
		outbox = "/outbox"
	}
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		taskID = "task"
	}
	filename := "result-" + taskID + ".json"
	resultPath := path.Join(outbox, filename)
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	resp := r.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: resultPath,
		Text: string(b),
	})
	if !resp.Ok {
		return fmt.Errorf("write result: %s", resp.Error)
	}
	return nil
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

func (r *AutonomousRunner) hasPendingObligationTask(obligationID string) bool {
	if strings.TrimSpace(obligationID) == "" {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pendingObligationRun[obligationID] > 0
}

func (r *AutonomousRunner) onTaskFinished(task types.Task, result types.TaskResult) {
	obligationID := ""
	if task.Metadata != nil {
		if v, ok := task.Metadata["obligation"]; ok {
			if s, ok := v.(string); ok {
				obligationID = strings.TrimSpace(s)
			}
		}
	}

	if obligationID != "" {
		r.mu.Lock()
		if result.Status == types.TaskStatusSucceeded {
			r.lastSatisfied[obligationID] = time.Now()
		}
		if r.pendingObligationRun[obligationID] > 0 {
			r.pendingObligationRun[obligationID]--
		}
		r.mu.Unlock()
	}
}
