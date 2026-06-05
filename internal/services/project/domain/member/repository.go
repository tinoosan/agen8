package member

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("member not found")

type Filter struct {
	ProjectID        string
	UserID           string
	HarnessKind      string
	NativeSessionRef string
	MemberType       string
	LifecycleState   string
	Limit            int
	Offset           int
}

type Reader interface {
	Get(ctx context.Context, id string) (Record, error)
	List(ctx context.Context, filter Filter) ([]Record, error)
}

type Writer interface {
	Create(ctx context.Context, member Record) error
	Update(ctx context.Context, member Record) error
}

type Repository interface {
	Reader
	Writer
}
