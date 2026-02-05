package store

import (
	"context"

	"github.com/tinoosan/workbench-core/pkg/config"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// SQLiteSessionStore adapts the package-level SQLite session functions to the pkg/store session interfaces.
type SQLiteSessionStore struct {
	Cfg config.Config
}

func NewSQLiteSessionStore(cfg config.Config) (*SQLiteSessionStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &SQLiteSessionStore{Cfg: cfg}, nil
}

func (s *SQLiteSessionStore) LoadSession(_ context.Context, sessionID string) (types.Session, error) {
	return LoadSession(s.Cfg, sessionID)
}

func (s *SQLiteSessionStore) SaveSession(_ context.Context, sess types.Session) error {
	return SaveSession(s.Cfg, sess)
}

func (s *SQLiteSessionStore) ListSessionsPaginated(_ context.Context, filter pkgstore.SessionFilter) ([]types.Session, error) {
	return ListSessionsPaginated(s.Cfg, filter)
}

func (s *SQLiteSessionStore) CountSessions(_ context.Context, filter pkgstore.SessionFilter) (int, error) {
	return CountSessions(s.Cfg, filter)
}

func (s *SQLiteSessionStore) ListSessionIDs(_ context.Context) ([]string, error) {
	return ListSessionIDs(s.Cfg)
}

func (s *SQLiteSessionStore) ListSessions(_ context.Context) ([]types.Session, error) {
	return ListSessions(s.Cfg)
}
