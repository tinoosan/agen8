package mission

import "context"

type Repository interface {
	GetMission(ctx context.Context, missionID MissionID) (Mission, error)
	ListMissions(ctx context.Context, filter MissionFilter) ([]Mission, error)
	CreateMission(ctx context.Context, mission Mission) error
	UpdateMission(ctx context.Context, mission Mission) error
	// DeleteMission permanently removes a mission and every descendant it owns
	// (key results, their progress entries, and lifecycle events) in a single
	// atomic operation. This is the storage-level hard delete; soft delete is
	// modelled as an archive status update via UpdateMission.
	DeleteMission(ctx context.Context, missionID MissionID) error
}

type MissionFilter struct {
	ProjectID string
	Statuses  []MissionStatus
	Limit     int
	Offset    int
}
