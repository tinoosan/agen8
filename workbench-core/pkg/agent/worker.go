package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

// WorkerConfig configures a task-processing worker that polls /inbox and writes to /outbox.
type WorkerConfig struct {
	Agent        *Agent
	PollInterval time.Duration
	InboxPath    string
	OutboxPath   string
	MaxReadBytes int
	Logf         func(format string, args ...any)
}

// Worker polls /inbox for Task envelopes, runs the agent, and writes TaskResult to /outbox.
// It is opt-in and does not affect default agent behavior.
type Worker struct {
	cfg WorkerConfig
}

func NewWorker(cfg WorkerConfig) (*Worker, error) {
	if cfg.Agent == nil {
		return nil, fmt.Errorf("agent is required")
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if strings.TrimSpace(cfg.InboxPath) == "" {
		cfg.InboxPath = "/inbox"
	}
	if strings.TrimSpace(cfg.OutboxPath) == "" {
		cfg.OutboxPath = "/outbox"
	}
	if cfg.MaxReadBytes <= 0 {
		cfg.MaxReadBytes = 64 * 1024
	}
	return &Worker{cfg: cfg}, nil
}

// Run starts the polling loop until ctx is canceled.
func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := w.runOnce(ctx); err != nil && w.cfg.Logf != nil {
				w.cfg.Logf("worker: runOnce error: %v", err)
			}
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) error {
	paths, err := w.listInbox(ctx)
	if err != nil {
		return err
	}
	for _, p := range paths {
		task, ok := w.readTask(ctx, p)
		if !ok {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(task.Status))
		if status == "in_progress" || status == "succeeded" || status == "failed" || status == "canceled" {
			continue
		}
		now := time.Now()
		task.Status = "in_progress"
		task.StartedAt = &now
		if err := w.writeTask(ctx, p, task); err != nil {
			return err
		}
		return w.processTask(ctx, p, task)
	}
	return nil
}

func (w *Worker) listInbox(ctx context.Context) ([]string, error) {
	resp := w.cfg.Agent.Exec.Exec(ctx, types.HostOpRequest{
		Op:   types.HostOpFSList,
		Path: w.cfg.InboxPath,
	})
	if !resp.Ok {
		return nil, fmt.Errorf("list inbox: %s", resp.Error)
	}
	paths := make([]string, 0, len(resp.Entries))
	for _, p := range resp.Entries {
		if strings.HasSuffix(strings.ToLower(p), ".json") {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func (w *Worker) readTask(ctx context.Context, taskPath string) (types.Task, bool) {
	resp := w.cfg.Agent.Exec.Exec(ctx, types.HostOpRequest{
		Op:       types.HostOpFSRead,
		Path:     taskPath,
		MaxBytes: w.cfg.MaxReadBytes,
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

func (w *Worker) writeTask(ctx context.Context, taskPath string, task types.Task) error {
	b, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	resp := w.cfg.Agent.Exec.Exec(ctx, types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: taskPath,
		Text: string(b),
	})
	if !resp.Ok {
		return fmt.Errorf("update task: %s", resp.Error)
	}
	return nil
}

func (w *Worker) processTask(ctx context.Context, taskPath string, task types.Task) error {
	final, err := w.cfg.Agent.Run(ctx, strings.TrimSpace(task.Goal))
	now := time.Now()
	result := types.TaskResult{
		TaskID:      task.TaskID,
		RunID:       task.AssignedToRunID,
		Status:      "succeeded",
		Summary:     strings.TrimSpace(final),
		CompletedAt: &now,
	}
	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	if err := w.writeResult(ctx, task, result); err != nil {
		return err
	}

	task.Status = result.Status
	task.CompletedAt = result.CompletedAt
	task.Error = result.Error
	if err := w.writeTask(ctx, taskPath, task); err != nil {
		return err
	}
	return nil
}

func (w *Worker) writeResult(ctx context.Context, task types.Task, result types.TaskResult) error {
	outbox := strings.TrimRight(w.cfg.OutboxPath, "/")
	if outbox == "" {
		outbox = "/outbox"
	}
	filename := "result-" + strings.TrimSpace(task.TaskID) + ".json"
	resultPath := path.Join(outbox, filename)
	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	resp := w.cfg.Agent.Exec.Exec(ctx, types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: resultPath,
		Text: string(b),
	})
	if !resp.Ok {
		return fmt.Errorf("write result: %s", resp.Error)
	}
	return nil
}
