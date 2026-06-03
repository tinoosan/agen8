package domain

import (
	"context"
	"time"
)

type Filter struct {
	Kind   Kind
	Status Status
	Ready  *bool
	Limit  int
	Offset int
}

type Record struct {
	ID             ID
	Kind           Kind
	Label          string
	Address        Address
	Status         Status
	Ready          bool
	CredentialRef  string
	Probe          Probe
	LastProbeError string
	LastProbedAt   *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Reader interface {
	Get(ctx context.Context, id ID) (Record, error)
	List(ctx context.Context, filter Filter) ([]Record, error)
}

type Writer interface {
	Save(ctx context.Context, location Record) (Record, error)
	Delete(ctx context.Context, id ID) error
}

type Repository interface {
	Reader
	Writer
}
