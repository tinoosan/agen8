package domain

import "context"

// TaskRepository is the task-domain repository boundary used by app/service
// composition. It aliases the canonical task contracts so capabilities stay
// consistent across domain and infra wiring.
type TaskRepository interface {
	TaskReader
	TaskWriter
}

type TaskReader interface {
	GetTask(ctx context.Context, taskID TaskID) (Task, error)
	ListTasks(ctx context.Context, filter TaskFilter) ([]Task, error)
	CountTasks(ctx context.Context, filter TaskFilter) (int, error)
}

type TaskWriter interface {
	CreateTask(ctx context.Context, task Task) error
	UpdateTask(ctx context.Context, task Task) error
}
