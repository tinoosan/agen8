package resources

import (
	"fmt"
	"os"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/validate"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// NewWorkspace creates a directory-backed resource for a run's workspace.
// The workspace is the agent-writable working directory mounted at "/scratch".
// Creates the directory if it doesn't exist.
func NewWorkspace(cfg config.Config, runId string) (*DirResource, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("runId", runId); err != nil {
		return nil, err
	}

	baseDir := fsutil.GetScratchDir(cfg.DataDir, runId)
	// create workspace directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating workspace directory %s: %w", baseDir, err)
	}
	return NewDirResource(baseDir, vfs.MountScratch)
}
