package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	decisionapp "github.com/tinoosan/agen8-mcp-server/internal/services/decision/app"
	decisionrpc "github.com/tinoosan/agen8-mcp-server/internal/services/decision/rpc"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	userapp "github.com/tinoosan/agen8-mcp-server/internal/services/user/app"
)

const (
	MethodDecisionCreate = "decision.create"
	MethodDecisionGet    = "decision.get"
	MethodDecisionDelete = "decision.delete"
	MethodDecisionList   = "decision.list"
	MethodDecisionCount  = "decision.count"
	MethodDecisionStats  = "decision.stats"
	MethodDecisionExport = "decision.export"
)

func RegisterDecision(reg *Registry, decisionSvc *decisionapp.Service, memberDisplay decisionrpc.MemberDisplayLookup, userSvc *userapp.Service) error {
	if decisionSvc == nil {
		return fmt.Errorf("decision service is required")
	}
	handler := decisionrpc.NewHandler(decisionSvc)
	if memberDisplay != nil {
		handler.SetMemberDisplayLookup(memberDisplay)
	}
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodDecisionCreate, false, withDecisionIdentity(handler.Create))
		},
		func() error {
			return AddBoundHandler(reg, MethodDecisionGet, false, withDecisionIdentity(handler.Get))
		},
		func() error {
			return AddBoundHandler(reg, MethodDecisionDelete, false, withDecisionIdentity(handler.Delete))
		},
		func() error {
			return AddBoundHandler(reg, MethodDecisionList, false, withDecisionIdentity(handler.List))
		},
		func() error {
			return AddBoundHandler(reg, MethodDecisionCount, false, withDecisionIdentity(handler.Count))
		},
		func() error {
			return AddBoundHandler(reg, MethodDecisionStats, false, withDecisionIdentity(handler.Stats))
		},
		func() error {
			return AddBoundHandler(reg, MethodDecisionExport, false, withDecisionIdentity(handler.Export))
		},
	)
}

func withDecisionIdentity[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
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
