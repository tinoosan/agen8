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
}

func NewTraceResource(runId string) (*TraceResource, error) {
	baseDir := fsutil.GetTraceDir(config.DataDir, runId)
	if runId == "" {
		return nil, fmt.Errorf("runId cannot be empty")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("error creating trace directory %s: %w", baseDir, err)
	}
	return &TraceResource{
		BaseDir: baseDir,
	}, nil
}

func (tr *TraceResource) List(subpath string) ([]vfs.Entry, error) {
	return nil, nil
}

func (tr *TraceResource) Read(subpath string) ([]byte, error) {
	return nil, nil
}

func (tr *TraceResource) Write(subpath string, data []byte) error {
	return nil
}

func (tr *TraceResource) Append(subpath string, data []byte) error {
	return nil
}

