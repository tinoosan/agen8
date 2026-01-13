package resources

import (
	"fmt"
	"os"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// ResultsResource is a directory-backed resource mounted at "/results".
//
// Usage pattern (callId-first, Pattern A)
//
// Tool call outputs are stored under:
//
//	/results/<callId>/response.json
//	/results/<callId>/artifacts/<file>
//
// This resource is only the filesystem mount; the tool runner/host decides what it writes.
// On disk (implementation detail) the base dir is:
//
//	data/runs/<runId>/results
func NewRunResults(runId string) (*DirResource, error) {
	if runId == "" {
		return nil, fmt.Errorf("runId cannot be empty")
	}

	baseDir := fsutil.GetResultsDir(config.DataDir, runId)
	// create results directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating results directory %s: %w", baseDir, err)
	}
	return NewDirResource(baseDir, vfs.MountResults)
}
