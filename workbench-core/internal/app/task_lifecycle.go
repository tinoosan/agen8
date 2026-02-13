package app

import (
	"context"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/agent/state"
	"github.com/tinoosan/workbench-core/pkg/types"
)

type runActiveTaskCanceler interface {
	CancelActiveTasksByRun(ctx context.Context, runID string, reason string) (int, error)
}

func cancelActiveTasksForRun(ctx context.Context, taskStore state.TaskStore, runID string, reason string) error {
	if taskStore == nil {
		return nil
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "run paused"
	}

	if canceler, ok := taskStore.(runActiveTaskCanceler); ok {
		_, err := canceler.CancelActiveTasksByRun(ctx, runID, reason)
		return err
	}

	// Fallback for non-SQLite stores: best-effort cancel currently-active tasks.
	tasks, err := taskStore.ListTasks(ctx, state.TaskFilter{
		RunID:  runID,
		Status: []types.TaskStatus{types.TaskStatusActive},
		Limit:  500,
	})
	if err != nil {
		return err
	}
	doneAt := time.Now().UTC()
	for _, task := range tasks {
		taskID := strings.TrimSpace(task.TaskID)
		if taskID == "" {
			continue
		}
		if err := taskStore.CompleteTask(ctx, taskID, types.TaskResult{
			TaskID:      taskID,
			Status:      types.TaskStatusCanceled,
			CompletedAt: &doneAt,
			Error:       reason,
		}); err != nil {
			return err
		}
	}
	return nil
}
