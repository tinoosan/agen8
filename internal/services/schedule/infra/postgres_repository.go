package infra

import (
	"context"
	"fmt"

	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type PostgresRepository struct {
	*SQLRepository
}

func NewPostgresRepository(handle *storagedb.Handle) (*PostgresRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("schedule postgres repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("schedule postgres repository: storage driver must be postgres, got %q", handle.Driver())
	}
	repo := &PostgresRepository{SQLRepository: newSQLRepository(handle.DB(), handle.Dialect())}
	if err := repo.ensureSchema(context.Background(), true); err != nil {
		return nil, err
	}
	return repo, nil
}
