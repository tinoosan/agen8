package decision

import (
	"context"

	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	decisiondomain "github.com/tinoosan/agen8/internal/services/decision/domain"
)

type Service interface {
	Log(context.Context, decisionapp.LogRequest) (decisionapp.Result, error)
	Delete(context.Context, decisiondomain.DecisionID) error
}

type CallContext struct {
	Decisions     Service
	ProjectID     string
	ActorMemberID string
	UserID        string
}

type Result struct {
	Text       string
	Structured any
}

type rawRequest struct {
	Action                 string   `json:"action"`
	DecisionID             *string  `json:"decision_id"`
	Title                  *string  `json:"title"`
	Rationale              *string  `json:"rationale"`
	Context                *string  `json:"context"`
	AlternativesRejected   *string  `json:"alternatives_rejected"`
	InvalidationConditions []string `json:"invalidation_conditions"`
	Confidence             *float64 `json:"confidence"`
	TaskRef                *string  `json:"task_ref"`
	KeyResultRef           *string  `json:"key_result_ref"`
	MissionRef             *string  `json:"mission_ref"`
}

type requestInput struct {
	Action                 string
	DecisionID             string
	Title                  string
	Rationale              string
	Context                string
	AlternativesRejected   string
	InvalidationConditions []string
	Confidence             float64
	TaskRef                string
	KeyResultRef           string
	MissionRef             string
}
