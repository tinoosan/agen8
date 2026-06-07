package domain

import (
	"testing"
	"time"
)

func tptr(t time.Time) *time.Time { return &t }

func specByTrigger(specs []Spec, trigger string) (Spec, bool) {
	for _, s := range specs {
		if s.Trigger == trigger {
			return s, true
		}
	}
	return Spec{}, false
}

func TestDeriveBacklogNudgeFiresAndEscalates(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	// Disable the age-based nudges so we isolate the backlog count behaviour.
	cfg := DeriveConfig{BacklogWarnThreshold: 3, BacklogCriticalThreshold: 5}

	pending := func(n int) []TaskSnapshot {
		out := make([]TaskSnapshot, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, TaskSnapshot{ID: "t", ProjectID: "proj", Status: "pending", CreatedAt: tptr(now)})
		}
		return out
	}

	// Below threshold → no nudge.
	if _, ok := specByTrigger(Derive("proj", pending(2), now, cfg), TriggerBacklogHigh); ok {
		t.Fatalf("backlog nudge should not fire below threshold")
	}

	// At warn threshold → warning, verbatim body, project subject.
	warn, ok := specByTrigger(Derive("proj", pending(3), now, cfg), TriggerBacklogHigh)
	if !ok {
		t.Fatalf("backlog nudge should fire at warn threshold")
	}
	if warn.Severity != SeverityWarning {
		t.Fatalf("severity = %q, want warning", warn.Severity)
	}
	const verbatim = "your backlog is quite high; maybe complete some tasks before you create new ones"
	if warn.Body != verbatim {
		t.Fatalf("body = %q, want verbatim nudge", warn.Body)
	}
	if warn.SubjectKind != SubjectProject || warn.SubjectID != "proj" {
		t.Fatalf("subject = %q/%q, want project/proj", warn.SubjectKind, warn.SubjectID)
	}
	if warn.Kind != KindStanding {
		t.Fatalf("backlog nudge should be a standing kind")
	}

	// At critical threshold → escalates to critical, body unchanged.
	crit, _ := specByTrigger(Derive("proj", pending(5), now, cfg), TriggerBacklogHigh)
	if crit.Severity != SeverityCritical {
		t.Fatalf("severity = %q, want critical", crit.Severity)
	}
	if crit.Body != verbatim {
		t.Fatalf("critical body should stay verbatim, got %q", crit.Body)
	}
	if crit.Metadata["backlog"] != "5" {
		t.Fatalf("backlog metadata = %q, want 5", crit.Metadata["backlog"])
	}
}

func TestDeriveTaskEvents(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cfg := DeriveConfig{EventLookback: time.Hour}

	tasks := []TaskSnapshot{
		{ID: "done-recent", ProjectID: "proj", Title: "Ship it", Status: "succeeded", CompletedAt: tptr(now.Add(-10 * time.Minute))},
		{ID: "done-old", ProjectID: "proj", Title: "Old", Status: "succeeded", CompletedAt: tptr(now.Add(-3 * time.Hour))},
		{ID: "done-undated", ProjectID: "proj", Title: "No date", Status: "succeeded"},
		{ID: "review-recent", ProjectID: "proj", Title: "Review me", Status: "in_review", UpdatedAt: tptr(now.Add(-5 * time.Minute))},
	}
	specs := Derive("proj", tasks, now, cfg)

	completed, ok := specByTrigger(specs, TriggerTaskCompleted)
	if !ok {
		t.Fatalf("expected a completed notification for the recent task")
	}
	if completed.SubjectID != "done-recent" {
		t.Fatalf("completed subject = %q, want done-recent (old/undated should be excluded)", completed.SubjectID)
	}
	if completed.LinkURL != "/project/proj/tasks/done-recent" {
		t.Fatalf("completed link = %q", completed.LinkURL)
	}

	review, ok := specByTrigger(specs, TriggerTaskInReview)
	if !ok {
		t.Fatalf("expected an in-review notification")
	}
	if review.Severity != SeverityWarning {
		t.Fatalf("in-review severity = %q, want warning", review.Severity)
	}

	// Only one completed spec — the old and undated ones are filtered out.
	count := 0
	for _, s := range specs {
		if s.Trigger == TriggerTaskCompleted {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("completed specs = %d, want 1", count)
	}
}

func TestDeriveStaleAndOverrun(t *testing.T) {
	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	cfg := DeriveConfig{StaleQueuedAfter: time.Hour, OverrunAfter: time.Hour}

	tasks := []TaskSnapshot{
		{ID: "fresh", ProjectID: "proj", Status: "pending", CreatedAt: tptr(now.Add(-10 * time.Minute))},
		{ID: "stale", ProjectID: "proj", Title: "Waiting", Status: "pending", CreatedAt: tptr(now.Add(-90 * time.Minute))},
		{ID: "verystale", ProjectID: "proj", Title: "Forgotten", Status: "pending", CreatedAt: tptr(now.Add(-3 * time.Hour))},
		{ID: "running", ProjectID: "proj", Title: "Churning", Status: "active", StartedAt: tptr(now.Add(-2 * time.Hour))},
	}
	specs := Derive("proj", tasks, now, cfg)

	// Two stale-queued specs (stale + verystale), fresh excluded.
	stale := 0
	var veryStaleSeverity Severity
	for _, s := range specs {
		if s.Trigger == TriggerTaskStale {
			stale++
			if s.SubjectID == "verystale" {
				veryStaleSeverity = s.Severity
			}
		}
	}
	if stale != 2 {
		t.Fatalf("stale specs = %d, want 2", stale)
	}
	if veryStaleSeverity != SeverityCritical {
		t.Fatalf("verystale severity = %q, want critical (>= 2x threshold)", veryStaleSeverity)
	}

	overrun, ok := specByTrigger(specs, TriggerTaskOverrun)
	if !ok {
		t.Fatalf("expected an overrun notification")
	}
	if overrun.Kind != KindStanding {
		t.Fatalf("overrun should be standing")
	}
}

func TestFormatCoarseDuration(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second:     "<1m",
		5 * time.Minute:      "5m",
		65 * time.Minute:     "1h 5m",
		2 * time.Hour:        "2h",
		(24 + 4) * time.Hour: "1d 4h",
		48 * time.Hour:       "2d",
	}
	for d, want := range cases {
		if got := formatCoarseDuration(d); got != want {
			t.Errorf("formatCoarseDuration(%s) = %q, want %q", d, got, want)
		}
	}
}
