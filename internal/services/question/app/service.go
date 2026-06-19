package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/internal/eventbus"
	"github.com/tinoosan/agen8/internal/services/question/domain"
)

type Service struct {
	repo      domain.Repository
	clock     domain.Clock
	events    EventPublisher
	decisions DecisionLogger
	logger    *slog.Logger
}

type EventPublisher interface {
	Publish(topic string, event any) error
}

type DecisionLogger interface {
	LogDecision(ctx context.Context, req LogDecisionRequest) (DecisionLogResult, error)
}

type LogDecisionRequest struct {
	ProjectID    string
	MemberID     string
	Title        string
	Rationale    string
	Context      string
	Confidence   float64
	TaskRef      string
	KeyResultRef string
	MissionRef   string
}

type DecisionLogResult struct {
	ID string
}

type Config struct {
	Questions domain.Repository
	Clock     domain.Clock
	Events    EventPublisher
	Decisions DecisionLogger
	Logger    *slog.Logger
}

type CreateRequest struct {
	ProjectID       string
	AskedByMemberID string
	Text            string
	AnswerKind      domain.AnswerKind
	Options         []string
	AsDecision      bool
	TaskRef         string
	KeyResultRef    string
	MissionRef      string
}

type AnswerRequest struct {
	QuestionID         string
	AnsweredByMemberID string
	Text               string
	SelectedOptions    []string
}

type Result struct {
	Question domain.Question
}

func NewService(cfg Config) (*Service, error) {
	switch {
	case cfg.Questions == nil:
		return nil, errors.New("question service: repository is required")
	case cfg.Clock == nil:
		return nil, errors.New("question service: clock is required")
	case cfg.Events == nil:
		return nil, errors.New("question service: event publisher is required")
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default().With("service", "question")
	}
	return &Service{
		repo:      cfg.Questions,
		clock:     cfg.Clock,
		events:    cfg.Events,
		decisions: cfg.Decisions,
		logger:    logger,
	}, nil
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (Result, error) {
	now := s.clock.Now().UTC()
	question, err := domain.NewQuestion(domain.NewQuestionInput{
		ID:              domain.QuestionID("question-" + uuid.NewString()),
		ProjectID:       req.ProjectID,
		AskedByMemberID: req.AskedByMemberID,
		Text:            req.Text,
		AnswerKind:      req.AnswerKind,
		Options:         req.Options,
		AsDecision:      req.AsDecision,
		TaskRef:         req.TaskRef,
		KeyResultRef:    req.KeyResultRef,
		MissionRef:      req.MissionRef,
	}, now)
	if err != nil {
		return Result{}, err
	}
	if err := s.repo.CreateQuestion(ctx, question); err != nil {
		return Result{}, fmt.Errorf("create question: %w", err)
	}
	if err := s.events.Publish(eventbus.TopicQuestionLifecycle, eventbus.QuestionOpenedEvent{
		ProjectID:       question.ProjectID,
		QuestionID:      string(question.ID),
		AskedByMemberID: question.AskedByMemberID,
		AnswerKind:      string(question.AnswerKind),
		AsDecision:      question.AsDecision,
		TaskRef:         question.TaskRef,
		KeyResultRef:    question.KeyResultRef,
		MissionRef:      question.MissionRef,
		EventType:       eventbus.QuestionEventOpened,
		Timestamp:       question.CreatedAt,
	}); err != nil {
		return Result{}, fmt.Errorf("publish question opened event: %w", err)
	}
	return Result{Question: question}, nil
}

func (s *Service) Answer(ctx context.Context, req AnswerRequest) (Result, error) {
	questionID := strings.TrimSpace(req.QuestionID)
	if questionID == "" {
		return Result{}, errors.New("question id is required")
	}
	question, err := s.repo.GetQuestion(ctx, domain.QuestionID(questionID))
	if err != nil {
		return Result{}, fmt.Errorf("get question: %w", err)
	}
	now := s.clock.Now().UTC()
	answered, err := question.AnswerWith(domain.AnswerPayload{
		Text:               req.Text,
		SelectedOptions:    req.SelectedOptions,
		AnsweredByMemberID: req.AnsweredByMemberID,
	}, now)
	if err != nil {
		return Result{}, err
	}
	if answered.AsDecision {
		if s.decisions == nil {
			return Result{}, errors.New("question service: decision logger is required for asDecision answer")
		}
		decision, err := s.decisions.LogDecision(ctx, LogDecisionRequest{
			ProjectID:    answered.ProjectID,
			MemberID:     answered.Answer.AnsweredByMemberID,
			Title:        decisionTitle(answered.Text),
			Rationale:    answerSummary(answered),
			Context:      fmt.Sprintf("Answer recorded for question %s.", answered.ID),
			Confidence:   1,
			TaskRef:      answered.TaskRef,
			KeyResultRef: answered.KeyResultRef,
			MissionRef:   answered.MissionRef,
		})
		if err != nil {
			return Result{}, fmt.Errorf("log question answer decision: %w", err)
		}
		answered, err = answered.WithDecisionID(decision.ID, s.clock.Now().UTC())
		if err != nil {
			return Result{}, err
		}
	}
	if err := s.repo.UpdateQuestion(ctx, answered); err != nil {
		return Result{}, fmt.Errorf("update question: %w", err)
	}
	if err := s.events.Publish(eventbus.TopicQuestionLifecycle, eventbus.QuestionAnsweredEvent{
		ProjectID:          answered.ProjectID,
		QuestionID:         string(answered.ID),
		AskedByMemberID:    answered.AskedByMemberID,
		AnsweredByMemberID: answered.Answer.AnsweredByMemberID,
		DecisionID:         answered.DecisionID,
		TaskRef:            answered.TaskRef,
		KeyResultRef:       answered.KeyResultRef,
		MissionRef:         answered.MissionRef,
		EventType:          eventbus.QuestionEventAnswered,
		Timestamp:          answered.AnsweredAt,
	}); err != nil {
		return Result{}, fmt.Errorf("publish question answered event: %w", err)
	}
	return Result{Question: answered}, nil
}

func (s *Service) Get(ctx context.Context, id domain.QuestionID) (domain.Question, error) {
	return s.repo.GetQuestion(ctx, id)
}

func decisionTitle(questionText string) string {
	const prefix = "Answer: "
	text := strings.TrimSpace(questionText)
	if len(text) > 72 {
		text = text[:72]
	}
	if text == "" {
		return prefix + "question"
	}
	return prefix + text
}

func answerSummary(question domain.Question) string {
	switch question.AnswerKind {
	case domain.AnswerKindFreeText:
		return strings.TrimSpace(question.Answer.Text)
	case domain.AnswerKindSingleSelect, domain.AnswerKindMultiSelect:
		return strings.Join(question.Answer.SelectedOptions, ", ")
	default:
		return strings.TrimSpace(question.Answer.Text)
	}
}
