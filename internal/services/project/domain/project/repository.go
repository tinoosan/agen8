package project

import (
	"context"
	"errors"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
)

var ErrRootInUse = errors.New("project root is already registered")

type Record struct {
	ID            types.ProjectID
	LocationID    types.LocationID
	Root          string
	UserID        string
	Title         string
	Status        Status
	Customization *Customization
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Filter struct {
	Status Status
	Limit  int
	Offset int
}

type Reader interface {
	Get(ctx context.Context, id types.ProjectID) (Record, error)
	List(ctx context.Context, filter Filter) ([]Record, error)
}

type Writer interface {
	Save(ctx context.Context, project Record) (Record, error)
	Delete(ctx context.Context, id types.ProjectID) error
}

type Repository interface {
	Reader
	Writer
}
