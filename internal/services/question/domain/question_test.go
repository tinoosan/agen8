package domain

import (
	"testing"
	"time"
)

func TestQuestionLifecycle(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	question, err := NewQuestion(NewQuestionInput{
		ID:              "question-1",
		ProjectID:       "project-1",
		AskedByMemberID: "member-agent",
		Text:            "Which path should we take?",
		AnswerKind:      AnswerKindSingleSelect,
		Options:         []string{"ship", "hold"},
	}, now)
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	if question.Status != StatusPending {
		t.Fatalf("status = %q, want pending", question.Status)
	}

	answered, err := question.AnswerWith(AnswerPayload{
		SelectedOptions:    []string{"ship"},
		AnsweredByMemberID: "member-human",
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("AnswerWith: %v", err)
	}
	if answered.Status != StatusAnswered {
		t.Fatalf("answered status = %q, want answered", answered.Status)
	}
	if _, err := answered.Expire(now.Add(2 * time.Minute)); err == nil {
		t.Fatal("Expire answered question returned nil error")
	}

	expired, err := question.Expire(now.Add(3 * time.Minute))
	if err != nil {
		t.Fatalf("Expire: %v", err)
	}
	if expired.Status != StatusExpired || expired.ExpiredAt.IsZero() {
		t.Fatalf("expired question = %#v", expired)
	}

	cancelled, err := question.Cancel(now.Add(4 * time.Minute))
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if cancelled.Status != StatusCancelled || cancelled.CancelledAt.IsZero() {
		t.Fatalf("cancelled question = %#v", cancelled)
	}
}

func TestQuestionSelectionValidation(t *testing.T) {
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	question, err := NewQuestion(NewQuestionInput{
		ID:              "question-1",
		ProjectID:       "project-1",
		AskedByMemberID: "member-agent",
		Text:            "Pick one.",
		AnswerKind:      AnswerKindSingleSelect,
		Options:         []string{"alpha", "beta"},
	}, now)
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	if _, err := question.AnswerWith(AnswerPayload{
		SelectedOptions:    []string{"gamma"},
		AnsweredByMemberID: "member-human",
	}, now.Add(time.Minute)); err == nil {
		t.Fatal("AnswerWith invalid selected option returned nil error")
	}
	if _, err := NewQuestion(NewQuestionInput{
		ID:              "question-2",
		ProjectID:       "project-1",
		AskedByMemberID: "member-agent",
		Text:            "Pick any.",
		AnswerKind:      AnswerKindMultiSelect,
	}, now); err == nil {
		t.Fatal("NewQuestion multi_select without options returned nil error")
	}
}
