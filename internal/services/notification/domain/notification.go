// Package domain holds the notification model and the pure projection that
// turns the current task snapshot into the notifications a user should see.
//
// Notifications here are a *derived* projection: nothing publishes them
// directly. Derive() reads the current set of tasks and emits Specs (desired
// notifications); the application service reconciles those Specs against the
// persisted notifications table. This keeps the producer side stateless and
// idempotent — re-running Derive over the same snapshot yields the same Specs,
// and the throttle key dedups them so a notification is created at most once.
package domain

import "time"

// Severity ranks how loudly a notification should present. It escalates with
// workload (e.g. the backlog nudge climbs info → warning → critical as the
// queue grows) rather than firing every event at the same volume.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Trigger identifies which rule produced a notification. Stored verbatim so the
// UI can group/filter and so reconciliation can tell standing nudges apart from
// one-time events.
const (
	TriggerTaskCompleted = "task.completed"    // a task was approved and marked done
	TriggerTaskInReview  = "task.in_review"    // a task finished and is awaiting human review
	TriggerBacklogHigh   = "backlog.high"      // the queued backlog crossed the nudge threshold
	TriggerTaskStale     = "task.stale_queued" // a task sat in the queue too long unclaimed
	TriggerTaskOverrun   = "task.overrun"      // an in-progress task has been running unusually long
)

// Subject kinds describe what a notification points at, used for deep-linking
// and (for standing nudges) auto-dismiss when the underlying entity resolves.
const (
	SubjectTask    = "task"
	SubjectProject = "project"
)

// Source tags the producer. Single-valued today; kept as a column so a future
// integration (CI, external webhook) can file into the same inbox.
const Source = "agen8"

// Kind distinguishes the two notification lifecycles, which reconcile
// differently:
//
//   - KindEvent: a one-time fact ("task completed"). Once created it is never
//     recreated for the same subject, even after the user dismisses it — the
//     throttle key acts as an "already told you" marker.
//   - KindStanding: a condition that is currently true ("backlog is high").
//     It lives while the condition holds and is auto-dismissed the moment the
//     condition clears, so the inbox reflects reality rather than history.
type Kind string

const (
	KindEvent    Kind = "event"
	KindStanding Kind = "standing"
)

// IsStandingTrigger reports whether a persisted notification (identified only by
// its trigger string) is a standing nudge — the set the reconciler is allowed to
// auto-dismiss when its condition no longer appears in the derived Specs.
func IsStandingTrigger(trigger string) bool {
	switch trigger {
	case TriggerBacklogHigh, TriggerTaskStale, TriggerTaskOverrun:
		return true
	default:
		return false
	}
}

// TaskSnapshot is the minimal, neutral view of a task that Derive needs. Keeping
// it free of the task service's own domain type lets the notification package
// stay decoupled (the app layer adapts task.Task → TaskSnapshot at the seam).
type TaskSnapshot struct {
	ID          string
	ProjectID   string
	Title       string
	Status      string
	CreatedAt   *time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	UpdatedAt   *time.Time
	ClaimedBy   string
}

// Notification is the persisted entity: one row per thing the user should see.
type Notification struct {
	ID          string
	UserID      string
	ProjectID   string
	Source      string
	Trigger     string
	Severity    Severity
	SubjectKind string
	SubjectID   string
	Title       string
	Body        string
	LinkSurface string
	LinkURL     string
	ThrottleKey string
	Metadata    map[string]string
	CreatedAt   time.Time
	ReadAt      *time.Time
	DismissedAt *time.Time
}

// Spec is a desired notification emitted by Derive. The reconciler turns new
// Specs into persisted Notifications; ThrottleKey is the identity used for
// dedup and for matching standing nudges across sync passes.
type Spec struct {
	Trigger     string
	Kind        Kind
	Severity    Severity
	SubjectKind string
	SubjectID   string
	Title       string
	Body        string
	LinkSurface string
	LinkURL     string
	ThrottleKey string
	Metadata    map[string]string
}
