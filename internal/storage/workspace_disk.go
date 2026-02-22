package storage

import (
	"context"
	"os"

	"github.com/tinoosan/agen8/pkg/fsutil"
)

// DiskWorkspacePreparer implements WorkspacePreparer using os.MkdirAll.
type DiskWorkspacePreparer struct {
	dataDir string
}

// NewDiskWorkspacePreparer returns a WorkspacePreparer for the given data directory.
func NewDiskWorkspacePreparer(dataDir string) *DiskWorkspacePreparer {
	return &DiskWorkspacePreparer{dataDir: dataDir}
}

// PrepareTeamWorkspace creates the team workspace directory.
func (d *DiskWorkspacePreparer) PrepareTeamWorkspace(ctx context.Context, teamID string) error {
	dir := fsutil.GetTeamWorkspaceDir(d.dataDir, teamID)
	return os.MkdirAll(dir, 0o755)
}
