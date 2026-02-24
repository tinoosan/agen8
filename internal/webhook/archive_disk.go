package webhook

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/tinoosan/agen8/pkg/types"
)

// DiskTaskArchiveWriter implements TaskArchiveWriter by writing task JSON
// files to a directory. The archive directory is created on first write.
type DiskTaskArchiveWriter struct {
	archiveDir string
}

// NewDiskTaskArchiveWriter returns a TaskArchiveWriter that writes to the
// given directory (e.g. runDir/inbox/archive or teamDir/inbox/archive).
func NewDiskTaskArchiveWriter(archiveDir string) *DiskTaskArchiveWriter {
	return &DiskTaskArchiveWriter{archiveDir: archiveDir}
}

// ArchiveTask writes the task as JSON to archiveDir/<taskID>.json.
// Best-effort; errors are ignored.
func (d *DiskTaskArchiveWriter) ArchiveTask(ctx context.Context, task types.Task) {
	if task.TaskID == "" {
		return
	}
	_ = os.MkdirAll(d.archiveDir, 0o755)
	b, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(d.archiveDir, task.TaskID+".json"), b, 0o644)
}
