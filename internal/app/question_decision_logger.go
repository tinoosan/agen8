package app

import (
	"context"

	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	questionapp "github.com/tinoosan/agen8/internal/services/question/app"
)

type decisionLogService interface {
	Log(ctx context.Context, req decisionapp.LogRequest) (decisionapp.Result, error)
}

type questionDecisionLogger struct {
	decisions decisionLogService
}

func newQuestionDecisionLogger(decisions *decisionapp.Service) questionDecisionLogger {
	return questionDecisionLogger{decisions: decisions}
}

func (l questionDecisionLogger) LogDecision(ctx context.Context, req questionapp.LogDecisionRequest) (questionapp.DecisionLogResult, error) {
	decision, err := l.decisions.Log(ctx, decisionapp.LogRequest{
		ProjectID:    req.ProjectID,
		MemberID:     req.MemberID,
		Title:        req.Title,
		Rationale:    req.Rationale,
		Context:      req.Context,
		Confidence:   req.Confidence,
		TaskRef:      req.TaskRef,
		KeyResultRef: req.KeyResultRef,
		MissionRef:   req.MissionRef,
	})
	if err != nil {
		return questionapp.DecisionLogResult{}, err
	}
	return questionapp.DecisionLogResult{ID: decision.ID}, nil
}
