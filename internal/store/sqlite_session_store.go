package store

import (
	"context"

	"github.com/tinoosan/agen8/pkg/config"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/types"
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

func (s *SQLiteSessionStore) LoadRun(_ context.Context, runID string) (types.Run, error) {
	return LoadRun(s.Cfg, runID)
}

func (s *SQLiteSessionStore) ListRunsBySession(_ context.Context, sessionID string) ([]types.Run, error) {
	return ListRunsBySession(s.Cfg, sessionID)
}

func (s *SQLiteSessionStore) SaveRun(_ context.Context, run types.Run) error {
	return SaveRun(s.Cfg, run)
}

func (s *SQLiteSessionStore) StopRun(_ context.Context, runID, status, errorMsg string) (types.Run, error) {
	return StopRun(s.Cfg, runID, status, errorMsg)
}

func (s *SQLiteSessionStore) ListRunsByStatus(_ context.Context, statuses []string) ([]types.Run, error) {
	return ListRunsByStatus(s.Cfg, statuses)
}

func (s *SQLiteSessionStore) ListChildRuns(_ context.Context, parentRunID string) ([]types.Run, error) {
	return ListChildRuns(s.Cfg, parentRunID)
}

func (s *SQLiteSessionStore) AddRunToSession(_ context.Context, sessionID, runID string) (types.Session, error) {
	return AddRunToSession(s.Cfg, sessionID, runID)
}

func (s *SQLiteSessionStore) ListActivities(ctx context.Context, runID string, limit, offset int) ([]types.Activity, error) {
	return ListActivities(ctx, s.Cfg, runID, limit, offset)
}

func (s *SQLiteSessionStore) CountActivities(ctx context.Context, runID string) (int, error) {
	return CountActivities(ctx, s.Cfg, runID)
}

func (s *SQLiteSessionStore) DeleteSession(_ context.Context, sessionID string) error {
	return DeleteSession(s.Cfg, sessionID)
}

func (s *SQLiteSessionStore) ClearHistoryForSession(_ context.Context, sessionID string) (HistoryClearResult, error) {
	return ClearHistoryForSession(s.Cfg, sessionID)
}

func (s *SQLiteSessionStore) ClearHistoryForRunIDs(_ context.Context, runIDs []string) (HistoryClearResult, error) {
	return ClearHistoryForRunIDs(s.Cfg, runIDs)
}
