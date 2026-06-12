package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8/internal/eventbus"
	decisionapp "github.com/tinoosan/agen8/internal/services/decision/app"
	"github.com/tinoosan/agen8/internal/services/question/domain"
)

func TestServiceCreatePublishesQuestionOpened(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)}
	repo := newRecordingQuestionRepo()
	events := &recordingEvents{}
	svc, err := NewService(Config{Questions: repo, Clock: clock, Events: events})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	result, err := svc.Create(context.Background(), CreateRequest{
		ProjectID:       "project-1",
		AskedByMemberID: "member-agent",
		Text:            "What should we do?",
		AnswerKind:      domain.AnswerKindFreeText,
		AsDecision:      true,
		TaskRef:         "task-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if result.Question.Status != domain.StatusPending {
		t.Fatalf("status = %q, want pending", result.Question.Status)
	}
	if len(events.records) != 1 {
		t.Fatalf("events = %d, want 1", len(events.records))
	}
	if events.records[0].topic != eventbus.TopicQuestionLifecycle {
		t.Fatalf("topic = %q", events.records[0].topic)
	}
	opened, ok := events.records[0].event.(eventbus.QuestionOpenedEvent)
	if !ok {
		t.Fatalf("event = %T, want QuestionOpenedEvent", events.records[0].event)
	}
	if opened.EventType != eventbus.QuestionEventOpened || opened.QuestionID != string(result.Question.ID) || !opened.AsDecision {
		t.Fatalf("opened event = %#v", opened)
	}
}

func TestServiceAnswerPublishesQuestionAnswered(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)}
	repo := newRecordingQuestionRepo()
	events := &recordingEvents{}
	svc, err := NewService(Config{Questions: repo, Clock: clock, Events: events})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	question := mustQuestion(t, domain.NewQuestionInput{
		ID:              "question-1",
		ProjectID:       "project-1",
		AskedByMemberID: "member-agent",
		Text:            "What should we do?",
		AnswerKind:      domain.AnswerKindFreeText,
	}, clock.now)
	if err := repo.CreateQuestion(context.Background(), question); err != nil {
		t.Fatalf("CreateQuestion: %v", err)
	}

	result, err := svc.Answer(context.Background(), AnswerRequest{
		QuestionID:         "question-1",
		AnsweredByMemberID: "member-human",
		Text:               "Ship the narrower version.",
	})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if result.Question.Status != domain.StatusAnswered {
		t.Fatalf("status = %q, want answered", result.Question.Status)
	}
	if len(events.records) != 1 {
		t.Fatalf("events = %d, want 1", len(events.records))
	}
	answered, ok := events.records[0].event.(eventbus.QuestionAnsweredEvent)
	if !ok {
		t.Fatalf("event = %T, want QuestionAnsweredEvent", events.records[0].event)
	}
	if answered.EventType != eventbus.QuestionEventAnswered || answered.AnsweredByMemberID != "member-human" {
		t.Fatalf("answered event = %#v", answered)
	}
}

func TestServiceAnswerAsDecisionLogsAndLinksDecision(t *testing.T) {
	clock := fixedClock{now: time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)}
	repo := newRecordingQuestionRepo()
	events := &recordingEvents{}
	decisions := &recordingDecisionLogger{result: decisionapp.Result{ID: "dec-1"}}
	svc, err := NewService(Config{Questions: repo, Clock: clock, Events: events, Decisions: decisions})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	question := mustQuestion(t, domain.NewQuestionInput{
		ID:              "question-1",
		ProjectID:       "project-1",
		AskedByMemberID: "member-agent",
		Text:            "Should this become a decision?",
		AnswerKind:      domain.AnswerKindSingleSelect,
		Options:         []string{"yes", "no"},
		AsDecision:      true,
		TaskRef:         "task-1",
		MissionRef:      "mission-1",
	}, clock.now)
	if err := repo.CreateQuestion(context.Background(), question); err != nil {
		t.Fatalf("CreateQuestion: %v", err)
	}

	result, err := svc.Answer(context.Background(), AnswerRequest{
		QuestionID:         "question-1",
		AnsweredByMemberID: "member-human",
		SelectedOptions:    []string{"yes"},
	})
	if err != nil {
		t.Fatalf("Answer: %v", err)
	}
	if len(decisions.requests) != 1 {
		t.Fatalf("decision log calls = %d, want 1", len(decisions.requests))
	}
	req := decisions.requests[0]
	if req.ProjectID != "project-1" || req.MemberID != "member-human" || req.TaskRef != "task-1" || req.MissionRef != "mission-1" {
		t.Fatalf("decision request = %#v", req)
	}
	if !strings.Contains(req.Context, "question-1") {
		t.Fatalf("decision context = %q, want question id", req.Context)
	}
	if result.Question.DecisionID != "dec-1" {
		t.Fatalf("DecisionID = %q, want dec-1", result.Question.DecisionID)
	}
	answered := events.records[0].event.(eventbus.QuestionAnsweredEvent)
	if answered.DecisionID != "dec-1" {
		t.Fatalf("event DecisionID = %q, want dec-1", answered.DecisionID)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type recordingQuestionRepo struct {
	questions map[domain.QuestionID]domain.Question
}

func newRecordingQuestionRepo() *recordingQuestionRepo {
	return &recordingQuestionRepo{questions: map[domain.QuestionID]domain.Question{}}
}

func (r *recordingQuestionRepo) CreateQuestion(_ context.Context, question domain.Question) error {
	r.questions[question.ID] = question
	return nil
}

func (r *recordingQuestionRepo) UpdateQuestion(_ context.Context, question domain.Question) error {
	if _, ok := r.questions[question.ID]; !ok {
		return domain.ErrQuestionNotFound
	}
	r.questions[question.ID] = question
	return nil
}

func (r *recordingQuestionRepo) GetQuestion(_ context.Context, id domain.QuestionID) (domain.Question, error) {
	question, ok := r.questions[id]
	if !ok {
		return domain.Question{}, domain.ErrQuestionNotFound
	}
	return question, nil
}

type recordingEvents struct {
	records []eventRecord
}

type eventRecord struct {
	topic string
	event any
}

func (e *recordingEvents) Publish(topic string, event any) error {
	e.records = append(e.records, eventRecord{topic: topic, event: event})
	return nil
}

type recordingDecisionLogger struct {
	result   decisionapp.Result
	requests []decisionapp.LogRequest
}

func (l *recordingDecisionLogger) Log(_ context.Context, req decisionapp.LogRequest) (decisionapp.Result, error) {
	l.requests = append(l.requests, req)
	return l.result, nil
}

func mustQuestion(t *testing.T, input domain.NewQuestionInput, now time.Time) domain.Question {
	t.Helper()
	question, err := domain.NewQuestion(input, now)
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	return question
}
