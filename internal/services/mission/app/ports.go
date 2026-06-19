package app

import (
	"context"
	"strings"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

type Caller = caller.Caller

type LinkedTaskID string

type LinkedTaskStatus string

const (
	LinkedTaskStatusSucceeded LinkedTaskStatus = "succeeded"
	LinkedTaskStatusFailed    LinkedTaskStatus = "failed"
	LinkedTaskStatusCanceled  LinkedTaskStatus = "canceled"
)

func (s LinkedTaskStatus) IsTerminal() bool {
	switch s {
	case LinkedTaskStatusSucceeded, LinkedTaskStatusFailed, LinkedTaskStatusCanceled:
		return true
	default:
		return false
	}
}

type LinkedTaskSnapshot struct {
	ID     LinkedTaskID
	Status LinkedTaskStatus
}

func (t LinkedTaskSnapshot) CleanID() LinkedTaskID {
	return LinkedTaskID(strings.TrimSpace(string(t.ID)))
}

type TaskLoader interface {
	Get(ctx context.Context, taskID LinkedTaskID) (LinkedTaskSnapshot, error)
}

type LinkedTaskLoader interface {
	ListTaskIDsForKeyResult(ctx context.Context, keyResultID krdomain.KeyResultID) ([]LinkedTaskID, error)
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
