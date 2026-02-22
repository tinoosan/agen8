package storage

import (
	"context"
	"os"
	"path/filepath"

	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/types"
)

// DiskPlanReader implements PlanReader using the filesystem.
type DiskPlanReader struct {
	dataDir string
}

// NewDiskPlanReader returns a PlanReader that reads from runDir/plan/.
func NewDiskPlanReader(dataDir string) *DiskPlanReader {
	return &DiskPlanReader{dataDir: dataDir}
}

// ReadPlan reads HEAD.md and CHECKLIST.md from the run's plan directory.
func (d *DiskPlanReader) ReadPlan(ctx context.Context, run types.Run) (checklist, details string, checklistErr, detailsErr string) {
	runDir := fsutil.GetRunDir(d.dataDir, run)
	planDir := filepath.Join(runDir, "plan")
	load := func(name string) (string, string) {
		b, err := os.ReadFile(filepath.Join(planDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				return "", ""
			}
			return "", err.Error()
		}
		return string(b), ""
	}
	details, detailsErr = load("HEAD.md")
	checklist, checklistErr = load("CHECKLIST.md")
	return checklist, details, checklistErr, detailsErr
}
