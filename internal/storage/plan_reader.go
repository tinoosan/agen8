package storage

import (
	"context"

	"github.com/tinoosan/agen8/pkg/types"
)

// PlanReader reads plan files (HEAD.md, CHECKLIST.md) for a run.
// Used by RPC plan.get to avoid direct file I/O in handlers.
type PlanReader interface {
	// ReadPlan returns checklist and details content for the run.
	// Errors for individual files are returned as checklistErr and detailsErr.
	ReadPlan(ctx context.Context, run types.Run) (checklist, details string, checklistErr, detailsErr string)
}
