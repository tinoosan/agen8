package domain

import "time"

// Task timing measures, kept deliberately DISTINCT (per the agent-management
// mission KR). The user was explicit that the span that matters most is "how
// long it takes for an agent to be aware of a task" — pickup/queue latency —
// NOT execution time. We expose both, separately:
//
//   - Pickup latency: CreatedAt (queued) → StartedAt (first claimed). How long
//     work sat unpicked. This is the "time to awareness" figure.
//   - In-progress duration: StartedAt → CompletedAt (or now, while in flight).
//     How long work has been open since it began.
//
// Both derive from timestamps that already exist on the Task — verified against
// the lifecycle before adding any field, which is why this needs no migration:
//   - CreatedAt  is set at creation (the queued moment).
//   - StartedAt  is stamped once in Claim() on the first pending→active move and
//     never overwritten (Release keeps it; a re-claim is guarded by nil-check),
//     so it is a stable "work first began" marker.
//   - CompletedAt is stamped on the terminal transitions (approve/fail/cancel).
//
// In-progress duration is WALL-CLOCK and does not subtract blocked/in_review
// intervals. A precise "active-only" measure would require a status-transition
// history, which is intentionally deferred — the MVP signal the user asked for
// ("a task worked past a threshold of time, I want to know") is well served by
// elapsed-since-start, and the cost of a transition log isn't justified yet.

// PickupLatency returns StartedAt - CreatedAt: how long the task waited from
// being queued until it was first claimed. Returns nil when either timestamp is
// absent (never claimed, or unknown creation time). Clock skew that would yield
// a negative span is clamped to zero.
func (t Task) PickupLatency() *time.Duration {
	if t.CreatedAt == nil || t.StartedAt == nil {
		return nil
	}
	d := t.StartedAt.Sub(*t.CreatedAt)
	if d < 0 {
		d = 0
	}
	return &d
}

// InProgressDuration returns (CompletedAt ?? now) - StartedAt: how long the task
// has been in flight since work began. For a finished task the closed span uses
// CompletedAt; for a task still in progress the open end is `now`. Returns nil
// when the task was never started. A clock is passed in so the value is testable
// and deterministic. Negative spans (skew) are clamped to zero.
func (t Task) InProgressDuration(now time.Time) *time.Duration {
	if t.StartedAt == nil {
		return nil
	}
	end := now
	if t.CompletedAt != nil {
		end = *t.CompletedAt
	}
	d := end.Sub(*t.StartedAt)
	if d < 0 {
		d = 0
	}
	return &d
}

// IsOverThreshold reports whether a still-in-flight task has been in progress
// longer than `threshold` — the "agent over its time threshold on a task"
// signal. Terminal tasks never breach (the work is done), a task that never
// started never breaches, and a non-positive threshold disables the check.
func (t Task) IsOverThreshold(threshold time.Duration, now time.Time) bool {
	if threshold <= 0 || t.IsTerminal() {
		return false
	}
	d := t.InProgressDuration(now)
	if d == nil {
		return false
	}
	return *d > threshold
}
