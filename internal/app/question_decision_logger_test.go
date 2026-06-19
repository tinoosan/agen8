package app

import (
	"context"
	"testing"

	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	questionapp "github.com/tinoosan/agen8/internal/services/question/app"
)

func TestQuestionDecisionLoggerMapsQuestionDTOsToDecisionService(t *testing.T) {
	decisions := &recordingDecisionLogService{
		result: decisionapp.Result{ID: "dec-1"},
	}
	logger := questionDecisionLogger{decisions: decisions}

	result, err := logger.LogDecision(context.Background(), questionapp.LogDecisionRequest{
		ProjectID:    "project-1",
		MemberID:     "member-1",
		Title:        "Answer: Ship it",
		Rationale:    "Ship the narrow version.",
		Context:      "Answer recorded for question question-1.",
		Confidence:   1,
		TaskRef:      "task-1",
		KeyResultRef: "kr-1",
		MissionRef:   "mission-1",
	})
	if err != nil {
		t.Fatalf("LogDecision: %v", err)
	}
	if result.ID != "dec-1" {
		t.Fatalf("result ID = %q, want dec-1", result.ID)
	}
	if len(decisions.requests) != 1 {
		t.Fatalf("decision log calls = %d, want 1", len(decisions.requests))
	}
	req := decisions.requests[0]
	if req.ProjectID != "project-1" || req.MemberID != "member-1" || req.TaskRef != "task-1" {
		t.Fatalf("decision request refs = %#v", req)
	}
	if req.Title != "Answer: Ship it" || req.Rationale != "Ship the narrow version." || req.Context == "" {
		t.Fatalf("decision request content = %#v", req)
	}
	if req.KeyResultRef != "kr-1" || req.MissionRef != "mission-1" || req.Confidence != 1 {
		t.Fatalf("decision request lineage = %#v", req)
	}
}

type recordingDecisionLogService struct {
	result   decisionapp.Result
	requests []decisionapp.LogRequest
}

func (s *recordingDecisionLogService) Log(_ context.Context, req decisionapp.LogRequest) (decisionapp.Result, error) {
	s.requests = append(s.requests, req)
	return s.result, nil
}
