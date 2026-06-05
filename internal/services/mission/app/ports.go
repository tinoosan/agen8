package app

import (
	"context"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	krdomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
)

type Caller = caller.Caller

type ProjectLoader interface {
	Get(ctx context.Context, projectID types.ProjectID) (projectdomain.Project, error)
}

type TaskLoader interface {
	Get(ctx context.Context, taskID taskdomain.TaskID) (taskdomain.Task, error)
}

type LinkedTaskLoader interface {
	ListTaskIDsForKeyResult(ctx context.Context, keyResultID krdomain.KeyResultID) ([]taskdomain.TaskID, error)
}

type EventPublisher interface {
	Append(ctx context.Context, event types.EventRecord) error
}

type LifecycleEventRepository interface {
	AppendLifecycleEvent(ctx context.Context, event types.EventRecord) error
	ListLifecycleEvents(ctx context.Context, missionID missiondomain.MissionID, filter LifecycleHistoryFilter) ([]types.EventRecord, int, error)
}

type MissionRepository = missiondomain.Repository
type KeyResultRepository = krdomain.KeyResultRepository
type ProgressEntryRepository = krdomain.ProgressEntryRepository
