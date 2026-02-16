package state

import (
	"context"
	"time"
)

const (
	TaskKindTask        = "task"
	TaskKindCallback    = "callback"
	TaskKindReview      = "review"
	TaskKindHeartbeat   = "heartbeat"
	TaskKindCoordinator = "coordinator"
	TaskKindOther       = "other"
)

type ArtifactRecord struct {
	ArtifactID  string
	TaskID      string
	TeamID      string
	RunID       string
	Role        string
	TaskKind    string
	IsSummary   bool
	DisplayName string
	VPath       string
	DiskPath    string
	ProducedAt  time.Time
	DayBucket   string
}

type ArtifactGroup struct {
	DayBucket  string
	Role       string
	TaskKind   string
	TaskID     string
	Goal       string
	Status     string
	ProducedAt time.Time
	Files      []ArtifactRecord
}

type ArtifactFilter struct {
	TeamID string
	RunID  string
	TaskID string

	DayBucket string
	Role      string
	TaskKind  string

	Limit int
}

type ArtifactSearchFilter struct {
	ArtifactFilter
	Query string
}

// ArtifactIndexer is an optional store extension used for team-centric deliverable browsing.
type ArtifactIndexer interface {
	UpsertTaskClassification(ctx context.Context, taskID, taskKind, roleSnapshot string) error
	ReplaceTaskArtifacts(ctx context.Context, taskID string, records []ArtifactRecord) error
	ListArtifactGroups(ctx context.Context, filter ArtifactFilter) ([]ArtifactGroup, error)
	ListArtifactsByTask(ctx context.Context, filter ArtifactFilter) ([]ArtifactRecord, error)
	SearchArtifacts(ctx context.Context, filter ArtifactSearchFilter) ([]ArtifactRecord, error)
}
