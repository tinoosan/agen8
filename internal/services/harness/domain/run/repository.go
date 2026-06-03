package run

import "context"

type Repository interface {
	Reader
	Writer
}

type Reader interface {
	Get(ctx context.Context, id string) (*Run, error)
	GetActiveBySession(ctx context.Context, sessionID string) (*Run, error)
	GetByTurnID(ctx context.Context, turnID string) (*Run, error)
	List(ctx context.Context, filter Filter) ([]Run, error)
}

type Writer interface {
	Save(ctx context.Context, run Run) error
	MarkRuntimeLost(ctx context.Context) ([]Run, error)
}

type Filter struct {
	ProjectID string
	SpaceID   string
	ChannelID string
	MemberID  string
	SessionID string
	Status    []Status
	Limit     int
}
