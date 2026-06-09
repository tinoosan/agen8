package mission

import (
	"context"

	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

type MissionLifecycleService interface {
	CreateMission(context.Context, missionapp.CreateMissionParams) (missiondomain.Mission, error)
	GetMission(context.Context, missiondomain.MissionID) (missiondomain.Mission, error)
	ListMissions(context.Context, string, missiondomain.MissionFilter) ([]missiondomain.Mission, error)
	UpdateMission(context.Context, missionapp.UpdateMissionParams) (missiondomain.Mission, error)
	DeleteMission(context.Context, missionapp.DeleteMissionParams) (missiondomain.Mission, error)
	GetLifecycleHistory(context.Context, missiondomain.MissionID, missionapp.LifecycleHistoryFilter) (missionapp.LifecycleHistory, error)
}

type KeyResultService interface {
	CreateKeyResult(context.Context, missionapp.CreateKeyResultParams) (krdomain.KeyResult, error)
	GetKeyResult(context.Context, krdomain.KeyResultID) (krdomain.KeyResult, error)
	ListKeyResults(context.Context, missiondomain.MissionID) ([]krdomain.KeyResult, error)
	UpdateKeyResult(context.Context, missionapp.UpdateKeyResultParams) (krdomain.KeyResult, error)
	DeleteKeyResult(context.Context, missionapp.DeleteKeyResultParams) (krdomain.KeyResult, error)
	ReopenKeyResult(context.Context, missionapp.ReopenKeyResultParams) (krdomain.KeyResult, error)
}

type ProgressService interface {
	UpdateProgress(context.Context, missionapp.UpdateProgressParams) (krdomain.KeyResult, error)
	GetProgressHistory(context.Context, krdomain.KeyResultID) ([]krdomain.ProgressEntry, error)
	ComputeMissionProgress(context.Context, missiondomain.MissionID) (missionapp.MissionProgress, error)
}

type MemberDirectory interface {
	GetMember(ctx context.Context, id member.ID) (member.Record, error)
}

type CallContext struct {
	Missions      MissionLifecycleService
	KeyResults    KeyResultService
	Progress      ProgressService
	Members       MemberDirectory
	ProjectID     string
	ActorMemberID string
}

type Result struct {
	Text       string
	Structured any
}

type rawRequest struct {
	Action          string   `json:"action"`
	MissionID       *string  `json:"mission_id"`
	KeyResultID     *string  `json:"key_result_id"`
	ProjectID       *string  `json:"project_id"`
	Title           *string  `json:"title"`
	Description     *string  `json:"description"`
	Status          *string  `json:"status"`
	StartDate       *string  `json:"start_date"`
	EndDate         *string  `json:"end_date"`
	Limit           *int     `json:"limit"`
	Offset          *int     `json:"offset"`
	MeasurementType *string  `json:"measurement_type"`
	Direction       *string  `json:"direction"`
	Unit            *string  `json:"unit"`
	Baseline        *float64 `json:"baseline"`
	TargetValue     *float64 `json:"target_value"`
	Value           *float64 `json:"value"`
	Note            *string  `json:"note"`
	ExpectedVersion *int64   `json:"expected_version"`
}

type requestInput struct {
	Action          string
	MissionID       string
	KeyResultID     string
	ProjectID       string
	Title           string
	Description     string
	Status          string
	StartDate       string
	EndDate         string
	Limit           int
	Offset          int
	MeasurementType string
	Direction       string
	Unit            string
	Baseline        *float64
	TargetValue     *float64
	Value           *float64
	Note            string
	ExpectedVersion int64
}
