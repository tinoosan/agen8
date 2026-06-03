// Package eventbus defines domain event types for the Watermill-based event bus.
package eventbus

import (
	"time"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

// Topic constants for domain events.
const (
	TopicMessagePublished     = "message.published"
	TopicSpaceMemberLifecycle = "space.member.lifecycle"
	TopicTaskLifecycle        = "task.lifecycle"
	TopicOALifecycle          = "oa.lifecycle"
	TopicEscalationLifecycle  = "escalation.lifecycle"
	TopicKRProgress           = "kr.progress"
	TopicDecisionLogged       = "decision.logged"
	TopicMissionLifecycle     = "mission.lifecycle"
	TopicPlanLifecycle        = "plan.lifecycle"
	TopicHumanInputLifecycle  = "human_input.lifecycle"
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

// MessagePublishedEvent is emitted when a communication message is published.
// Handlers persist the message, route it to the destination space/member, and
// inject it for mid-turn steering.
type MessagePublishedEvent struct {
	Message     types.Message       `json:"message"`
	Envelope    types.MemberMessage `json:"envelope"`
	Source      string              `json:"source"`
	PublishedAt time.Time           `json:"publishedAt"`
}

// TaskLifecycleEvent is emitted on task state transitions (creation, completion,
// failure, cancellation). Subscribers such as the policy escalation engine use
// these events to trigger automated operator actions.
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

// OALifecycleEvent is emitted on operator action status transitions.
// Subscribers can use these events for notifications, space projections,
// and cross-surface state invalidation.
type OALifecycleEvent struct {
	ProjectID     string    `json:"projectId"`
	SpaceID       string    `json:"spaceId"`
	ActionID      string    `json:"actionId"`
	TaskRef       string    `json:"taskRef,omitempty"`
	EventType     string    `json:"eventType"` // "oa.created", "oa.acknowledged", "oa.started", "oa.completed", "oa.blocked", "oa.unblocked", "oa.canceled", "oa.verified"
	OldStatus     string    `json:"oldStatus"`
	NewStatus     string    `json:"newStatus"`
	Title         string    `json:"title,omitempty"`
	Urgency       string    `json:"urgency,omitempty"`
	Category      string    `json:"category,omitempty"`
	OutcomeStatus string    `json:"outcomeStatus,omitempty"` // Only on completion
	Author        string    `json:"author,omitempty"`
	Text          string    `json:"text,omitempty"`
	Blocking      bool      `json:"blocking"`
	Timestamp     time.Time `json:"timestamp"`
}

// EscalationLifecycleEvent is emitted on escalation status transitions
// (created, escalated, resolved, canceled). Used by notification evaluator (F4)
// and space projection integration.
type EscalationLifecycleEvent struct {
	ProjectID       string    `json:"projectId"`
	SpaceID         string    `json:"spaceId"`
	EscalationID    string    `json:"escalationId"`
	TaskRef         string    `json:"taskRef,omitempty"`
	EventType       string    `json:"eventType"` // "escalation.created", "escalation.escalated", "escalation.resolved", "escalation.canceled"
	OldStatus       string    `json:"oldStatus"`
	NewStatus       string    `json:"newStatus"`
	Resolution      string    `json:"resolution,omitempty"`      // Only on resolved
	ResolvedBy      string    `json:"resolvedBy,omitempty"`      // Only on resolved
	Title           string    `json:"title,omitempty"`           // For notification evaluator (F4)
	Urgency         string    `json:"urgency,omitempty"`         // For notification evaluator (F4)
	PreviousUrgency string    `json:"previousUrgency,omitempty"` // Only on auto-escalation
	NewUrgency      string    `json:"newUrgency,omitempty"`      // Only on auto-escalation
	Category        string    `json:"category,omitempty"`        // For notification evaluator (F4)
	Timestamp       time.Time `json:"timestamp"`
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
// (activated, paused, completed, archived). Subscribers use these events
// for coordinator activation, heartbeat lifecycle management, and
// operator notifications.
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
	AuthorType  string `json:"authorType,omitempty"` // "coordinator" | "operator" — for plan.commented
	Text        string `json:"text,omitempty"`       // comment/rejection/veto text (truncated to 500 chars)
}

// HumanInputLifecycleEvent is emitted when a pending human-input row is
// inserted (Kind="pending") or removed (Kind="resolved") by the runtime
// supervisor. Subscribers turn pending events into operator notifications
// so the operator sees the question even when not viewing the channel.
//
// MemberID is the asker's stable id. MemberName is the resolved display
// name at publish time — UI surfaces should prefer MemberName.
//
// DeclarationKind distinguishes question sets ("questions") from
// approve-reject prompts ("approve_reject"). Blocking is true when the
// underlying question set has at least one blocking question, used to
// pick critical-vs-warning severity in the inbox.
//
// Resolution is "" for Kind="pending", "answered" / "cancelled" for
// Kind="resolved". The (Kind, Resolution) pair lets the notification
// service auto-dismiss the matching pending notification by Subject
// when the question is answered or cancelled.
type HumanInputLifecycleEvent struct {
	Kind string `json:"kind"` // "pending" | "resolved"
	// UserID is the routing identity for inbox notifications. The
	// supervisor stamps this at publish time so downstream subscribers
	// don't have to infer it from other fields. In single-user-local
	// mode UserID == ProjectID; hosted multi-user mode will populate
	// it from the authenticated session. Always populated — empty
	// means the supervisor failed to resolve a routing user and the
	// event should be dropped rather than guess.
	UserID          string    `json:"userId"`
	ProjectID       string    `json:"projectId"`
	SpaceID         string    `json:"spaceId"`
	ChannelID       string    `json:"channelId"`
	MemberID        string    `json:"memberId,omitempty"`
	MemberName      string    `json:"memberName,omitempty"`
	ToolCallID      string    `json:"toolCallId"`
	ToolName        string    `json:"toolName,omitempty"`
	DeclarationKind string    `json:"declarationKind,omitempty"` // "questions" | "approve_reject"
	Title           string    `json:"title,omitempty"`
	QuestionCount   int       `json:"questionCount,omitempty"`
	Blocking        bool      `json:"blocking,omitempty"`
	Resolution      string    `json:"resolution,omitempty"` // "" | "answered" | "cancelled"
	Timestamp       time.Time `json:"timestamp"`
}
