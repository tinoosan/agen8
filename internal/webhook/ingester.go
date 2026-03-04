package webhook

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8/pkg/events"
	pkgtask "github.com/tinoosan/agen8/pkg/services/task"
	"github.com/tinoosan/agen8/pkg/types"
)

// TaskIngester accepts a fully constructed task, persists it via the task
// service, optionally archives it, and emits events. It decouples HTTP handling
// from persistence and storage concerns.
type TaskIngester interface {
	// IngestTask creates the task in the service and optionally archives it.
	// Returns the task ID and any error from the service.
	IngestTask(ctx context.Context, task types.Task) (taskID string, err error)
}

// WebhookTaskIngester implements TaskIngester by delegating to a TaskService
// and optionally archiving via TaskArchiveWriter.
type WebhookTaskIngester struct {
	taskService pkgtask.TaskServiceForRPC
	archive     TaskArchiveWriter
	emit        func(context.Context, events.Event)
}

// NewWebhookTaskIngester returns a TaskIngester that creates tasks via the
// service and archives them when a TaskArchiveWriter is provided.
func NewWebhookTaskIngester(taskService pkgtask.TaskServiceForRPC, archive TaskArchiveWriter, emit func(context.Context, events.Event)) *WebhookTaskIngester {
	return &WebhookTaskIngester{
		taskService: taskService,
		archive:     archive,
		emit:        emit,
	}
}

// IngestTask creates the task in the store, archives it (best-effort), and
// emits webhook.task.queued when successful.
func (w *WebhookTaskIngester) IngestTask(ctx context.Context, task types.Task) (taskID string, err error) {
	if w.taskService == nil {
		return "", fmt.Errorf("task service not configured")
	}
	if err := w.taskService.CreateTask(ctx, task); err != nil {
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
