package domain

import (
	"context"
	"time"
)

type Filter struct {
	Kind   Kind
	Status Status
	Limit  int
	Offset int
}

type Record struct {
	ID        ID
	Kind      Kind
	Label     string
	Status    Status
	Fields    []FieldRef
	CreatedAt time.Time
	UpdatedAt time.Time
}

type SecretMaterial struct {
	CredentialID ID
	StorageKind  StorageKind
	Payload      []byte
	UpdatedAt    time.Time
}

type ResolvedCredential struct {
	ID      ID
	Kind    Kind
	Purpose Purpose
	Values  map[string]string
}

type Reader interface {
	Get(ctx context.Context, id ID) (Record, error)
	List(ctx context.Context, filter Filter) ([]Record, error)
}

type Writer interface {
	Save(ctx context.Context, record Record) (Record, error)
	Delete(ctx context.Context, id ID) error
}

type MaterialStore interface {
	PutMaterial(ctx context.Context, material SecretMaterial) error
	GetMaterial(ctx context.Context, id ID) (SecretMaterial, error)
	DeleteMaterial(ctx context.Context, id ID) error
}

type Repository interface {
	Reader
	Writer
	MaterialStore
}
