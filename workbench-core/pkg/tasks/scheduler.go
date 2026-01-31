package tasks

import (
	"sort"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

// ReadyTasks returns tasks that are ready to run, ordered by priority then creation time.
func ReadyTasks(all map[string]types.Task) []types.Task {
	out := make([]types.Task, 0, len(all))
	for _, task := range all {
		out = append(out, task)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		return taskTime(out[i]).Before(taskTime(out[j]))
	})
	return out
}

func taskTime(task types.Task) time.Time {
	if task.CreatedAt != nil {
		return task.CreatedAt.UTC()
	}
	return time.Time{}
}
