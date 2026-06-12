package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8/internal/caller"
	taskapp "github.com/tinoosan/agen8/internal/services/task/app"
	taskrpc "github.com/tinoosan/agen8/internal/services/task/rpc"
)

const (
	MethodTaskCreate         = "task.create"
	MethodTaskGet            = "task.get"
	MethodTaskList           = "task.list"
	MethodTaskUpdate         = "task.update"
	MethodTaskCancel         = "task.cancel"
	MethodTaskAssign         = "task.assign"
	MethodTaskAttachArtifact = "task.attachArtifact"
)

func RegisterTask(reg *Registry, taskSvc *taskapp.Service, members taskrpc.MemberDisplayLookup) error {
	if taskSvc == nil {
		return fmt.Errorf("task service is required")
	}
	handler := taskrpc.NewHandler(taskSvc, members)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodTaskCreate, false, withTaskCaller(handler.Create))
		},
		func() error {
			return AddBoundHandler(reg, MethodTaskGet, false, withTaskCaller(handler.Get))
		},
		func() error {
			return AddBoundHandler(reg, MethodTaskList, false, withTaskCaller(handler.List))
		},
		func() error {
			return AddBoundHandler(reg, MethodTaskUpdate, false, withTaskCaller(handler.Update))
		},
		func() error {
			return AddBoundHandler(reg, MethodTaskCancel, false, withTaskCaller(handler.Cancel))
		},
		func() error {
			return AddBoundHandler(reg, MethodTaskAssign, false, withTaskCaller(handler.Assign))
		},
		func() error {
			return AddBoundHandler(reg, MethodTaskAttachArtifact, false, withTaskCaller(handler.AttachArtifact))
		},
	)
}

func withTaskCaller[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
	return func(ctx context.Context, params Params) (Result, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			var zero Result
			return zero, err
		}
		ctx = caller.ContextWithCaller(ctx, caller.Caller{
			UserID:   identity.UserID,
			MemberID: identity.MemberID,
			Role:     identity.Role,
		})
		return fn(ctx, params)
	}
}
