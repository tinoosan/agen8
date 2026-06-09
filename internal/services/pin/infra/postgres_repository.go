package infra

import (
	"context"
	"fmt"

	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type PostgresRepository struct {
	*sqlStore
}

func NewPostgresRepository(handle *storagedb.Handle) (*PostgresRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("pin postgres repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("pin postgres repository: storage driver must be postgres, got %q", handle.Driver())
	}
	store := newSQLStore(handle)
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return &PostgresRepository{sqlStore: store}, nil
}
