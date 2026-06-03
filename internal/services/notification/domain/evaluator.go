package domain

import (
	"context"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// TriggerEvaluator is the OCP extension point for domain-specific notification logic.
// Each bounded context implements this interface to plug into the notification system.
// Adding a new domain = adding a new evaluator. The Service never needs
// to change.
type TriggerEvaluator interface {
	// Source returns the domain identifier (e.g. "heartbeat", "task").
	Source() string

	// Evaluate inspects a domain event and returns zero or more notifications
	// to raise. Returns nil if the event doesn't warrant notification.
	// The evaluator owns the trigger logic — it knows what "critical" means
	// in its domain.
	Evaluate(ctx context.Context, event types.EventRecord) []Notification
}

// SubjectResolver is an optional capability evaluators can implement to
// declare which active notifications should be auto-dismissed in response
// to a domain event. Typical use: a "thing X needs attention" notification
// is raised on TopicX.pending, then auto-cleared on TopicX.resolved without
// requiring the user to manually dismiss it.
//
// The notification service type-asserts each registered TriggerEvaluator
// against this interface during HandleEvent, so adopting it is opt-in per
// domain. Evaluators that don't need auto-dismiss simply don't implement it.
//
// Resolve must be deterministic and side-effect free — the service applies
// the returned dismissals through the canonical DismissBySubject path so
// the broadcast and store invariants stay in one place.
type SubjectResolver interface {
	Resolve(ctx context.Context, event types.EventRecord) []SubjectDismissal
}

// SubjectDismissal addresses a set of notifications scoped by the same
// (UserID, Source, Subject) triple that produced them. The notification
// service forwards each dismissal to NotificationRepository.DismissBySubject.
type SubjectDismissal struct {
	UserID  string
	Source  string
	Subject Subject
}
