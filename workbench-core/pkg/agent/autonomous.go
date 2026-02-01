package agent

import (
	"context"
	"encoding/json"
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
	Memory            MemoryProvider
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
	return nil
}

func (cfg AutonomousRunnerConfig) WithDefaults() AutonomousRunnerConfig {
	cfg.Role = cfg.Role.Normalize()
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
	return cfg
}

type MemorySnippet struct {
	Title    string
	Filename string
	Content  string
	Score    float64
}

type MemoryProvider interface {
	Search(ctx context.Context, query string, limit int) ([]MemorySnippet, error)
	Save(ctx context.Context, title, content string) error
}

type Notifier interface {
	Notify(ctx context.Context, task types.Task, result types.TaskResult) error
}

// AutonomousRunner is an always-on control loop:
// - pulls tasks from /inbox
// - generates its own tasks based on Role triggers when idle
// - executes tasks via the underlying Agent
// - writes results to /outbox
type AutonomousRunner struct {
	cfg AutonomousRunnerConfig

	q *queue.TaskQueue

	mu          sync.RWMutex
	seenTaskIDs map[string]bool
	lastFired   map[string]time.Time
}

func NewAutonomousRunner(cfg AutonomousRunnerConfig) (*AutonomousRunner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.WithDefaults()
	return &AutonomousRunner{
		cfg:         cfg,
		q:           queue.New(),
		seenTaskIDs: map[string]bool{},
		lastFired:   map[string]time.Time{},
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
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-inboxTicker.C:
			_ = r.drainInbox(ctx)

		case <-roleTicker.C:
			r.maybeEnqueueProactive(ctx)
		default:
			// Execute next task if available.
			if item := r.q.Next(); item != nil {
				_ = r.executeQueuedTask(ctx, item)
				// Drain inbox immediately so tasks that arrived during execution are picked up without waiting for the ticker.
				_ = r.drainInbox(ctx)
				continue
			}
			// When idle, drain inbox once so a newly dropped task is picked up within one cycle.
			_ = r.drainInbox(ctx)
			// Yield a bit when idle.
			time.Sleep(50 * time.Millisecond)
		}
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

	for _, p := range paths {
		// Control plane: allow the monitor to update role/model asynchronously.
		if path.Base(p) == "control.json" {
			_ = r.processControlFile(ctx, p)
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
	return nil
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
	if !resp.Ok || strings.TrimSpace(resp.Text) == "" {
		return nil
	}
	var c control
	if err := json.Unmarshal([]byte(resp.Text), &c); err != nil {
		return nil
	}
	if c.Processed {
		return nil
	}

	changed := false
	if strings.TrimSpace(c.Role) != "" {
		role.ReloadDefaultManager()
		r.cfg.Role = role.Get(c.Role).Normalize()
		changed = true
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
		return nil
	}
	_ = r.cfg.Agent.ExecHostOp(ctx, types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: controlPath,
		Text: string(b),
	})
	if r.cfg.Emit != nil && changed {
		r.cfg.Emit(ctx, events.Event{
			Type:    "daemon.control",
			Message: "Control update applied",
			Data:    map[string]string{"role": r.cfg.Role.Name, "model": r.cfg.Agent.GetModel()},
		})
	}
	return nil
}

func (r *AutonomousRunner) maybeEnqueueProactive(ctx context.Context) {
	// Only generate new work when idle (no queued work).
	if !r.q.IsIdle() {
		return
	}

	now := time.Now()
	for _, t := range r.cfg.Role.Triggers {
		key := triggerKey(r.cfg.Role.Name, t)
		r.mu.RLock()
		lastFired := r.lastFired[key]
		r.mu.RUnlock()
		if !shouldFireTrigger(now, t, lastFired) {
			continue
		}
		r.mu.Lock()
		r.lastFired[key] = now
		r.mu.Unlock()
		if strings.TrimSpace(t.Goal) == "" {
			continue
		}
		r.enqueueSelfGoal(t.Goal, 5, "trigger")
		return
	}

	// If nothing fired, pick a standing goal (first) as background work.
	if len(r.cfg.Role.StandingGoals) > 0 {
		r.enqueueSelfGoal(r.cfg.Role.StandingGoals[0], 9, "standing")
	}
}

func (r *AutonomousRunner) enqueueSelfGoal(goal string, priority int, source string) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return
	}
	now := time.Now()
	taskID := "self-" + uuid.NewString()
	task := types.Task{
		TaskID:    taskID,
		Goal:      goal,
		Priority:  priority,
		Status:    types.TaskStatusPending,
		CreatedAt: &now,
		Metadata:  map[string]any{"source": source, "role": r.cfg.Role.Name},
	}
	r.q.Enqueue(&queue.Item{Task: task})
	if r.cfg.Emit != nil {
		r.cfg.Emit(context.Background(), events.Event{
			Type:    "task.generated",
			Message: "Generated task",
			Data: map[string]string{
				"taskId": taskID,
				"source": source,
				"role":   r.cfg.Role.Name,
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
	_ = r.writeTask(ctx, item.Path, task)

	if r.cfg.Emit != nil {
		r.cfg.Emit(ctx, events.Event{
			Type:    "task.start",
			Message: "Task started",
			Data: map[string]string{
				"taskId": taskID,
				"role":   r.cfg.Role.Name,
				"goal":   truncateText(goal, 100),
			},
		})
	}

	// Inject role + relevant memories by cloning the agent with an augmented system prompt.
	runAgent := a
	if r.cfg.Memory != nil || r.cfg.Role.Name != "" || r.cfg.Role.Description != "" {
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
	_ = r.writeResult(ctx, task, result)

	task.Status = result.Status
	task.CompletedAt = result.CompletedAt
	task.Error = result.Error
	_ = r.writeTask(ctx, item.Path, task)

	if r.cfg.Emit != nil {
		data := map[string]string{
			"taskId": taskID,
			"status": string(result.Status),
			"role":   r.cfg.Role.Name,
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
	return nil
}

func buildAugmentedSystemPrompt(base string, r role.Role, memories []MemorySnippet) string {
	base = strings.TrimSpace(base)
	var b strings.Builder
	if base != "" {
		b.WriteString(base)
	}

	rr := r.Normalize()
	if rr.Name != "" || rr.Description != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("<role>\n")
		if rr.Name != "" {
			b.WriteString("Name: ")
			b.WriteString(rr.Name)
			b.WriteString("\n")
		}
		if rr.Description != "" {
			b.WriteString("Description: ")
			b.WriteString(rr.Description)
			b.WriteString("\n")
		}
		if rr.Content != "" {
			b.WriteString("Instructions:\n")
			b.WriteString(rr.Content)
			if !strings.HasSuffix(rr.Content, "\n") {
				b.WriteString("\n")
			}
		}
		if len(rr.StandingGoals) > 0 {
			b.WriteString("StandingGoals:\n")
			for _, g := range rr.StandingGoals {
				if strings.TrimSpace(g) == "" {
					continue
				}
				b.WriteString("- ")
				b.WriteString(strings.TrimSpace(g))
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

func triggerKey(roleName string, t role.Trigger) string {
	return strings.ToLower(strings.TrimSpace(roleName)) + "|" + strings.ToLower(strings.TrimSpace(t.Type)) + "|" + strings.TrimSpace(t.TimeOfDay) + "|" + strings.TrimSpace(t.Goal)
}

func shouldFireTrigger(now time.Time, t role.Trigger, last time.Time) bool {
	switch strings.ToLower(strings.TrimSpace(t.Type)) {
	case "interval":
		if t.Interval <= 0 {
			return false
		}
		if last.IsZero() {
			return true
		}
		return now.Sub(last) >= t.Interval

	case "time_of_day":
		hhmm := strings.TrimSpace(t.TimeOfDay)
		if len(hhmm) != 5 || hhmm[2] != ':' {
			return false
		}
		target := time.Date(now.Year(), now.Month(), now.Day(), atoi2(hhmm[0], hhmm[1]), atoi2(hhmm[3], hhmm[4]), 0, 0, now.Location())
		// Fire if we're within the same minute window and haven't fired today.
		if now.Before(target) || now.Sub(target) > time.Minute {
			return false
		}
		return last.IsZero() || last.YearDay() != now.YearDay() || last.Year() != now.Year()
	default:
		return false
	}
}

func atoi2(a, b byte) int {
	if a < '0' || a > '9' || b < '0' || b > '9' {
		return 0
	}
	return int(a-'0')*10 + int(b-'0')
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
