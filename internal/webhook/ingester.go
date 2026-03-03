package webhook

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8/pkg/agent/state"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/types"
)

// TaskIngester accepts a fully constructed task, persists it via the task
// store, optionally archives it, and emits events. It decouples HTTP handling
// from persistence and storage concerns.
type TaskIngester interface {
	// IngestTask creates the task in the store and optionally archives it.
	// Returns the task ID and any error from the store.
	IngestTask(ctx context.Context, task types.Task) (taskID string, err error)
}

// WebhookTaskIngester implements TaskIngester by delegating to a TaskStore
// and optionally archiving via TaskArchiveWriter.
type WebhookTaskIngester struct {
	taskStore state.TaskStore
	archive   TaskArchiveWriter
	emit      func(context.Context, events.Event)
}

// NewWebhookTaskIngester returns a TaskIngester that creates tasks via the
// store and archives them when a TaskArchiveWriter is provided.
func NewWebhookTaskIngester(taskStore state.TaskStore, archive TaskArchiveWriter, emit func(context.Context, events.Event)) *WebhookTaskIngester {
	return &WebhookTaskIngester{
		taskStore: taskStore,
		archive:   archive,
		emit:      emit,
	}
}

// IngestTask creates the task in the store, archives it (best-effort), and
// emits webhook.task.queued when successful.
func (w *WebhookTaskIngester) IngestTask(ctx context.Context, task types.Task) (taskID string, err error) {
	if w.taskStore == nil {
		return "", fmt.Errorf("task store not configured")
	}
	if err := w.taskStore.CreateTask(ctx, task); err != nil {
		return "", err
	}
	if w.archive != nil {
		w.archive.ArchiveTask(ctx, task)
	}
	if w.emit != nil {
		w.emit(ctx, events.Event{
			Type:    "webhook.task.queued",
			Message: "Webhook task queued",
			Data:    map[string]string{"taskId": task.TaskID},
		})
	}
	return task.TaskID, nil
}
