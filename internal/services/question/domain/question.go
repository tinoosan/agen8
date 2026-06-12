package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type QuestionID string

type AnswerKind string

const (
	AnswerKindFreeText     AnswerKind = "free_text"
	AnswerKindSingleSelect AnswerKind = "single_select"
	AnswerKindMultiSelect  AnswerKind = "multi_select"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusAnswered  Status = "answered"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)

type NewQuestionInput struct {
	ID              QuestionID
	ProjectID       string
	AskedByMemberID string
	Text            string
	AnswerKind      AnswerKind
	Options         []string
	AsDecision      bool
	TaskRef         string
	KeyResultRef    string
	MissionRef      string
}

type AnswerPayload struct {
	Text               string    `json:"text,omitempty"`
	SelectedOptions    []string  `json:"selectedOptions,omitempty"`
	AnsweredByMemberID string    `json:"answeredByMemberId,omitempty"`
	AnsweredAt         time.Time `json:"answeredAt,omitempty"`
}

type Question struct {
	ID              QuestionID    `json:"id"`
	ProjectID       string        `json:"projectId"`
	AskedByMemberID string        `json:"askedByMemberId"`
	Text            string        `json:"text"`
	AnswerKind      AnswerKind    `json:"answerKind"`
	Options         []string      `json:"options,omitempty"`
	AsDecision      bool          `json:"asDecision"`
	TaskRef         string        `json:"taskRef,omitempty"`
	KeyResultRef    string        `json:"keyResultRef,omitempty"`
	MissionRef      string        `json:"missionRef,omitempty"`
	Status          Status        `json:"status"`
	Answer          AnswerPayload `json:"answer,omitempty"`
	DecisionID      string        `json:"decisionId,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
	AnsweredAt      time.Time     `json:"answeredAt,omitempty"`
	ExpiredAt       time.Time     `json:"expiredAt,omitempty"`
	CancelledAt     time.Time     `json:"cancelledAt,omitempty"`
}

func NewQuestion(input NewQuestionInput, now time.Time) (Question, error) {
	if input.AnswerKind == "" {
		input.AnswerKind = AnswerKindFreeText
	}
	q := Question{
		ID:              QuestionID(strings.TrimSpace(string(input.ID))),
		ProjectID:       strings.TrimSpace(input.ProjectID),
		AskedByMemberID: strings.TrimSpace(input.AskedByMemberID),
		Text:            strings.TrimSpace(input.Text),
		AnswerKind:      AnswerKind(strings.TrimSpace(string(input.AnswerKind))),
		Options:         normalizeStrings(input.Options),
		AsDecision:      input.AsDecision,
		TaskRef:         strings.TrimSpace(input.TaskRef),
		KeyResultRef:    strings.TrimSpace(input.KeyResultRef),
		MissionRef:      strings.TrimSpace(input.MissionRef),
		Status:          StatusPending,
		CreatedAt:       now.UTC(),
		UpdatedAt:       now.UTC(),
	}
	if err := q.Validate(); err != nil {
		return Question{}, err
	}
	return q, nil
}

func (q Question) Validate() error {
	switch {
	case strings.TrimSpace(string(q.ID)) == "":
		return errors.New("question id is required")
	case strings.TrimSpace(q.ProjectID) == "":
		return errors.New("question project id is required")
	case strings.TrimSpace(q.AskedByMemberID) == "":
		return errors.New("question askedByMemberId is required")
	case strings.TrimSpace(q.Text) == "":
		return errors.New("question text is required")
	case q.CreatedAt.IsZero():
		return errors.New("question createdAt is required")
	case q.UpdatedAt.IsZero():
		return errors.New("question updatedAt is required")
	}
	if err := validateAnswerKind(q.AnswerKind, q.Options); err != nil {
		return err
	}
	switch q.Status {
	case StatusPending:
		if !q.AnsweredAt.IsZero() || q.DecisionID != "" {
			return errors.New("pending question cannot have answer state")
		}
	case StatusAnswered:
		if q.AnsweredAt.IsZero() {
			return errors.New("answered question requires answeredAt")
		}
		if err := validateAnswer(q.AnswerKind, q.Options, q.Answer); err != nil {
			return err
		}
	case StatusExpired:
		if q.ExpiredAt.IsZero() {
			return errors.New("expired question requires expiredAt")
		}
	case StatusCancelled:
		if q.CancelledAt.IsZero() {
			return errors.New("cancelled question requires cancelledAt")
		}
	default:
		return fmt.Errorf("unsupported question status %q", q.Status)
	}
	return nil
}

func (q Question) AnswerWith(answer AnswerPayload, now time.Time) (Question, error) {
	if q.Status != StatusPending {
		return Question{}, fmt.Errorf("question must be pending to answer, got %q", q.Status)
	}
	answer.Text = strings.TrimSpace(answer.Text)
	answer.AnsweredByMemberID = strings.TrimSpace(answer.AnsweredByMemberID)
	answer.SelectedOptions = normalizeStrings(answer.SelectedOptions)
	answer.AnsweredAt = now.UTC()
	if answer.AnsweredByMemberID == "" {
		return Question{}, errors.New("answer answeredByMemberId is required")
	}
	if err := validateAnswer(q.AnswerKind, q.Options, answer); err != nil {
		return Question{}, err
	}
	q.Status = StatusAnswered
	q.Answer = answer
	q.AnsweredAt = now.UTC()
	q.UpdatedAt = now.UTC()
	if err := q.Validate(); err != nil {
		return Question{}, err
	}
	return q, nil
}

func (q Question) WithDecisionID(decisionID string, now time.Time) (Question, error) {
	if q.Status != StatusAnswered {
		return Question{}, fmt.Errorf("question must be answered before linking decision, got %q", q.Status)
	}
	q.DecisionID = strings.TrimSpace(decisionID)
	q.UpdatedAt = now.UTC()
	if q.DecisionID == "" {
		return Question{}, errors.New("decision id is required")
	}
	if err := q.Validate(); err != nil {
		return Question{}, err
	}
	return q, nil
}

func (q Question) Expire(now time.Time) (Question, error) {
	if q.Status != StatusPending {
		return Question{}, fmt.Errorf("question must be pending to expire, got %q", q.Status)
	}
	q.Status = StatusExpired
	q.ExpiredAt = now.UTC()
	q.UpdatedAt = now.UTC()
	if err := q.Validate(); err != nil {
		return Question{}, err
	}
	return q, nil
}

func (q Question) Cancel(now time.Time) (Question, error) {
	if q.Status != StatusPending {
		return Question{}, fmt.Errorf("question must be pending to cancel, got %q", q.Status)
	}
	q.Status = StatusCancelled
	q.CancelledAt = now.UTC()
	q.UpdatedAt = now.UTC()
	if err := q.Validate(); err != nil {
		return Question{}, err
	}
	return q, nil
}

func validateAnswerKind(kind AnswerKind, options []string) error {
	switch kind {
	case AnswerKindFreeText:
		return nil
	case AnswerKindSingleSelect, AnswerKindMultiSelect:
		if len(options) == 0 {
			return fmt.Errorf("%s question requires options", kind)
		}
		return nil
	default:
		return fmt.Errorf("unsupported answer kind %q", kind)
	}
}

func validateAnswer(kind AnswerKind, options []string, answer AnswerPayload) error {
	switch kind {
	case AnswerKindFreeText:
		if strings.TrimSpace(answer.Text) == "" {
			return errors.New("free_text answer requires text")
		}
		return nil
	case AnswerKindSingleSelect:
		if len(answer.SelectedOptions) != 1 {
			return errors.New("single_select answer requires exactly one selected option")
		}
	case AnswerKindMultiSelect:
		if len(answer.SelectedOptions) == 0 {
			return errors.New("multi_select answer requires at least one selected option")
		}
	default:
		return fmt.Errorf("unsupported answer kind %q", kind)
	}
	allowed := make(map[string]struct{}, len(options))
	for _, option := range options {
		allowed[option] = struct{}{}
	}
	for _, selected := range answer.SelectedOptions {
		if _, ok := allowed[selected]; !ok {
			return fmt.Errorf("selected option %q is not available", selected)
		}
	}
	return nil
}

func normalizeStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
