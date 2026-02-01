package tasks

import (
	"sort"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

// ReadyTasks returns tasks ordered by Priority (ascending) then CreatedAt (oldest first).
func ReadyTasks(tasks map[string]types.Task) []types.Task {
	out := make([]types.Task, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		ti := taskTime(out[i])
		tj := taskTime(out[j])
		if !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return out[i].TaskID < out[j].TaskID
	})
	return out
}

func taskTime(task types.Task) time.Time {
	if task.CreatedAt != nil {
		return task.CreatedAt.UTC()
	}
	return time.Time{}
}
