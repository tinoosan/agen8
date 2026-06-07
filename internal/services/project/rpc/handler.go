package rpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
)

type Handler struct {
	svc *projectapp.Service
}

func NewHandler(svc *projectapp.Service) *Handler {
	if svc == nil {
		panic("project RPC handler requires project service")
	}
	return &Handler{svc: svc}
}

func (h *Handler) ProjectGet(ctx context.Context, p ProjectGetParams) (ProjectGetResult, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return ProjectGetResult{}, err
	}
	project, err := h.svc.GetProject(ctx, projectID)
	if err != nil {
		return ProjectGetResult{}, err
	}
	return ProjectGetResult{Project: NewProjectView(project)}, nil
}

func (h *Handler) ProjectCreate(ctx context.Context, p ProjectCreateParams) (ProjectCreateResult, error) {
	root := strings.TrimSpace(p.Root)
	if root == "" {
		return ProjectCreateResult{}, invalidParams("root is required")
	}
	project, err := h.svc.CreateProject(ctx, projectapp.CreateProjectInput{
		LocationID: types.LocationID(strings.TrimSpace(p.LocationID)),
		Root:       root,
		Title:      strings.TrimSpace(p.Title),
		Status:     projectdomain.Status(strings.TrimSpace(p.Status)),
	})
	if err != nil {
		return ProjectCreateResult{}, internalError("create project", err)
	}
	return ProjectCreateResult{Project: NewProjectView(project)}, nil
}

func (h *Handler) ProjectSave(ctx context.Context, p ProjectSaveParams) (ProjectSaveResult, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return ProjectSaveResult{}, err
	}
	root := strings.TrimSpace(p.Root)
	if root == "" {
		return ProjectSaveResult{}, invalidParams("root is required")
	}
	project, err := h.svc.SaveProject(ctx, projectapp.SaveProjectInput{
		ID:         projectID,
		LocationID: types.LocationID(strings.TrimSpace(p.LocationID)),
		Root:       root,
		Title:      strings.TrimSpace(p.Title),
		Status:     projectdomain.Status(strings.TrimSpace(p.Status)),
	})
	if err != nil {
		return ProjectSaveResult{}, internalError("save project", err)
	}
	return ProjectSaveResult{Project: NewProjectView(project)}, nil
}

func (h *Handler) ProjectList(ctx context.Context, p ProjectListParams) (ProjectListResult, error) {
	projects, err := h.svc.ListProjects(ctx, projectdomain.Filter{
		Status: projectdomain.Status(strings.TrimSpace(p.Status)),
		Limit:  p.Limit,
		Offset: p.Offset,
	})
	if err != nil {
		return ProjectListResult{}, err
	}
	views := make([]ProjectView, 0, len(projects))
	for _, project := range projects {
		views = append(views, NewProjectView(project))
	}
	return ProjectListResult{Projects: views}, nil
}

func (h *Handler) ProjectArchive(ctx context.Context, p ProjectArchiveParams) (ProjectArchiveResult, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return ProjectArchiveResult{}, err
	}
	project, err := h.svc.ArchiveProject(ctx, projectID)
	if err != nil {
		return ProjectArchiveResult{}, internalError("archive project", err)
	}
	return ProjectArchiveResult{Project: NewProjectView(project)}, nil
}

func (h *Handler) ProjectDelete(ctx context.Context, p ProjectDeleteParams) (struct{}, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return struct{}{}, err
	}
	if err := h.svc.DeleteProject(ctx, projectID); err != nil {
		return struct{}{}, internalError("delete project", err)
	}
	return struct{}{}, nil
}

func (h *Handler) LinkTokenCreate(ctx context.Context, p LinkTokenCreateParams) (LinkTokenCreateResult, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return LinkTokenCreateResult{}, err
	}
	issued, err := h.svc.CreateLinkToken(ctx, projectapp.CreateLinkTokenInput{
		ProjectID: projectID,
		Label:     strings.TrimSpace(p.Label),
	})
	if err != nil {
		return LinkTokenCreateResult{}, internalError("create link token", err)
	}
	return LinkTokenCreateResult{
		ID:          issued.ID,
		Prefix:      issued.Prefix,
		Token:       issued.Token,
		ProjectID:   issued.ProjectID,
		WorkspaceID: issued.WorkspaceID,
		Label:       issued.Label,
		ExpiresAt:   issued.ExpiresAt,
		CreatedAt:   cloneTime(issued.CreatedAt),
	}, nil
}

