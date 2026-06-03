package infra

import (
	"context"
	"fmt"

	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type PostgresRepository struct {
	*sqlStore
}

func NewPostgresRepository(handle *storagedb.Handle) (*PostgresRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("message postgres repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("message postgres repository: storage driver must be postgres, got %q", handle.Driver())
	}
	repo := &PostgresRepository{sqlStore: newSQLStore(handle)}
	if err := repo.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}
