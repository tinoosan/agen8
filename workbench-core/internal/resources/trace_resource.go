package resources

import (
	"fmt"
	"os"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type TraceResource struct {
	BaseDir string
	RunId   string
}

func NewTraceResource(runId string) (*TraceResource, error) {
	if runId == "" {
		return nil, fmt.Errorf("runId cannot be empty")
	}
	baseDir := fsutil.GetTraceDir(config.DataDir, runId)
	// create trace directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating trace directory %s: %w", baseDir, err)
	}
	return &TraceResource{
		BaseDir: baseDir,
		RunId:   runId,
	}, nil
}

func (tr *TraceResource) List(subpath string) ([]vfs.Entry, error) {
	// Treat "" and "." as the root of the trace resource
	if subpath == "" || subpath == "." {
		return []vfs.Entry{
			{Path: "events", IsDir: false},
			{Path: "run", IsDir: false},
		}, nil
	}

	// Anything else is a non-root list request
	return nil, fmt.Errorf("invalid subpath %q: cannot list non-root", subpath)
}

func (tr *TraceResource) Read(subpath string) ([]byte, error) {
	return nil, nil
}

func (tr *TraceResource) Write(subpath string, data []byte) error {
	return fmt.Errorf("write not supported for trace resource")
}

func (tr *TraceResource) Append(subpath string, data []byte) error {
	return fmt.Errorf("append not supported for trace resource")
}