func (h *Handler) MemberRegister(ctx context.Context, p MemberRegisterParams) (MemberRegisterResult, error) {
	projectID := strings.TrimSpace(p.ProjectID)
	if projectID == "" {
		return MemberRegisterResult{}, invalidParams("projectId is required")
	}
	rosterMember := member.Record{
		ProjectID:   projectID,
		DisplayName: strings.TrimSpace(p.DisplayName),
		MemberType:  member.TypeCoordinator,
		HarnessKind: "web",
	}
	result, err := h.svc.RegisterMember(ctx, rosterMember)
	if err != nil {
		return MemberRegisterResult{}, err
	}
	return MemberRegisterResult{
		Member:            NewMemberView(result.Member),
		GrantedMemberType: result.GrantedMemberType,
	}, nil
}

func (h *Handler) MemberGet(ctx context.Context, p MemberGetParams) (MemberGetResult, error) {
	id, err := requireMemberID(p.MemberID)
	if err != nil {
		return MemberGetResult{}, err
	}
	rosterMember, err := h.svc.GetMember(ctx, id)
	if err != nil {
		return MemberGetResult{}, err
	}
	return MemberGetResult{Member: NewMemberView(rosterMember)}, nil
}

func (h *Handler) MemberList(ctx context.Context, p MemberListParams) (MemberListResult, error) {
	filter := member.Filter{
		ProjectID:      strings.TrimSpace(p.ProjectID),
		UserID:         strings.TrimSpace(p.UserID),
		MemberType:     strings.TrimSpace(p.MemberType),
		LifecycleState: strings.TrimSpace(p.LifecycleState),
		Limit:          p.Limit,
		Offset:         p.Offset,
	}
	members, err := h.svc.ListMembers(ctx, filter)
	if err != nil {
		return MemberListResult{}, err
	}
	views := make([]MemberView, 0, len(members))
	for _, rosterMember := range members {
		views = append(views, NewMemberView(rosterMember))
	}
	return MemberListResult{Members: views}, nil
}

func (h *Handler) MemberUpdate(ctx context.Context, p MemberUpdateParams) (MemberUpdateResult, error) {
	id, err := requireMemberID(p.MemberID)
	if err != nil {
		return MemberUpdateResult{}, err
	}
	displayName := strings.TrimSpace(p.DisplayName)
	if displayName == "" {
		return MemberUpdateResult{}, invalidParams("displayName is required")
	}
	rosterMember, err := h.svc.UpdateMember(ctx, id, displayName)
	if err != nil {
		return MemberUpdateResult{}, err
	}
	return MemberUpdateResult{Member: NewMemberView(rosterMember)}, nil
}

func (h *Handler) MemberRemove(ctx context.Context, p MemberRemoveParams) (MemberRemoveResult, error) {
	id, err := requireMemberID(p.MemberID)
	if err != nil {
		return MemberRemoveResult{}, err
	}
	rosterMember, err := h.svc.RemoveMember(ctx, id)
	if err != nil {
		return MemberRemoveResult{}, err
	}
	return MemberRemoveResult{Member: NewMemberView(rosterMember)}, nil
}

func requireMemberID(raw string) (member.ID, error) {
	id := member.ID(strings.TrimSpace(raw))
	if id == "" {
		return "", invalidParams("memberId is required")
	}
	return id, nil
}

func requireProjectID(value string) (types.ProjectID, error) {
	id := types.ProjectID(strings.TrimSpace(value))
	if id == "" {
		return "", invalidParams("projectId is required")
	}
	return id, nil
}

func invalidParams(message string) error {
	return rpcError{code: -32602, message: strings.TrimSpace(message)}
}

func internalError(action string, err error) error {
	return fmt.Errorf("%s: %w", action, err)
}

type rpcError struct {
	code    int
	message string
}

func (e rpcError) Error() string {
	return e.message
}

func (e rpcError) RPCCode() int {
	return e.code
}
