package app

import (
	"context"
	"strings"

	pkgtask "github.com/tinoosan/workbench-core/pkg/services/task"
)

func cancelActiveTasksForRun(ctx context.Context, canceler pkgtask.ActiveTaskCanceler, runID string, reason string) error {
	if canceler == nil {
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
	_, err := canceler.CancelActiveTasksByRun(ctx, runID, reason)
	return err
}
