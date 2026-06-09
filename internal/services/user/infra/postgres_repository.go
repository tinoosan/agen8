package infra

import (
	"context"
	"fmt"

	user "github.com/tinoosan/agen8/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type PostgresRepository struct {
	store *sqlStore
}

func NewPostgresRepository(handle *storagedb.Handle) (*PostgresRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("user postgres repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return nil, fmt.Errorf("user postgres repository: storage driver must be postgres, got %q", handle.Driver())
	}
	store := newSQLStore(handle)
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return &PostgresRepository{store: store}, nil
}

func (r *PostgresRepository) Get(ctx context.Context, id user.ID) (user.User, error) {
	return r.store.Get(ctx, id)
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (user.User, error) {
	return r.store.GetByEmail(ctx, email)
}

func (r *PostgresRepository) FirstActive(ctx context.Context) (user.User, error) {
	return r.store.FirstActive(ctx)
}

func (r *PostgresRepository) Count(ctx context.Context) (int, error) {
	return r.store.Count(ctx)
}

func (r *PostgresRepository) Create(ctx context.Context, record user.User) error {
	return r.store.Create(ctx, record)
}

func (r *PostgresRepository) Update(ctx context.Context, record user.User) error {
	return r.store.Update(ctx, record)
}
