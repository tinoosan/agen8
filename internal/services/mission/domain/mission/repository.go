package mission

import "context"

type Repository interface {
	GetMission(ctx context.Context, missionID MissionID) (Mission, error)
	ListMissions(ctx context.Context, filter MissionFilter) ([]Mission, error)
	CreateMission(ctx context.Context, mission Mission) error
	UpdateMission(ctx context.Context, mission Mission) error
}

type MissionFilter struct {
	ProjectID string
	Statuses  []MissionStatus
	Limit     int
	Offset    int
}
