package decision

import (
	"context"

	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	humaninput "github.com/tinoosan/agen8-mcp-server/internal/services/humaninput/domain"
)

type Service interface {
	Log(context.Context, decisionapp.LogRequest) (decisionapp.Result, error)
	CompleteAskUser(context.Context, decisionapp.AskUserRequest, humaninput.QuestionsResult) (decisionapp.Result, error)
}

type CallContext struct {
	Decisions     Service
	ProjectID     string
	SpaceID       string
	ActorMemberID string
}

type Result struct {
	Text       string
	Structured any
}

type rawRequest struct {
	Action                 string                `json:"action"`
	Title                  *string               `json:"title"`
	Rationale              *string               `json:"rationale"`
	Context                *string               `json:"context"`
	AlternativesRejected   *string               `json:"alternatives_rejected"`
	InvalidationConditions []string              `json:"invalidation_conditions"`
	Confidence             *float64              `json:"confidence"`
	TaskRef                *string               `json:"task_ref"`
	KeyResultRef           *string               `json:"key_result_ref"`
	MissionRef             *string               `json:"mission_ref"`
	PlanRef                *string               `json:"plan_ref"`
	Questions              []humaninput.Question `json:"questions"`
	Answers                []humaninput.Answer   `json:"answers"`
	Cancelled              *bool                 `json:"cancelled"`
}

type requestInput struct {
	Action                 string
	Title                  string
	Rationale              string
	Context                string
	AlternativesRejected   string
	InvalidationConditions []string
	Confidence             float64
	TaskRef                string
	KeyResultRef           string
	MissionRef             string
	PlanRef                string
	Questions              []humaninput.Question
	Answers                []humaninput.Answer
	Cancelled              bool
}
