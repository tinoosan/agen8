package mission

type MissionID string

type MissionStatus string

const (
	MissionStatusDraft     MissionStatus = "draft"
	MissionStatusActive    MissionStatus = "active"
	MissionStatusPaused    MissionStatus = "paused"
	MissionStatusCompleted MissionStatus = "completed"
	MissionStatusArchived  MissionStatus = "archived"
)
