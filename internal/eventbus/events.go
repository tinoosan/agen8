// Package eventbus defines domain event types for the Watermill-based event bus.
package eventbus

import (
	"time"
)

// Topic constants for domain events.
const (
	TopicSpaceMemberLifecycle = "space.member.lifecycle"
	TopicTaskLifecycle        = "task.lifecycle"
	TopicKRProgress           = "kr.progress"
	TopicDecisionLogged       = "decision.logged"
	TopicMissionLifecycle     = "mission.lifecycle"
	TopicPlanLifecycle        = "plan.lifecycle"
)

const (
	SpaceMemberEventRegistered      = "space.member.registered"
	SpaceMemberEventConfigChanged   = "space.member.config_changed"
	SpaceMemberEventIdentityChanged = "space.member.identity_changed"
	SpaceMemberEventRemoved         = "space.member.removed"
)

type SpaceMemberLifecycleEvent struct {
	UserID         string    `json:"userId,omitempty"`
	ProjectID      string    `json:"projectId,omitempty"`
	SpaceID        string    `json:"spaceId"`
	MemberID       string    `json:"memberId"`
	ChannelID      string    `json:"channelId"`
	DisplayName    string    `json:"displayName"`
	MemberType     string    `json:"memberType"`
	EventType      string    `json:"eventType"`
	LifecycleState string    `json:"lifecycleState"`
	HarnessKind    string    `json:"harnessKind"`
	Model          string    `json:"model"`
	Effort         string    `json:"effort"`
	PermissionMode string    `json:"harnessPermissionMode,omitempty"`
	ConfigRef      string    `json:"harnessConfigRef,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
}

// Plan lifecycle event type constants.
const (
	PlanEventCreated           = "plan.created"
	PlanEventActivated         = "plan.activated" // autonomous direct-activate (draft → active)
	PlanEventSubmitted         = "plan.submitted"
	PlanEventApproved          = "plan.approved"
	PlanEventRejected          = "plan.rejected"
	PlanEventPhaseStarted      = "plan.phase_started"
	PlanEventTodoCompleted     = "plan.todo_completed"
	PlanEventPhaseCompleted    = "plan.phase_completed"
	PlanEventCompleted         = "plan.completed"
	PlanEventAbandoned         = "plan.abandoned"
	PlanEventCommented         = "plan.commented"
	PlanEventEdited            = "plan.edited"
	PlanEventAmendmentProposed = "plan.amendment_proposed"
	PlanEventAmendmentApplied  = "plan.amendment_applied"
	PlanEventAmendmentVetoed   = "plan.amendment_vetoed"
)

// TaskLifecycleEvent is emitted on task state transitions.
type TaskLifecycleEvent struct {
	ProjectID string            `json:"projectId"`
	SpaceID   string            `json:"spaceId"`
	TaskID    string            `json:"taskId"`
	RunID     string            `json:"runId"`
	EventType string            `json:"eventType"` // "task.created", "task.completed", "task.failed", "task.canceled"
	Status    string            `json:"status"`
	Values    map[string]string `json:"values,omitempty"` // Metrics/attributes for policy condition evaluation
	Timestamp time.Time         `json:"timestamp"`
}

// KRProgressEvent is emitted when key result progress is updated.
// EventType is one of: "kr.progress_updated", "kr.milestone", "kr.completed",
// "mission.completed".
type KRProgressEvent struct {
	ProjectID      string    `json:"projectId"`
	MissionID      string    `json:"missionId"`
	KeyResultID    string    `json:"keyResultId"`
	KeyResultTitle string    `json:"keyResultTitle,omitempty"`
	EventType      string    `json:"eventType"`
	OldProgress    int       `json:"oldProgress"`
	NewProgress    int       `json:"newProgress"`
	Milestone      int       `json:"milestone,omitempty"` // 25, 50, 75, or 100
	UpdatedBy      string    `json:"updatedBy"`
	Timestamp      time.Time `json:"timestamp"`
}

// MissionLifecycleEvent is emitted when a mission changes status
// (activated, paused, completed, archived).
type MissionLifecycleEvent struct {
	EventID        string    `json:"eventId,omitempty"`
	ProjectID      string    `json:"projectId"`
	MissionID      string    `json:"missionId"`
	Title          string    `json:"title"`
	EventType      string    `json:"eventType"` // "mission.activated", "mission.paused", "mission.completed", "mission.archived"
	OldStatus      string    `json:"oldStatus"`
	NewStatus      string    `json:"newStatus"`
	KRCount        int       `json:"krCount,omitempty"`
	SpaceCount     int       `json:"spaceCount,omitempty"`
	Trigger        string    `json:"trigger,omitempty"`        // "coordinator", "manual", "auto"
	TriggerSpaceID string    `json:"triggerSpaceId,omitempty"` // space that initiated the transition (used to skip self-notification)
	Timestamp      time.Time `json:"timestamp"`
}

// DecisionLoggedEvent is emitted when a decision is created.
// The existing DecisionNotificationEvaluator subscribes to these events.
//
// MemberID is the asker's stable identity. MemberName is the resolved
// display name at publish time — consumers should prefer MemberName for
// presentation so MemberID never leaks into the UI surface.
type DecisionLoggedEvent struct {
	ProjectID    string    `json:"projectId"`
	SpaceID      string    `json:"spaceId"`
	DecisionID   string    `json:"decisionId"`
	Source       string    `json:"source"`
	MemberID     string    `json:"memberId,omitempty"`
	MemberName   string    `json:"memberName,omitempty"`
	Title        string    `json:"title"`
	Confidence   float64   `json:"confidence"`
	KeyResultRef string    `json:"keyResultRef,omitempty"`
	MissionRef   string    `json:"missionRef,omitempty"`
	PlanRef      string    `json:"planRef,omitempty"`
	TaskRef      string    `json:"taskRef,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// PlanLifecycleEvent is emitted on every plan state transition and significant
// action. Title and Text carry human-readable context so downstream handlers
// never need to re-query the plan.
type PlanLifecycleEvent struct {
	ProjectID string    `json:"projectId,omitempty"`
	SpaceID   string    `json:"spaceId"`
	PlanID    string    `json:"planId"`
	Title     string    `json:"title,omitempty"` // plan title
	EventType string    `json:"eventType"`
	Status    string    `json:"status"`
	Mode      string    `json:"mode"`
	Timestamp time.Time `json:"timestamp"`
	// Contextual fields (zero-valued when not applicable):
	PhaseID     string `json:"phaseId,omitempty"`
	TodoID      string `json:"todoId,omitempty"`
	AmendmentID string `json:"amendmentId,omitempty"`
	CommentID   string `json:"commentId,omitempty"`
	AuthorType  string `json:"authorType,omitempty"`
	Text        string `json:"text,omitempty"`       // comment/rejection/veto text (truncated to 500 chars)
}
