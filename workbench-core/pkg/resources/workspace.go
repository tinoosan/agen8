package resources

import (
	"fmt"
	"os"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/validate"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// NewWorkspace creates a directory-backed resource for a run's workspace.
// The workspace is the agent-writable working directory mounted at "/scratch".
func NewWorkspace(cfg config.Config, runId string) (*DirResource, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("runId", runId); err != nil {
		return nil, err
	}

	baseDir := fsutil.GetScratchDir(cfg.DataDir, runId)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating workspace directory %s: %w", baseDir, err)
	}
	return NewDirResource(baseDir, vfs.MountScratch)
}
