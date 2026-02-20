package resources

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/validate"
	"github.com/tinoosan/agen8/pkg/vfs"
)

// NewWorkspace creates a directory-backed resource for a run's workspace.
// The workspace is the agent-writable working directory mounted at "/workspace".
func NewWorkspace(cfg config.Config, runID string) (*DirResource, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("runID", runID); err != nil {
		return nil, err
	}

	baseDir := fsutil.GetWorkspaceDir(cfg.DataDir, runID)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating workspace directory %s: %w", baseDir, err)
	}
	return NewDirResource(baseDir, vfs.MountWorkspace)
}

// NewWorkspaceFromRunDir creates a workspace resource under the given run root directory.
// Use this when the run dir is already computed (e.g. via fsutil.GetRunDir for child runs).
func NewWorkspaceFromRunDir(runDir string) (*DirResource, error) {
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating run directory %s: %w", runDir, err)
	}
	baseDir := filepath.Join(runDir, "workspace")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating workspace directory %s: %w", baseDir, err)
	}
	return NewDirResource(baseDir, vfs.MountWorkspace)
}

// NewWorkspaceFromPath creates a workspace resource at the given path (the workspace root).
// Use this when the workspace path is already computed (e.g. fsutil.GetWorkspaceDirForRun for subagents).
func NewWorkspaceFromPath(workspaceDir string) (*DirResource, error) {
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating workspace directory %s: %w", workspaceDir, err)
	}
	return NewDirResource(workspaceDir, vfs.MountWorkspace)
}
