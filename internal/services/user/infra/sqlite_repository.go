package infra

import (
	"context"
	"fmt"

	user "github.com/tinoosan/agen8/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type SQLiteRepository struct {
	store *sqlStore
}

func NewSQLiteRepository(handle *storagedb.Handle) (*SQLiteRepository, error) {
	if handle == nil || handle.DB() == nil {
		return nil, fmt.Errorf("user sqlite repository: db handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return nil, fmt.Errorf("user sqlite repository: storage driver must be sqlite, got %q", handle.Driver())
	}
	store := newSQLStore(handle)
	if err := store.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return &SQLiteRepository{store: store}, nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id user.ID) (user.User, error) {
	return r.store.Get(ctx, id)
}

func (r *SQLiteRepository) GetByEmail(ctx context.Context, email string) (user.User, error) {
	return r.store.GetByEmail(ctx, email)
}

func (r *SQLiteRepository) FirstActive(ctx context.Context) (user.User, error) {
	return r.store.FirstActive(ctx)
}

func (r *SQLiteRepository) Count(ctx context.Context) (int, error) {
	return r.store.Count(ctx)
}

func (r *SQLiteRepository) Create(ctx context.Context, record user.User) error {
	return r.store.Create(ctx, record)
}

func (r *SQLiteRepository) Update(ctx context.Context, record user.User) error {
	return r.store.Update(ctx, record)
}
