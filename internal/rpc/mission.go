package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8/internal/caller"
	missionapp "github.com/tinoosan/agen8/internal/services/mission/app"
	missionrpc "github.com/tinoosan/agen8/internal/services/mission/rpc"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
)

const (
	MethodMissionCreate   = "mission.create"
	MethodMissionGet      = "mission.get"
	MethodMissionList     = "mission.list"
	MethodMissionUpdate   = "mission.update"
	MethodMissionDelete   = "mission.delete"
	MethodMissionPurge    = "mission.purge"
	MethodMissionProgress = "mission.progress"
	MethodMissionHistory  = "mission.history"

	MethodMissionKRCreate   = "mission.kr.create"
	MethodMissionKRGet      = "mission.kr.get"
	MethodMissionKRList     = "mission.kr.list"
	MethodMissionKRUpdate   = "mission.kr.update"
	MethodMissionKRDelete   = "mission.kr.delete"
	MethodMissionKRReopen   = "mission.kr.reopen"
	MethodMissionKRProgress = "mission.kr.progress"
	MethodMissionKRHistory  = "mission.kr.progressHistory"
)

func RegisterMission(reg *Registry, missionSvc *missionapp.Service) error {
	if missionSvc == nil {
		return fmt.Errorf("mission service is required")
	}
	handler := missionrpc.NewHandler(missionSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodMissionCreate, false, withMissionCaller(handler.Create))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionGet, false, withMissionCaller(handler.Get))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionList, false, withMissionCaller(handler.List))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionUpdate, false, withMissionCaller(handler.Update))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionDelete, false, withMissionCaller(handler.Delete))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionPurge, false, withMissionCaller(handler.Purge))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionProgress, false, withMissionCaller(handler.Progress))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionHistory, false, withMissionCaller(handler.LifecycleHistory))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionKRCreate, false, withMissionCaller(handler.CreateKeyResult))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionKRGet, false, withMissionCaller(handler.GetKeyResult))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionKRList, false, withMissionCaller(handler.ListKeyResults))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionKRUpdate, false, withMissionCaller(handler.UpdateKeyResult))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionKRDelete, false, withMissionCaller(handler.DeleteKeyResult))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionKRReopen, false, withMissionCaller(handler.ReopenKeyResult))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionKRProgress, false, withMissionCaller(handler.UpdateProgress))
		},
		func() error {
			return AddBoundHandler(reg, MethodMissionKRHistory, false, withMissionCaller(handler.ProgressHistory))
		},
	)
}

func withMissionCaller[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
	return func(ctx context.Context, params Params) (Result, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			var zero Result
			return zero, err
		}
		ctx = caller.ContextWithCaller(ctx, caller.Caller{
			UserID:   identity.UserID,
			MemberID: member.ID(identity.MemberID),
			Role:     identity.Role,
		})
		return fn(ctx, params)
	}
}
