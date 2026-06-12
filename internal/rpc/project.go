package rpc

import (
	"context"
	"fmt"

	"github.com/tinoosan/agen8/internal/caller"

	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	projectrpc "github.com/tinoosan/agen8/internal/services/project/rpc"
)

const (
	MethodProjectGet     = "project.get"
	MethodProjectCreate  = "project.create"
	MethodProjectSave    = "project.save"
	MethodProjectUpdate  = "project.update"
	MethodProjectArchive = "project.archive"
	MethodProjectDelete  = "project.delete"
	MethodProjectList    = "project.list"

	MethodProjectLinkTokenCreate = "project.linkToken.create"
	MethodProjectLinkTokenList   = "project.linkToken.list"
	MethodProjectLinkTokenRevoke = "project.linkToken.revoke"

	MethodProjectMemberRegister = "project.member.register"
	MethodProjectMemberGet      = "project.member.get"
	MethodProjectMemberList     = "project.member.list"
	MethodProjectMemberUpdate   = "project.member.update"
	MethodProjectMemberRemove   = "project.member.remove"
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
			MemberID: identity.MemberID,
			Role:     identity.Role,
		})
		return fn(ctx, params)
	}
}

// PostProjectCreate runs after a successful project.create — the daemon wires
// hook auto-provisioning here so the project service stays free of auth/hook
// dependencies. Returns whether hooks were installed; must never error.
type PostProjectCreate func(ctx context.Context, userID, projectTitle, root string) bool

func RegisterProject(reg *Registry, projectSvc *projectapp.Service, postCreate PostProjectCreate) error {
	if projectSvc == nil {
		return fmt.Errorf("project service is required")
	}
	handler := projectrpc.NewHandler(projectSvc)
	return RegisterHandlers(
		func() error {
			return AddBoundHandler(reg, MethodProjectGet, false, handler.ProjectGet)
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectCreate, false, withProjectCaller(func(ctx context.Context, p projectrpc.ProjectCreateParams) (projectrpc.ProjectCreateResult, error) {
				res, err := handler.ProjectCreate(ctx, p)
				if err != nil || postCreate == nil {
					return res, err
				}
				identity, idErr := RequireIdentity(ctx)
				if idErr != nil {
					return res, nil
				}
				installed := postCreate(ctx, identity.UserID, res.Project.Title, res.Project.Root)
				res.HooksInstalled = &installed
				return res, nil
			}))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectSave, false, withProjectCaller(handler.ProjectSave))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectUpdate, false, withProjectCaller(handler.ProjectUpdate))
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
			return AddBoundHandler(reg, MethodProjectLinkTokenCreate, false, withProjectCaller(handler.LinkTokenCreate))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectLinkTokenList, false, withProjectCaller(handler.LinkTokenList))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectLinkTokenRevoke, false, withProjectCaller(handler.LinkTokenRevoke))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectMemberRegister, false, withProjectCaller(handler.MemberRegister))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectMemberGet, false, withProjectCaller(handler.MemberGet))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectMemberList, false, withProjectCaller(handler.MemberList))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectMemberUpdate, false, withProjectCaller(handler.MemberUpdate))
		},
		func() error {
			return AddBoundHandler(reg, MethodProjectMemberRemove, false, withProjectCaller(handler.MemberRemove))
		},
	)
}
