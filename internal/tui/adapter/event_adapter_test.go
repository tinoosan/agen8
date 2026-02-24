package adapter

import (
	"testing"
	"time"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestEventRecordToActivity_TaskDoneUsesTaskID(t *testing.T) {
	ev := types.EventRecord{
		EventID:   "event-123",
		RunID:     "run-1",
		Timestamp: time.Now(),
		Type:      "task.done",
		Message:   "Task finished",
		Data:      map[string]string{"taskId": "task-abc", "summary": "Done"},
	}

	act, ok := EventRecordToActivity(ev)
	if !ok {
		t.Fatalf("expected task.done to map to activity")
	}
	if act.ID != "task-abc" {
		t.Fatalf("expected ID task-abc, got %q", act.ID)
	}
	if act.Title != "Done" {
		t.Fatalf("expected summary title, got %q", act.Title)
	}
}
