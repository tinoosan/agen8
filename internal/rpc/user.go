package rpc

import (
	"context"
	"fmt"

	userapp "github.com/tinoosan/agen8-mcp-server/internal/services/user/app"
	userrpc "github.com/tinoosan/agen8-mcp-server/internal/services/user/rpc"
)

const (
	MethodUserStatus        = "user.status"
	MethodUserGet           = "user.get"
	MethodUserUpdateProfile = "user.updateProfile"
	MethodUserSuspend       = "user.suspend"
	MethodUserClose         = "user.close"
)

func RegisterUser(reg *Registry, userSvc *userapp.Service) error {
	if userSvc == nil {
		return fmt.Errorf("user service is required")
	}
	handler := userrpc.NewHandler(userSvc, func(ctx context.Context) (userrpc.Identity, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			return userrpc.Identity{}, err
		}
		return userrpc.Identity{
			UserID: identity.UserID,
			Role:   identity.Role,
		}, nil
	})
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodUserStatus, true, handler.Status)
		},
		func() error {
			return AddBoundHandler(reg, MethodUserGet, false, handler.Get)
		},
		func() error {
			return AddBoundHandler(reg, MethodUserUpdateProfile, false, handler.UpdateProfile)
		},
		func() error {
			return AddBoundHandler(reg, MethodUserSuspend, false, handler.Suspend)
		},
		func() error {
			return AddBoundHandler(reg, MethodUserClose, false, handler.Close)
		},
	)
}
