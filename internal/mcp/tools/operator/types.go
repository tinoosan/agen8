package operator

import (
	"context"
	"encoding/json"

	operatorapp "github.com/tinoosan/agen8-mcp-server/internal/services/operator/app"
	operatordomain "github.com/tinoosan/agen8-mcp-server/internal/services/operator/domain"
)

type Service interface {
	Create(context.Context, operatordomain.CreateParams) (operatordomain.OperatorAction, error)
	CreateEscalation(context.Context, operatorapp.CreateEscalationParams) (operatordomain.Escalation, error)
}

type CallContext struct {
	Operator      Service
	ProjectID     string
	SpaceID       string
	ActorMemberID string
}

type Result struct {
	Text       string
	Structured any
}

type rawRequest struct {
	Action               string          `json:"action"`
	Title                *string         `json:"title"`
	Description          *string         `json:"description"`
	Recommendation       *string         `json:"recommendation"`
	Category             *string         `json:"category"`
	Urgency              *string         `json:"urgency"`
	Confidence           *float64        `json:"confidence"`
	TaskRef              *string         `json:"task_ref"`
	KeyResultRef         *string         `json:"key_result_ref"`
	MissionRef           *string         `json:"mission_ref"`
	RunID                *string         `json:"run_id"`
	Blocking             *bool           `json:"blocking"`
	RequiresVerification *bool           `json:"requires_verification"`
	DeadlineHours        *int            `json:"deadline_hours"`
	Metadata             json.RawMessage `json:"metadata"`
}

type requestInput struct {
	Action               string
	Title                string
	Description          string
	Recommendation       string
	Category             string
	Urgency              string
	Confidence           float64
	TaskRef              string
	KeyResultRef         string
	MissionRef           string
	RunID                string
	Blocking             bool
	RequiresVerification bool
	DeadlineHours        int
	Metadata             map[string]string
}
