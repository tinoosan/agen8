package hosttools

import (
	"context"
	"strings"
)

type contextKey string

const parentTaskIDContextKey contextKey = "task_create_parent_task_id"

// WithParentTaskID annotates tool-call context with the currently executing parent task ID.
func WithParentTaskID(ctx context.Context, taskID string) context.Context {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ctx
	}
	return context.WithValue(ctx, parentTaskIDContextKey, taskID)
}

// ParentTaskIDFromContext returns the parent task ID if one was attached to the context.
func ParentTaskIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	raw := ctx.Value(parentTaskIDContextKey)
	val, _ := raw.(string)
	return strings.TrimSpace(val)
}
