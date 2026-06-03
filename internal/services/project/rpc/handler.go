package rpc

import (
	"context"
	"fmt"
	"strings"

	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"

	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
	"github.com/tinoosan/agen8-mcp-server/pkg/types"
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

func (h *Handler) ProjectSpaceList(ctx context.Context, p ProjectSpaceListParams) (ProjectSpaceListResult, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return ProjectSpaceListResult{}, err
	}
	spaces, err := h.svc.ListProjectSpaces(ctx, projectID)
	if err != nil {
		return ProjectSpaceListResult{}, err
	}
	views := make([]ProjectSpaceView, 0, len(spaces))
	for _, space := range spaces {
		views = append(views, NewProjectSpaceView(space))
	}
	return ProjectSpaceListResult{Spaces: views}, nil
}

func (h *Handler) ClusterSave(ctx context.Context, p ClusterSaveParams) (ClusterSaveResult, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return ClusterSaveResult{}, err
	}
	clusterID := cluster.ID(strings.TrimSpace(p.ClusterID))
	if clusterID == "" {
		return ClusterSaveResult{}, invalidParams("clusterId is required")
	}
	view, err := h.svc.SaveCluster(ctx, projectapp.SaveClusterInput{
		ID:        clusterID,
		ProjectID: projectID,
		Name:      strings.TrimSpace(p.Name),
		Status:    cluster.Status(strings.TrimSpace(p.Status)),
	})
	if err != nil {
		return ClusterSaveResult{}, internalError("save project cluster", err)
	}
	return ClusterSaveResult{Cluster: NewClusterView(view)}, nil
}

func (h *Handler) ClusterList(ctx context.Context, p ClusterListParams) (ClusterListResult, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return ClusterListResult{}, err
	}
	clusters, err := h.svc.ListClusters(ctx, projectID)
	if err != nil {
		return ClusterListResult{}, err
	}
	views := make([]ClusterView, 0, len(clusters))
	for _, cluster := range clusters {
		views = append(views, NewClusterView(cluster))
	}
	return ClusterListResult{Clusters: views}, nil
}

func (h *Handler) ClusterSpaceSave(ctx context.Context, p ClusterSpaceSaveParams) (ClusterSpaceSaveResult, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return ClusterSpaceSaveResult{}, err
	}
	clusterID := cluster.ID(strings.TrimSpace(p.ClusterID))
	if clusterID == "" {
		return ClusterSpaceSaveResult{}, invalidParams("clusterId is required")
	}
	spaceID := spacedomain.SpaceID(strings.TrimSpace(p.SpaceID))
	if spaceID == "" {
		return ClusterSpaceSaveResult{}, invalidParams("spaceId is required")
	}
	ref, err := h.svc.SaveClusterSpace(ctx, projectapp.SaveClusterSpaceInput{
		ClusterID: clusterID,
		ProjectID: projectID,
		SpaceID:   spaceID,
		SortOrder: p.SortOrder,
		Pinned:    p.Pinned,
	})
	if err != nil {
		return ClusterSpaceSaveResult{}, internalError("save project cluster space", err)
	}
	return ClusterSpaceSaveResult{Space: NewClusterSpaceView(ref)}, nil
}

func (h *Handler) ClusterSpaceRemove(ctx context.Context, p ClusterSpaceRemoveParams) (struct{}, error) {
	projectID, err := requireProjectID(p.ProjectID)
	if err != nil {
		return struct{}{}, err
	}
	clusterID := cluster.ID(strings.TrimSpace(p.ClusterID))
	if clusterID == "" {
		return struct{}{}, invalidParams("clusterId is required")
	}
	spaceID := spacedomain.SpaceID(strings.TrimSpace(p.SpaceID))
	if spaceID == "" {
		return struct{}{}, invalidParams("spaceId is required")
	}
	if err := h.svc.RemoveClusterSpace(ctx, projectID, clusterID, spaceID); err != nil {
		return struct{}{}, internalError("remove project cluster space", err)
	}
	return struct{}{}, nil
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
