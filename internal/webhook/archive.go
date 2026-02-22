package webhook

import (
	"context"

	"github.com/tinoosan/agen8/pkg/types"
)

// TaskArchiveWriter persists inbound webhook task payloads for debugging and
// external integrations. Implementations are best-effort; failures must not
// block task creation.
type TaskArchiveWriter interface {
	// ArchiveTask writes the task payload to storage. Best-effort; errors are
	// logged but not returned.
	ArchiveTask(ctx context.Context, task types.Task)
}
