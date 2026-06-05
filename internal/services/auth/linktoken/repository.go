package linktoken

import "context"

type Repository interface {
	Reader
	Writer
}

type Reader interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (LinkToken, error)
	Get(ctx context.Context, id ID) (LinkToken, error)
}

type Writer interface {
	Create(ctx context.Context, token LinkToken) error
	Update(ctx context.Context, token LinkToken) error
}
