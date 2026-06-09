package apikey

import "context"

import user "github.com/tinoosan/agen8/internal/services/user/domain"

type Repository interface {
	Reader
	Writer
}

type Reader interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (Key, error)
	Get(ctx context.Context, id ID) (Key, error)
	ListByUser(ctx context.Context, userID user.ID) ([]Key, error)
}

type Writer interface {
	Create(ctx context.Context, key Key) error
	Update(ctx context.Context, key Key) error
}
