package workspace

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("workspace not found")

type Filter struct {
	ProjectID      string
	UserID         string
	LocationID     string
	Root           string
	Machine        string
	LifecycleState string
	Limit          int
	Offset         int
}

type Reader interface {
	Get(ctx context.Context, id string) (Record, error)
	List(ctx context.Context, filter Filter) ([]Record, error)
}

type Writer interface {
	Create(ctx context.Context, ws Record) error
	Update(ctx context.Context, ws Record) error
}

type Repository interface {
	Reader
	Writer
}
