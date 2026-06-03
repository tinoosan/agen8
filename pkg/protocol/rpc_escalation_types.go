package protocol

import "time"

const (
	MethodEscalationCreate       = "escalation.create"
	MethodEscalationGet          = "escalation.get"
	MethodEscalationList         = "escalation.list"
	MethodEscalationListPending  = "escalation.listPending"
	MethodEscalationResolve      = "escalation.resolve"
	MethodEscalationCancel       = "escalation.cancel"
	MethodEscalationCountPending = "escalation.countPending"
)

type EscalationView struct {
	ID              string            `json:"id"`
	ProjectID       string            `json:"projectId"`
	SpaceID         string            `json:"spaceId,omitempty"`
	TaskRef         string            `json:"taskRef,omitempty"`
	KeyResultRef    string            `json:"keyResultRef,omitempty"`
	MissionRef      string            `json:"missionRef,omitempty"`
	Source          string            `json:"source"`
	MemberID        string            `json:"memberId,omitempty"`
	Category        string            `json:"category"`
	Urgency         string            `json:"urgency"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	Recommendation  string            `json:"recommendation,omitempty"`
	Confidence      float64           `json:"confidence,omitempty"`
	Status          string            `json:"status"`
	Resolution      string            `json:"resolution,omitempty"`
	ResolutionNote  string            `json:"resolutionNote,omitempty"`
	Deadline        *time.Time        `json:"deadline,omitempty"`
	EscalatedAt     *time.Time        `json:"escalatedAt,omitempty"`
	OriginalUrgency string            `json:"originalUrgency,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
	ResolvedAt      *time.Time        `json:"resolvedAt,omitempty"`
	ResolvedBy      string            `json:"resolvedBy,omitempty"`
}

type EscalationCreateParams struct {
	ProjectID      string            `json:"projectId"`
	SpaceID        string            `json:"spaceId,omitempty"`
	TaskRef        string            `json:"taskRef,omitempty"`
	KeyResultRef   string            `json:"keyResultRef,omitempty"`
	MissionRef     string            `json:"missionRef,omitempty"`
	Source         string            `json:"source"`
	MemberID       string            `json:"memberId,omitempty"`
	Category       string            `json:"category"`
	Urgency        string            `json:"urgency"`
	Title          string            `json:"title"`
	Description    string            `json:"description"`
	Recommendation string            `json:"recommendation,omitempty"`
	Confidence     float64           `json:"confidence,omitempty"`
	Deadline       *time.Time        `json:"deadline,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type EscalationCreateResult struct {
	Escalation EscalationView `json:"escalation"`
}

type EscalationGetParams struct {
	EscalationID string `json:"escalationId"`
}

type EscalationGetResult struct {
	Escalation EscalationView `json:"escalation"`
}

type EscalationListParams struct {
	ProjectID string   `json:"projectId"`
	Status    []string `json:"status,omitempty"`
	Urgency   []string `json:"urgency,omitempty"`
	Category  []string `json:"category,omitempty"`
	SpaceID   string   `json:"spaceId,omitempty"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

type EscalationListResult struct {
	Escalations []EscalationView `json:"escalations"`
}

type EscalationListPendingParams struct {
	ProjectID string `json:"projectId"`
}

type EscalationListPendingResult struct {
	Escalations []EscalationView `json:"escalations"`
}

type EscalationResolveParams struct {
	EscalationID   string `json:"escalationId"`
	Resolution     string `json:"resolution"` // "approve" | "reject"
	ResolutionNote string `json:"resolutionNote,omitempty"`
	ResolvedBy     string `json:"resolvedBy"`
}

type EscalationResolveResult struct {
	Escalation EscalationView `json:"escalation"`
}

type EscalationCancelParams struct {
	EscalationID string `json:"escalationId"`
}

type EscalationCancelResult struct {
	Escalation EscalationView `json:"escalation"`
}

type EscalationCountPendingParams struct {
	ProjectID string `json:"projectId"`
}

type EscalationCountPendingResult struct {
	Count int `json:"count"`
}
