package app

import (
	"context"

	"github.com/tinoosan/agen8/pkg/config"
	implstore "github.com/tinoosan/agen8/internal/store"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
)

// noopCLISupervisor is a RuntimeSupervisor that does nothing.
// Used when the session service is created for CLI/TUI without a daemon.
type noopCLISupervisor struct{}

func (noopCLISupervisor) ResumeRun(context.Context, string) error { return nil }
func (noopCLISupervisor) StopRun(string) error                    { return nil }

// NewSessionServiceForCLI creates a session service for CLI and TUI use.
// It uses the SQLite store and a no-op supervisor (Stop/Delete will not actually
// stop runtimes, but LoadRun, LoadSession, ListSessionsPaginated, LatestRun,
// LatestRunningRun work correctly).
func NewSessionServiceForCLI(cfg config.Config) (pkgsession.Service, error) {
	store, err := implstore.NewSQLiteSessionStore(cfg)
	if err != nil {
		return nil, err
	}
	return pkgsession.NewManager(cfg, store, noopCLISupervisor{}), nil
}
