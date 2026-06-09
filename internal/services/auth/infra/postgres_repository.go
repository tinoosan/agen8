package infra

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8/internal/services/auth/apikey"
	"github.com/tinoosan/agen8/internal/services/auth/linktoken"
	"github.com/tinoosan/agen8/internal/services/auth/password"
	"github.com/tinoosan/agen8/internal/services/auth/session"
	user "github.com/tinoosan/agen8/internal/services/user/domain"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

type PostgresRepositories struct {
	store *sqlStore
}

type postgresPasswordRepository struct {
	store *sqlStore
}

type postgresSessionRepository struct {
	store *sqlStore
}

type postgresAPIKeyRepository struct {
	store *sqlStore
}

type postgresLinkTokenRepository struct {
	store *sqlStore
}

func newPostgresRepositories(handle *storagedb.Handle) (Repositories, error) {
	if handle == nil {
		return Repositories{}, fmt.Errorf("storage handle is required")
	}
	if handle.Driver() != storagedb.DriverPostgres {
		return Repositories{}, fmt.Errorf("postgres auth repository requires postgres handle")
	}
	store := newSQLStore(handle)
	if err := store.ensureSchema(context.Background()); err != nil {
		return Repositories{}, err
	}
	repos := &PostgresRepositories{store: store}
	return Repositories{
		Passwords:  &postgresPasswordRepository{store: repos.store},
		Sessions:   &postgresSessionRepository{store: repos.store},
		APIKeys:    &postgresAPIKeyRepository{store: repos.store},
		LinkTokens: &postgresLinkTokenRepository{store: repos.store},
	}, nil
}

func (r *postgresPasswordRepository) Get(ctx context.Context, userID user.ID) (password.Credential, error) {
	return r.store.Get(ctx, userID)
}

func (r *postgresPasswordRepository) Save(ctx context.Context, credential password.Credential) error {
	return r.store.Save(ctx, credential)
}

func (r *postgresPasswordRepository) Delete(ctx context.Context, userID user.ID) error {
	return r.store.Delete(ctx, userID)
}

func (r *postgresSessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (session.Session, error) {
	return r.store.GetByTokenHash(ctx, tokenHash)
}

func (r *postgresSessionRepository) Create(ctx context.Context, record session.Session) error {
	return r.store.Create(ctx, record)
}

func (r *postgresSessionRepository) Update(ctx context.Context, record session.Session) error {
	return r.store.Update(ctx, record)
}

func (r *postgresAPIKeyRepository) GetByTokenHash(ctx context.Context, tokenHash string) (apikey.Key, error) {
	return r.store.GetAPIKeyByTokenHash(ctx, tokenHash)
}

func (r *postgresAPIKeyRepository) Get(ctx context.Context, id apikey.ID) (apikey.Key, error) {
	return r.store.GetAPIKey(ctx, id)
}

func (r *postgresAPIKeyRepository) ListByUser(ctx context.Context, userID user.ID) ([]apikey.Key, error) {
	return r.store.ListAPIKeysByUser(ctx, userID)
}

func (r *postgresAPIKeyRepository) Create(ctx context.Context, key apikey.Key) error {
	return r.store.CreateAPIKey(ctx, key)
}

func (r *postgresAPIKeyRepository) Update(ctx context.Context, key apikey.Key) error {
	return r.store.UpdateAPIKey(ctx, key)
}

func (r *postgresLinkTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (linktoken.LinkToken, error) {
	return r.store.GetLinkTokenByTokenHash(ctx, tokenHash)
}

func (r *postgresLinkTokenRepository) Get(ctx context.Context, id linktoken.ID) (linktoken.LinkToken, error) {
	return r.store.GetLinkToken(ctx, id)
}

func (r *postgresLinkTokenRepository) Create(ctx context.Context, token linktoken.LinkToken) error {
	return r.store.CreateLinkToken(ctx, token)
}

func (r *postgresLinkTokenRepository) Update(ctx context.Context, token linktoken.LinkToken) error {
	return r.store.UpdateLinkToken(ctx, token)
}
