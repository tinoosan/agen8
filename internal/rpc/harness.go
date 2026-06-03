package rpc

import (
	"context"
	"fmt"
	"strings"

	harnessapp "github.com/tinoosan/agen8-mcp-server/internal/services/harness/app"
	harnessrpc "github.com/tinoosan/agen8-mcp-server/internal/services/harness/rpc"
	"github.com/tinoosan/agen8-mcp-server/pkg/protocol"
)

const (
	MethodHarnessConfigOptions = "harness.configOptions"
	MethodHarnessSessionGet    = "harness.session.get"
	MethodHarnessSessionList   = "harness.session.list"
	MethodHarnessRunList       = "harness.run.list"
)

func RegisterHarness(reg *Registry, harnessSvc *harnessapp.Service) error {
	if harnessSvc == nil {
		return fmt.Errorf("harness service is required")
	}
	handler := harnessrpc.MustNewHandler(harnessSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodHarnessConfigOptions, true, withAuthenticatedCaller(handler.ConfigOptions))
		},
		func() error {
			return AddBoundHandler(reg, MethodHarnessSessionGet, false, withAuthenticatedCaller(handler.SessionGet))
		},
		func() error {
			return AddBoundHandler(reg, MethodHarnessSessionList, false, withAuthenticatedCaller(handler.SessionList))
		},
		func() error {
			return AddBoundHandler(reg, MethodHarnessRunList, false, withAuthenticatedCaller(handler.RunList))
		},
		func() error {
			return AddBoundHandler(reg, protocol.MethodTurnCancel, false, withHarnessIdentity(handler.TurnCancel))
		},
	)
}

func withAuthenticatedCaller[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
	return func(ctx context.Context, params Params) (Result, error) {
		if _, err := RequireIdentity(ctx); err != nil {
			var zero Result
			return zero, err
		}
		return fn(ctx, params)
	}
}

func withHarnessIdentity[Params any, Result any](fn func(context.Context, string, Params) (Result, error)) func(context.Context, Params) (Result, error) {
	return func(ctx context.Context, params Params) (Result, error) {
		identity, err := RequireIdentity(ctx)
		if err != nil {
			var zero Result
			return zero, err
		}
		requestedBy := strings.TrimSpace(identity.MemberID)
		if requestedBy == "" {
			requestedBy = strings.TrimSpace(identity.UserID)
		}
		return fn(ctx, requestedBy, params)
	}
}
