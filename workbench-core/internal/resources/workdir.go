package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/internal/validate"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// NewWorkdirResource creates a directory-backed resource for the host working directory.
//
// /workdir is intended to map to the OS directory the user launched Workbench from
// (or a flag override). Unlike /workspace (run-scoped scratch), /workdir should point
// at "real project files" so the agent can read and modify them via normal fs.* ops.
func NewWorkdirResource(workdir string) (*DirResource, error) {
	workdir = strings.TrimSpace(workdir)
	if err := validate.NonEmpty("workdir", workdir); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(workdir)
	if err != nil {
		return nil, fmt.Errorf("abs workdir: %w", err)
	}
	st, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat workdir: %w", err)
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("workdir is not a directory: %s", abs)
	}
	return NewDirResource(abs, vfs.MountWorkdir)
}
