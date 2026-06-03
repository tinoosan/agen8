package protocol

import (
	"time"

	"github.com/tinoosan/agen8-mcp-server/pkg/types"
)

const (
	MethodHeartbeatScheduleList    = "heartbeat.schedule.list"
	MethodHeartbeatScheduleApprove = "heartbeat.schedule.approve"
	MethodHeartbeatScheduleReject  = "heartbeat.schedule.reject"
)

type HeartbeatScheduleListParams struct {
	SpaceID string `json:"spaceId"`
}

type HeartbeatScheduleListResult struct {
	Entries []HeartbeatScheduleEntry `json:"entries"`
}

type HeartbeatScheduleApproveParams struct {
	EntryID string `json:"entryId"`
}

type HeartbeatScheduleApproveResult struct {
	Entry HeartbeatScheduleEntry `json:"entry"`
}

type HeartbeatScheduleRejectParams struct {
	EntryID string `json:"entryId"`
}

type HeartbeatScheduleRejectResult struct {
	Entry HeartbeatScheduleEntry `json:"entry"`
}

type HeartbeatScheduleEntry struct {
	EntryID         string                               `json:"entryId"`
	SpaceID         string                               `json:"spaceId"`
	RoleID          string                               `json:"roleId"`
	CreatedBy       string                               `json:"createdBy"`
	Name            string                               `json:"name"`
	Goal            string                               `json:"goal"`
	Context         *types.HeartbeatScheduleEntryContext `json:"context,omitempty"`
	Priority        int                                  `json:"priority"`
	ScheduleType    string                               `json:"scheduleType"`
	ScheduleExpr    string                               `json:"scheduleExpr"`
	Status          string                               `json:"status"`
	GuardrailReason string                               `json:"guardrailReason,omitempty"`
	DedupeKey       string                               `json:"dedupeKey,omitempty"`
	ExpiresAt       *time.Time                           `json:"expiresAt,omitempty"`
	NextRunAt       *time.Time                           `json:"nextRunAt,omitempty"`
	CreatedAt       time.Time                            `json:"createdAt"`
	UpdatedAt       time.Time                            `json:"updatedAt"`
}

func NewHeartbeatScheduleEntry(entry types.HeartbeatScheduleEntry) HeartbeatScheduleEntry {
	return HeartbeatScheduleEntry{
		EntryID:         entry.EntryID,
		SpaceID:         entry.SpaceID,
		RoleID:          entry.RoleID,
		CreatedBy:       entry.CreatedBy,
		Name:            entry.Name,
		Goal:            entry.Goal,
		Context:         entry.Context,
		Priority:        entry.Priority,
		ScheduleType:    entry.ScheduleType,
		ScheduleExpr:    entry.ScheduleExpr,
		Status:          entry.Status,
		GuardrailReason: entry.GuardrailReason,
		DedupeKey:       entry.DedupeKey,
		ExpiresAt:       entry.ExpiresAt,
		NextRunAt:       entry.NextRunAt,
		CreatedAt:       entry.CreatedAt,
		UpdatedAt:       entry.UpdatedAt,
	}
}

type HeartbeatManageUpdatePayload = types.HeartbeatManageUpdateParams
