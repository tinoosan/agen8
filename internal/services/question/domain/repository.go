package domain

import (
	"context"
	"errors"
)

var ErrQuestionNotFound = errors.New("question not found")

type Repository interface {
	CreateQuestion(ctx context.Context, question Question) error
	UpdateQuestion(ctx context.Context, question Question) error
	GetQuestion(ctx context.Context, id QuestionID) (Question, error)
}
