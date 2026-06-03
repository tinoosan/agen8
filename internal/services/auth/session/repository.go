package session

import "context"

type Repository interface {
	Reader
	Writer
}

type Reader interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (Session, error)
}

type Writer interface {
	Create(ctx context.Context, session Session) error
	Update(ctx context.Context, session Session) error
}
