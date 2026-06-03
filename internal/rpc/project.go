package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"

	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	projectrpc "github.com/tinoosan/agen8-mcp-server/internal/services/project/rpc"
)

const (
	MethodProjectGet           = "project.get"
	MethodProjectCreate        = "project.create"
	MethodProjectSave          = "project.save"
	MethodProjectArchive       = "project.archive"
	MethodProjectDelete        = "project.delete"
	MethodProjectList          = "project.list"
	MethodProjectSpaceList     = "project.space.list"
	MethodProjectClusterSave   = "project.cluster.save"
	MethodProjectClusterList   = "project.cluster.list"
	MethodProjectClusterSpace  = "project.cluster.space.save"
	MethodProjectClusterRemove = "project.cluster.space.remove"
)

func withProjectCaller[Params any, Result any](fn func(context.Context, Params) (Result, error)) func(context.Context, Params) (Result, error) {
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

func RegisterProject(reg *Registry, projectSvc *projectapp.Service) error {
	if projectSvc == nil {
		return fmt.Errorf("project service is required")
	}
	handler := projectrpc.NewHandler(projectSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodProjectGet, false, handler.ProjectGet)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectCreate, false, handler.ProjectCreate)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectSave, false, handler.ProjectSave)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectArchive, false, handler.ProjectArchive)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectDelete, false, handler.ProjectDelete)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectList, true, handler.ProjectList)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectSpaceList, false, withProjectCaller(handler.ProjectSpaceList))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectClusterSave, false, handler.ClusterSave)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectClusterList, false, handler.ClusterList)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectClusterSpace, false, handler.ClusterSpaceSave)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectClusterRemove, false, handler.ClusterSpaceRemove)
		},
	)
}
