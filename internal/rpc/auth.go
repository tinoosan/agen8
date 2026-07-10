package rpc

import (
	"context"
	"fmt"

	authapp "github.com/tinoosan/agen8/internal/services/auth/app"
	authrpc "github.com/tinoosan/agen8/internal/services/auth/rpc"
)

const (
	MethodAuthStatus       = "auth.status"
	MethodAuthLogin        = "auth.login"
	MethodAuthLogout       = "auth.logout"
	MethodAuthAPIKeyList   = "auth.apiKey.list"   // #nosec G101 -- RPC method name, not a credential.
	MethodAuthAPIKeyCreate = "auth.apiKey.create" // #nosec G101 -- RPC method name, not a credential.
	MethodAuthAPIKeyRevoke = "auth.apiKey.revoke" // #nosec G101 -- RPC method name, not a credential.
)

func RegisterAuth(reg *Registry, authSvc *authapp.Service) error {
	if authSvc == nil {
		return fmt.Errorf("auth service is required")
	}
	handler := authrpc.NewHandler(authSvc, func(ctx context.Context) (authrpc.Identity, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			return authrpc.Identity{}, err
		}
		return authrpc.Identity{
			UserID: identity.UserID,
			Role:   identity.Role,
		}, nil
	})
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodAuthStatus, true, handler.AuthStatus)
		},
		func() error {
			return AddBoundHandler(reg, MethodAuthLogin, false, handler.Login)
		},
		func() error {
			return AddBoundHandler(reg, MethodAuthLogout, false, handler.Logout)
		},
		func() error {
			return AddBoundHandler(reg, MethodAuthAPIKeyList, true, handler.ListAPIKeys)
		},
		func() error {
			return AddBoundHandler(reg, MethodAuthAPIKeyCreate, false, handler.CreateAPIKey)
		},
		func() error {
			return AddBoundHandler(reg, MethodAuthAPIKeyRevoke, false, handler.RevokeAPIKey)
		},
	)
}
