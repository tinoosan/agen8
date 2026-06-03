package user

import "context"

type Repository interface {
	Reader
	Writer
}

type Reader interface {
	Get(ctx context.Context, id ID) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
	FirstActive(ctx context.Context) (User, error)
	Count(ctx context.Context) (int, error)
}

type Writer interface {
	Create(ctx context.Context, record User) error
	Update(ctx context.Context, record User) error
}
