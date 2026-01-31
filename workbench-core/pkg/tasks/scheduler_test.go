package tasks

import (
	"testing"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

func TestReadyTasks_OrdersByPriorityThenTime(t *testing.T) {
	earlier := time.Now().Add(-1 * time.Hour)
	later := time.Now()

	tasks := map[string]types.Task{
		"a": {TaskID: "a", Goal: "low", Priority: 2, CreatedAt: &earlier},
		"b": {TaskID: "b", Goal: "high", Priority: 0, CreatedAt: &later},
		"c": {TaskID: "c", Goal: "mid", Priority: 1, CreatedAt: &later},
		"d": {TaskID: "d", Goal: "mid-old", Priority: 1, CreatedAt: &earlier},
	}

	got := ReadyTasks(tasks)
	order := []string{}
	for _, tsk := range got {
		order = append(order, tsk.TaskID)
	}

	expected := []string{"b", "d", "c", "a"}
	if len(order) != len(expected) {
		t.Fatalf("unexpected count %v", order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Fatalf("order[%d]=%s want %s (full=%v)", i, order[i], expected[i], order)
		}
	}
}
