package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	scheduleapp "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/app"
	schedulerpc "github.com/tinoosan/agen8-mcp-server/internal/services/schedule/rpc"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

const (
	MethodScheduleCreate = "schedule.create"
	MethodScheduleGet    = "schedule.get"
	MethodScheduleList   = "schedule.list"
	MethodScheduleCancel = "schedule.cancel"
)

func RegisterSchedule(reg *Registry, scheduleSvc *scheduleapp.Service) error {
	if scheduleSvc == nil {
		return fmt.Errorf("schedule service is required")
	}
	handler := schedulerpc.NewHandler(scheduleSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodScheduleCreate, false, withScheduleCaller(handler.Create))
		},
		func() error {
			return AddBoundHandler(reg, MethodScheduleGet, false, withScheduleCaller(handler.Get))
		},
		func() error {
			return AddBoundHandler(reg, MethodScheduleList, false, withScheduleCaller(handler.List))
		},
		func() error {
			return AddBoundHandler(reg, MethodScheduleCancel, false, withScheduleCaller(handler.Cancel))
		},
	)
}

func withScheduleCaller[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
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
