package domain

import (
	"context"
)

type SpaceFilter struct {
	SpaceID   string
	ProjectID string
	UserID    string
	Status    string
	Limit     int
	Offset    int
}

type Reader interface {
	Get(ctx context.Context, id SpaceID) (SpaceRecord, error)
	List(ctx context.Context, filter SpaceFilter) ([]SpaceRecord, error)
}

type Writer interface {
	Create(ctx context.Context, space SpaceRecord) error
	Update(ctx context.Context, space SpaceRecord) error
	Delete(ctx context.Context, id SpaceID) error
}

type Repository interface {
	Reader
	Writer
}
