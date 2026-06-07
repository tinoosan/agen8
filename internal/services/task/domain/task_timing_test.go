package domain

import (
	"testing"
	"time"
)

// tp returns a pointer to a time offset from fixedTime (declared in
// task_aggregate_test.go) by the given duration — keeps the timing cases terse.
func tp(d time.Duration) *time.Time {
	t := fixedTime.Add(d)
	return &t
}

func TestPickupLatency(t *testing.T) {
	t.Run("nil when never claimed", func(t *testing.T) {
		task := Task{CreatedAt: tp(0)}
		if got := task.PickupLatency(); got != nil {
			t.Fatalf("PickupLatency=%v want nil", got)
		}
	})
	t.Run("nil when creation time unknown", func(t *testing.T) {
		task := Task{StartedAt: tp(time.Minute)}
		if got := task.PickupLatency(); got != nil {
			t.Fatalf("PickupLatency=%v want nil", got)
		}
	})
	t.Run("started minus created", func(t *testing.T) {
		task := Task{CreatedAt: tp(0), StartedAt: tp(3 * time.Minute)}
		got := task.PickupLatency()
		if got == nil || *got != 3*time.Minute {
			t.Fatalf("PickupLatency=%v want 3m", got)
		}
	})
	t.Run("clamps clock skew to zero", func(t *testing.T) {
		task := Task{CreatedAt: tp(5 * time.Minute), StartedAt: tp(time.Minute)}
		got := task.PickupLatency()
		if got == nil || *got != 0 {
			t.Fatalf("PickupLatency=%v want 0", got)
		}
	})
}

func TestInProgressDuration(t *testing.T) {
	now := fixedTime.Add(time.Hour)
	t.Run("nil when never started", func(t *testing.T) {
		task := Task{CreatedAt: tp(0)}
		if got := task.InProgressDuration(now); got != nil {
			t.Fatalf("InProgressDuration=%v want nil", got)
		}
	})
	t.Run("uses now while in flight", func(t *testing.T) {
		task := Task{StartedAt: tp(10 * time.Minute)} // started, not completed
		got := task.InProgressDuration(now)
		want := 50 * time.Minute // now is +60m, started +10m
		if got == nil || *got != want {
			t.Fatalf("InProgressDuration=%v want %v", got, want)
		}
	})
	t.Run("uses completedAt when finished", func(t *testing.T) {
		task := Task{StartedAt: tp(10 * time.Minute), CompletedAt: tp(25 * time.Minute)}
		got := task.InProgressDuration(now)
		if got == nil || *got != 15*time.Minute {
			t.Fatalf("InProgressDuration=%v want 15m", got)
		}
	})
	t.Run("clamps clock skew to zero", func(t *testing.T) {
		task := Task{StartedAt: tp(30 * time.Minute), CompletedAt: tp(10 * time.Minute)}
		got := task.InProgressDuration(now)
		if got == nil || *got != 0 {
			t.Fatalf("InProgressDuration=%v want 0", got)
		}
	})
}

func TestIsOverThreshold(t *testing.T) {
	now := fixedTime.Add(time.Hour)
	t.Run("true when in-flight task exceeds threshold", func(t *testing.T) {
		task := Task{Status: TaskStatusActive, StartedAt: tp(0)} // in progress 60m
		if !task.IsOverThreshold(30*time.Minute, now) {
			t.Fatal("expected breach for 60m > 30m threshold")
		}
	})
	t.Run("false when under threshold", func(t *testing.T) {
		task := Task{Status: TaskStatusActive, StartedAt: tp(0)} // 60m
		if task.IsOverThreshold(90*time.Minute, now) {
			t.Fatal("expected no breach for 60m < 90m threshold")
		}
	})
	t.Run("terminal tasks never breach", func(t *testing.T) {
		task := Task{Status: TaskStatusSucceeded, StartedAt: tp(0), CompletedAt: tp(90 * time.Minute)}
		if task.IsOverThreshold(time.Minute, now) {
			t.Fatal("terminal task should never breach")
		}
	})
	t.Run("never-started task never breaches", func(t *testing.T) {
		task := Task{Status: TaskStatusPending, CreatedAt: tp(0)}
		if task.IsOverThreshold(time.Minute, now) {
			t.Fatal("unstarted task should never breach")
		}
	})
	t.Run("non-positive threshold disables the check", func(t *testing.T) {
		task := Task{Status: TaskStatusActive, StartedAt: tp(0)}
		if task.IsOverThreshold(0, now) {
			t.Fatal("threshold of 0 should disable the check")
		}
	})
}
