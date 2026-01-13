package resources

import (
	"fmt"
	"os"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// NewRunWorkspace creates a directory-backed resource for a run's workspace.
// The workspace is the agent-writable working directory mounted at "/workspace".
// Creates the directory if it doesn't exist.
func NewRunWorkspace(runId string) (*DirResource, error) {
	if runId == "" {
		return nil, fmt.Errorf("runId cannot be empty")
	}

	baseDir := fsutil.GetWorkspaceDir(config.DataDir, runId)
	// create workspace directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating workspace directory %s: %w", baseDir, err)
	}
	return NewDirResource(baseDir, vfs.MountWorkspace)
}
