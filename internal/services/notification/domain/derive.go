package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DeriveConfig holds the advisory thresholds the projection uses. These are
// soft signals, not enforced gates: crossing one surfaces a nudge, it never
// blocks work. Defaults are deliberately generous so the inbox stays useful
// rather than noisy.
type DeriveConfig struct {
	// BacklogWarnThreshold is the queued-task count at which the backlog nudge
	// first fires (at warning severity). Zero disables the nudge.
	BacklogWarnThreshold int
	// BacklogCriticalThreshold is the queued-task count at which the backlog
	// nudge escalates to critical. Should be >= BacklogWarnThreshold.
	BacklogCriticalThreshold int
	// StaleQueuedAfter is how long a task may sit pending before it is flagged
	// as stuck in the queue. Zero disables the stale-queued nudge.
	StaleQueuedAfter time.Duration
	// OverrunAfter is how long a task may stay in progress before it is flagged
	// as running long. Zero disables the overrun nudge.
	OverrunAfter time.Duration
	// EventLookback bounds one-time event notifications (completed / in-review)
	// to recent activity, so the very first sync doesn't flood the inbox with a
	// notification for every historically-finished task. Zero means "no bound".
	EventLookback time.Duration
}

// DefaultDeriveConfig returns sensible advisory defaults.
func DefaultDeriveConfig() DeriveConfig {
	return DeriveConfig{
		BacklogWarnThreshold:     8,
		BacklogCriticalThreshold: 20,
		StaleQueuedAfter:         2 * time.Hour,
		OverrunAfter:             3 * time.Hour,
		EventLookback:            24 * time.Hour,
	}
}

// Derive is the pure heart of the feature: given the current tasks for a
// project and the wall clock, it returns the notifications that *should* exist
// right now. It performs no I/O and is fully deterministic for a given input,
// which is what lets the reconciler treat it as idempotent.
func Derive(projectID string, tasks []TaskSnapshot, now time.Time, cfg DeriveConfig) []Spec {
	specs := make([]Spec, 0, len(tasks)+1)
	backlog := 0

	for _, t := range tasks {
		switch t.Status {
		case "pending":
			backlog++
			if cfg.StaleQueuedAfter > 0 && t.CreatedAt != nil {
				if age := now.Sub(t.CreatedAt.UTC()); age >= cfg.StaleQueuedAfter {
					specs = append(specs, Spec{
						Trigger:     TriggerTaskStale,
						Kind:        KindStanding,
						Severity:    ageSeverity(age, cfg.StaleQueuedAfter),
						SubjectKind: SubjectTask,
						SubjectID:   t.ID,
						Title:       "Task stuck in the queue",
						Body:        fmt.Sprintf("%s has been queued for %s without being picked up.", taskLabel(t), formatCoarseDuration(age)),
						LinkSurface: "task",
						LinkURL:     taskLink(t),
						ThrottleKey: "task.stale:" + t.ID,
					})
				}
			}

		case "active":
			if cfg.OverrunAfter > 0 && t.StartedAt != nil {
				if dur := now.Sub(t.StartedAt.UTC()); dur >= cfg.OverrunAfter {
					specs = append(specs, Spec{
						Trigger:     TriggerTaskOverrun,
						Kind:        KindStanding,
						Severity:    ageSeverity(dur, cfg.OverrunAfter),
						SubjectKind: SubjectTask,
						SubjectID:   t.ID,
						Title:       "Task running long",
						Body:        fmt.Sprintf("%s has been in progress for %s.", taskLabel(t), formatCoarseDuration(dur)),
						LinkSurface: "task",
						LinkURL:     taskLink(t),
						ThrottleKey: "task.overrun:" + t.ID,
					})
				}
			}

		case "in_review":
			// Entering review updates the task, so UpdatedAt is the best proxy
			// for "when did this land in my review queue".
			if withinLookback(t.UpdatedAt, now, cfg.EventLookback) {
				specs = append(specs, Spec{
					Trigger:     TriggerTaskInReview,
					Kind:        KindEvent,
					Severity:    SeverityWarning, // needs a human — louder than an FYI
					SubjectKind: SubjectTask,
					SubjectID:   t.ID,
					Title:       "Task awaiting your review",
					Body:        fmt.Sprintf("%s finished and is waiting for review.", taskLabel(t)),
					LinkSurface: "task",
					LinkURL:     taskLink(t),
					ThrottleKey: "task.in_review:" + t.ID,
				})
			}

		case "succeeded":
			if withinLookback(t.CompletedAt, now, cfg.EventLookback) {
				specs = append(specs, Spec{
					Trigger:     TriggerTaskCompleted,
					Kind:        KindEvent,
					Severity:    SeverityInfo,
					SubjectKind: SubjectTask,
					SubjectID:   t.ID,
					Title:       "Task completed",
					Body:        fmt.Sprintf("%s was approved and marked done.", taskLabel(t)),
					LinkSurface: "task",
					LinkURL:     taskLink(t),
					ThrottleKey: "task.completed:" + t.ID,
				})
			}
		}
	}

	if cfg.BacklogWarnThreshold > 0 && backlog >= cfg.BacklogWarnThreshold {
		severity := SeverityWarning
		if cfg.BacklogCriticalThreshold > 0 && backlog >= cfg.BacklogCriticalThreshold {
			severity = SeverityCritical
		}
		specs = append(specs, Spec{
			Trigger:     TriggerBacklogHigh,
			Kind:        KindStanding,
			Severity:    severity,
			SubjectKind: SubjectProject,
			SubjectID:   projectID,
			Title:       "Backlog is piling up",
			// Verbatim nudge copy from the acceptance criteria — do not reword.
			Body:        "your backlog is quite high; maybe complete some tasks before you create new ones",
			LinkSurface: "tasks",
			LinkURL:     "",
			ThrottleKey: "backlog.high:" + projectID,
			Metadata:    map[string]string{"backlog": strconv.Itoa(backlog)},
		})
	}

	return specs
}

// ageSeverity escalates a duration-based nudge: warning once it crosses the
// threshold, critical once it doubles it.
func ageSeverity(elapsed, threshold time.Duration) Severity {
	if threshold > 0 && elapsed >= 2*threshold {
		return SeverityCritical
	}
	return SeverityWarning
}

// withinLookback reports whether ts is set and no older than lookback. A zero
// lookback means "unbounded" (any non-nil timestamp qualifies). A nil timestamp
// never qualifies — we won't emit an event we can't date.
func withinLookback(ts *time.Time, now time.Time, lookback time.Duration) bool {
	if ts == nil {
		return false
	}
	if lookback <= 0 {
		return true
	}
	return now.Sub(ts.UTC()) <= lookback
}

func taskLabel(t TaskSnapshot) string {
	if title := strings.TrimSpace(t.Title); title != "" {
		return "\"" + title + "\""
	}
	return "A task"
}

func taskLink(t TaskSnapshot) string {
	id := strings.TrimSpace(t.ID)
	project := strings.TrimSpace(t.ProjectID)
	if id == "" || project == "" {
		return ""
	}
	return "/project/" + project + "/tasks/" + id
}

// formatCoarseDuration renders a duration at minute granularity, mirroring the
// frontend's taskTiming formatter so backend-authored bodies read the same as
// client-side labels: "<1m", "45m", "2h 15m", "3d 4h".
func formatCoarseDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	totalMinutes := int(d / time.Minute)
	days := totalMinutes / (60 * 24)
	hours := (totalMinutes % (60 * 24)) / 60
	minutes := totalMinutes % 60

	switch {
	case days > 0:
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	case hours > 0:
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
