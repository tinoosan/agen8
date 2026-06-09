package infra

import (
	"context"
	"fmt"

	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type SQLiteRepository struct {
	*sqlStore
}

func NewSQLiteRepository(handle *storagedb.Handle) (*SQLiteRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("location sqlite repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("location sqlite repository: storage driver must be sqlite, got %q", handle.Driver())
	}
	store := newSQLStore(handle)
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return &SQLiteRepository{sqlStore: store}, nil
}
