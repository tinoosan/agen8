package agent

import (
	"context"

	"github.com/tinoosan/agen8/pkg/types"
)

type Notifier interface {
	Notify(ctx context.Context, task types.Task, result types.TaskResult) error
}

