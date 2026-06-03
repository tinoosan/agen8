package domain

import (
	"context"
	"time"
)

type Reader interface {
	Get(ctx context.Context, id RequestID) (Request, error)
	FindByIdempotency(ctx context.Context, projectID, toolName string, toolCallID ToolCallID, idempotencyKey string) (Request, error)
	ListPending(ctx context.Context, filter Filter) ([]Request, error)
}

type Writer interface {
	CreatePending(ctx context.Context, req Request) (Request, error)
	Resolve(ctx context.Context, mutation ResolveMutation) (Request, error)
	ExpireDue(ctx context.Context, now time.Time, limit int) (ExpireBatch, error)
	AbortByToolCall(ctx context.Context, toolCallID ToolCallID, reason string) (Request, error)
}

type Repository interface {
	Reader
	Writer
}
