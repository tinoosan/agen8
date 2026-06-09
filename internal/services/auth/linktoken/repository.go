package linktoken

import "context"

type Repository interface {
	Reader
	Writer
}

// Filter narrows a List. An empty filter lists everything; ProjectID is the
// field the project service uses to fetch the tokens it owns. Listing returns
// every matching token regardless of state — revoked and expired tokens are
// still part of a project's link history and the caller decides how to present
// them.
type Filter struct {
	ProjectID string
	UserID    string
	Limit     int
	Offset    int
}

type Reader interface {
	GetByTokenHash(ctx context.Context, tokenHash string) (LinkToken, error)
	Get(ctx context.Context, id ID) (LinkToken, error)
	List(ctx context.Context, filter Filter) ([]LinkToken, error)
}

type Writer interface {
	Create(ctx context.Context, token LinkToken) error
	Update(ctx context.Context, token LinkToken) error
}
