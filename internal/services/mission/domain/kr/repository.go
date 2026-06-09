package kr

import (
	"context"
	"time"

	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
)

type KeyResultRepository interface {
	GetKeyResult(ctx context.Context, keyResultID KeyResultID) (KeyResult, error)
	ListKeyResults(ctx context.Context, missionID missiondomain.MissionID) ([]KeyResult, error)
	CreateKeyResult(ctx context.Context, keyResult KeyResult) error
	UpdateKeyResult(ctx context.Context, keyResult KeyResult) error
}

type ProgressEntryRepository interface {
	AppendProgressEntry(ctx context.Context, entry ProgressEntry) error
	ListProgressEntries(ctx context.Context, keyResultID KeyResultID) ([]ProgressEntry, error)
}

type ProgressEntry struct {
	ID              string
	KeyResultID     KeyResultID
	PreviousValue   float64
	NewValue        float64
	ProgressPercent int
	UpdatedBy       string
	Note            string
	CreatedAt       time.Time
}
