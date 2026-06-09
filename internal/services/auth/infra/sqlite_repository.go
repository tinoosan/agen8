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

type SQLiteRepositories struct {
	store *sqlStore
}

type sqlitePasswordRepository struct {
	store *sqlStore
}

type sqliteSessionRepository struct {
	store *sqlStore
}

type sqliteAPIKeyRepository struct {
	store *sqlStore
}

type sqliteLinkTokenRepository struct {
	store *sqlStore
}

func newSQLiteRepositories(handle *storagedb.Handle) (Repositories, error) {
	if handle == nil {
		return Repositories{}, fmt.Errorf("storage handle is required")
	}
	if handle.Driver() != storagedb.DriverSQLite {
		return Repositories{}, fmt.Errorf("sqlite auth repository requires sqlite handle")
	}
	store := newSQLStore(handle)
	if err := store.ensureSchema(context.Background()); err != nil {
		return Repositories{}, err
	}
	repos := &SQLiteRepositories{store: store}
	return Repositories{
		Passwords:  &sqlitePasswordRepository{store: repos.store},
		Sessions:   &sqliteSessionRepository{store: repos.store},
		APIKeys:    &sqliteAPIKeyRepository{store: repos.store},
		LinkTokens: &sqliteLinkTokenRepository{store: repos.store},
	}, nil
}

func (r *sqlitePasswordRepository) Get(ctx context.Context, userID user.ID) (password.Credential, error) {
	return r.store.Get(ctx, userID)
}

func (r *sqlitePasswordRepository) Save(ctx context.Context, credential password.Credential) error {
	return r.store.Save(ctx, credential)
}

func (r *sqlitePasswordRepository) Delete(ctx context.Context, userID user.ID) error {
	return r.store.Delete(ctx, userID)
}

func (r *sqliteSessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (session.Session, error) {
	return r.store.GetByTokenHash(ctx, tokenHash)
}

func (r *sqliteSessionRepository) Create(ctx context.Context, record session.Session) error {
	return r.store.Create(ctx, record)
}

func (r *sqliteSessionRepository) Update(ctx context.Context, record session.Session) error {
	return r.store.Update(ctx, record)
}

func (r *sqliteAPIKeyRepository) GetByTokenHash(ctx context.Context, tokenHash string) (apikey.Key, error) {
	return r.store.GetAPIKeyByTokenHash(ctx, tokenHash)
}

func (r *sqliteAPIKeyRepository) Get(ctx context.Context, id apikey.ID) (apikey.Key, error) {
	return r.store.GetAPIKey(ctx, id)
}

func (r *sqliteAPIKeyRepository) ListByUser(ctx context.Context, userID user.ID) ([]apikey.Key, error) {
	return r.store.ListAPIKeysByUser(ctx, userID)
}

func (r *sqliteAPIKeyRepository) Create(ctx context.Context, key apikey.Key) error {
	return r.store.CreateAPIKey(ctx, key)
}

func (r *sqliteAPIKeyRepository) Update(ctx context.Context, key apikey.Key) error {
	return r.store.UpdateAPIKey(ctx, key)
}

func (r *sqliteLinkTokenRepository) GetByTokenHash(ctx context.Context, tokenHash string) (linktoken.LinkToken, error) {
	return r.store.GetLinkTokenByTokenHash(ctx, tokenHash)
}

func (r *sqliteLinkTokenRepository) Get(ctx context.Context, id linktoken.ID) (linktoken.LinkToken, error) {
	return r.store.GetLinkToken(ctx, id)
}

func (r *sqliteLinkTokenRepository) Create(ctx context.Context, token linktoken.LinkToken) error {
	return r.store.CreateLinkToken(ctx, token)
}

func (r *sqliteLinkTokenRepository) Update(ctx context.Context, token linktoken.LinkToken) error {
	return r.store.UpdateLinkToken(ctx, token)
}
