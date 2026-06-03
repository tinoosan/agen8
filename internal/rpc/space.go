package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	spacerpc "github.com/tinoosan/agen8-mcp-server/internal/services/space/rpc"
)

const (
	MethodSpaceCreate = "space.create"
	MethodSpaceGet    = "space.get"
	MethodSpaceList   = "space.list"
	MethodSpaceUpdate = "space.update"
	MethodSpaceClose  = "space.close"
	MethodSpaceReopen = "space.reopen"
	MethodSpaceDelete = "space.delete"

	MethodSpaceMemberRegister     = "space.member.register"
	MethodSpaceMemberGet          = "space.member.get"
	MethodSpaceMemberList         = "space.member.list"
	MethodSpaceMemberUpdateConfig = "space.member.updateConfig"
	MethodSpaceMemberRemove       = "space.member.remove"
)

func RegisterSpace(reg *Registry, spaceSvc *spaceapp.Service) error {
	if spaceSvc == nil {
		return fmt.Errorf("space service is required")
	}
	handler := spacerpc.NewHandler(spaceSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodSpaceCreate, false, withSpaceCaller(handler.SpaceCreate))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceGet, false, withSpaceCaller(handler.SpaceGet))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceList, false, withSpaceCaller(handler.SpaceList))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceUpdate, false, withSpaceCaller(handler.SpaceUpdate))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceClose, false, withSpaceCaller(handler.SpaceClose))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceReopen, false, withSpaceCaller(handler.SpaceReopen))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceDelete, false, withSpaceCaller(handler.SpaceDelete))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceMemberRegister, false, withSpaceCaller(handler.MemberRegister))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceMemberGet, false, withSpaceCaller(handler.MemberGet))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceMemberList, false, withSpaceCaller(handler.MemberList))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceMemberUpdateConfig, false, withSpaceCaller(handler.MemberUpdateConfig))
		},
		func() error {
			return AddBoundHandler(reg, MethodSpaceMemberRemove, false, withSpaceCaller(handler.MemberRemove))
		},
	)
}

func withSpaceCaller[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
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
