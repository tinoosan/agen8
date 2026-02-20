package hosttools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type contextKey string

const parentTaskIDContextKey contextKey = "task_create_parent_task_id"
const batchWaveStateContextKey contextKey = "task_create_batch_wave_state"

type batchWaveState struct {
	mu  sync.Mutex
	ids map[string]string
}

// WithParentTaskID annotates tool-call context with the currently executing parent task ID.
func WithParentTaskID(ctx context.Context, taskID string) context.Context {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ctx
	}
	return context.WithValue(ctx, parentTaskIDContextKey, taskID)
}

// WithBatchWaveState ensures task_create calls in this context share wave IDs per parent task.
func WithBatchWaveState(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if existing, _ := ctx.Value(batchWaveStateContextKey).(*batchWaveState); existing != nil {
		return ctx
	}
	return context.WithValue(ctx, batchWaveStateContextKey, &batchWaveState{ids: map[string]string{}})
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

// EnsureBatchWaveIDFromContext returns a stable wave ID for the parent task within this context.
func EnsureBatchWaveIDFromContext(ctx context.Context, parentTaskID, originID string) string {
	parentTaskID = strings.TrimSpace(parentTaskID)
	if parentTaskID == "" {
		return ""
	}
	originID = strings.TrimSpace(originID)
	state, _ := ctx.Value(batchWaveStateContextKey).(*batchWaveState)
	if state == nil {
		return fmt.Sprintf("wave-%s-%s-%d", parentTaskID, fallbackID(originID), time.Now().UTC().UnixNano())
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if id := strings.TrimSpace(state.ids[parentTaskID]); id != "" {
		return id
	}
	id := fmt.Sprintf("wave-%s-%s-%d", parentTaskID, fallbackID(originID), time.Now().UTC().UnixNano())
	state.ids[parentTaskID] = id
	return id
}

func fallbackID(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}
