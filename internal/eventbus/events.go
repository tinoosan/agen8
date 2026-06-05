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
	TaskRef      string    `json:"taskRef,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}
