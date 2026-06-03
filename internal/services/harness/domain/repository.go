package domain

import "context"

type SessionRepository interface {
	SessionReader
	SessionWriter
}

type SessionReader interface {
	Get(ctx context.Context, id string) (*Session, error)
	GetActiveByMember(ctx context.Context, memberID string) (*Session, error)
	ListActive(ctx context.Context) ([]*Session, error)
	ListByMember(ctx context.Context, memberID string) ([]*Session, error)
	ListBySpace(ctx context.Context, spaceID string) ([]*Session, error)
}

type SessionWriter interface {
	Save(ctx context.Context, session *Session) error
}
