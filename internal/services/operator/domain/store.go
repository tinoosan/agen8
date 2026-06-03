package domain

import (
	"context"
	"time"
)

type ActionRepository interface {
	SaveAction(ctx context.Context, action OperatorAction) error
	GetAction(ctx context.Context, id OperatorActionID) (OperatorAction, error)
	FindActionsByProject(ctx context.Context, projectID string, filter ActionFilter) ([]OperatorAction, error)
	FindActionsByTask(ctx context.Context, taskRef string) ([]OperatorAction, error)
	FindPendingActions(ctx context.Context, projectID string) ([]OperatorAction, error)
	CountActionsByStatus(ctx context.Context, projectID string) (map[OAStatus]int, error)
	FindActionByAttachmentID(ctx context.Context, attachmentID string) (OperatorAction, error)
}

type ActionFilter struct {
	Status   []OAStatus
	Urgency  []Urgency
	Category []Category
	SpaceID  string
	Limit    int
	Offset   int
}

type EscalationRepository interface {
	SaveEscalation(ctx context.Context, esc Escalation) error
	GetEscalation(ctx context.Context, id EscalationID) (Escalation, error)
	FindEscalationsByProject(ctx context.Context, projectID string, filter EscalationFilter) ([]Escalation, error)
	FindPendingEscalationsByTask(ctx context.Context, taskRef string) ([]Escalation, error)
	CountPendingEscalations(ctx context.Context, projectID string) (int, error)
	UpdateEscalationStatus(ctx context.Context, id EscalationID, status Status, resolution Resolution, note string, resolvedBy string) error
	FindExpiredPendingEscalations(ctx context.Context, now time.Time) ([]Escalation, error)
	EscalateEscalationUrgency(ctx context.Context, id EscalationID, newUrgency Urgency, originalUrgency Urgency, escalatedAt time.Time) error
	// FindPendingEscalationDuplicate keys dedup on (spaceID, taskRef, category, urgency, since).
	// Adding urgency to the tuple closes a bug where re-escalating at higher urgency
	// silently merged with a still-pending lower-urgency duplicate.
	FindPendingEscalationDuplicate(ctx context.Context, spaceID, taskRef string, category Category, urgency Urgency, since time.Time) (Escalation, bool, error)
}

type EscalationFilter struct {
	Status   []Status
	Urgency  []Urgency
	Category []Category
	SpaceID  string
	Limit    int
	Offset   int
}
