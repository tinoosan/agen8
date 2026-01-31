package queue

import (
	"sort"
	"sync"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

// Item is one queued task plus optional provenance (e.g. inbox file path).
type Item struct {
	Task types.Task

	// Path is the VFS path of the source task file (e.g. "/inbox/task-123.json").
	// Empty for self-generated tasks.
	Path string
}

// TaskQueue is a small in-memory priority queue for tasks.
// Lower Priority values run first; within equal priority, older tasks run first.
type TaskQueue struct {
	mu    sync.Mutex
	items []*Item
}

func New() *TaskQueue {
	return &TaskQueue{}
}

func (q *TaskQueue) Enqueue(items ...*Item) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range items {
		if it == nil {
			continue
		}
		q.items = append(q.items, it)
	}
	sort.SliceStable(q.items, func(i, j int) bool {
		if q.items[i].Task.Priority != q.items[j].Task.Priority {
			return q.items[i].Task.Priority < q.items[j].Task.Priority
		}
		return taskTime(q.items[i].Task).Before(taskTime(q.items[j].Task))
	})
}

func (q *TaskQueue) Next() *Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	for idx, it := range q.items {
		status := it.Task.Status
		if status == "" || status == types.TaskStatusPending {
			// Remove from queue and return.
			q.items = append(q.items[:idx], q.items[idx+1:]...)
			return it
		}
	}
	return nil
}

func (q *TaskQueue) IsIdle() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	filtered := q.items[:0]
	idle := true
	for _, it := range q.items {
		status := it.Task.Status
		if status == "" || status == types.TaskStatusPending || status == types.TaskStatusActive {
			idle = false
			filtered = append(filtered, it)
		}
	}
	q.items = filtered
	return idle
}

func (q *TaskQueue) Snapshot() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Item, 0, len(q.items))
	for _, it := range q.items {
		if it == nil {
			continue
		}
		out = append(out, *it)
	}
	return out
}

func taskTime(task types.Task) time.Time {
	if task.CreatedAt != nil {
		return task.CreatedAt.UTC()
	}
	return time.Time{}
}
