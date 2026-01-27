package orchestrator

import (
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

// IsTaskReady returns true if all dependencies are completed successfully.
func IsTaskReady(task types.Task, all map[string]types.Task) bool {
	if len(task.WaitFor) == 0 {
		return true
	}
	for _, dep := range task.WaitFor {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		other, ok := all[dep]
		if !ok {
			return false
		}
		if !isTaskSucceeded(other.Status) {
			return false
		}
	}
	return true
}

// ReadyTasks returns tasks that are ready to run, ordered by priority then creation time.
func ReadyTasks(all map[string]types.Task) []types.Task {
	out := make([]types.Task, 0, len(all))
	for _, task := range all {
		if IsTaskReady(task, all) {
			out = append(out, task)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi := priorityRank(out[i].Priority)
		pj := priorityRank(out[j].Priority)
		if pi != pj {
			return pi < pj
		}
		return taskTime(out[i]).Before(taskTime(out[j]))
	})
	return out
}

func isTaskSucceeded(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "success", "done", "complete", "completed":
		return true
	default:
		return false
	}
}

func priorityRank(p string) int {
	switch strings.ToUpper(strings.TrimSpace(p)) {
	case "P0":
		return 0
	case "P1":
		return 1
	case "P2":
		return 2
	case "P3":
		return 3
	default:
		return 9
	}
}

func taskTime(task types.Task) time.Time {
	if task.CreatedAt != nil {
		return task.CreatedAt.UTC()
	}
	return time.Time{}
}
