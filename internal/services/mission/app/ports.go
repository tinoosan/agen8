package app

import (
	"context"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	krdomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8-mcp-server/internal/services/mission/domain/mission"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	taskdomain "github.com/tinoosan/agen8-mcp-server/internal/services/task/domain"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

type Caller = caller.Caller

type SpaceLoader interface {
	Get(ctx context.Context, spaceID spacedomain.SpaceID) (spacedomain.SpaceRecord, error)
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
