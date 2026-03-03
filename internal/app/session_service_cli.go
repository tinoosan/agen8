package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	implstore "github.com/tinoosan/agen8/internal/store"
	"github.com/tinoosan/agen8/pkg/config"
	pkgsession "github.com/tinoosan/agen8/pkg/services/session"
)

// noopCLISupervisor is a RuntimeSupervisor that does nothing.
// Used when the session service is created for CLI/TUI without a daemon.
type noopCLISupervisor struct{}

var errCLIRuntimeControlUnsupported = errors.New("runtime control is not supported without a daemon supervisor")

func (noopCLISupervisor) ResumeRun(_ context.Context, runID string) error {
	return fmt.Errorf("resume run %s: %w", strings.TrimSpace(runID), errCLIRuntimeControlUnsupported)
}

func (noopCLISupervisor) StopRun(_ context.Context, runID string) error {
	return fmt.Errorf("stop run %s: %w", strings.TrimSpace(runID), errCLIRuntimeControlUnsupported)
}

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
