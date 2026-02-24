package storage

import (
	"context"
)

// WorkspacePreparer ensures a team workspace directory exists.
// Used by RPC session.start (team mode) to avoid direct os.MkdirAll in handlers.
type WorkspacePreparer interface {
	// PrepareTeamWorkspace creates the team workspace directory if needed.
	PrepareTeamWorkspace(ctx context.Context, teamID string) error
}
