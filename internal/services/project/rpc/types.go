package rpc

import (
	"time"

	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/cluster"
	projectdomain "github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/project"
)

type ProjectView struct {
	ID         string     `json:"id"`
	LocationID string     `json:"locationId"`
	Root       string     `json:"root"`
	Title      string     `json:"title,omitempty"`
	Status     string     `json:"status"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
	UpdatedAt  *time.Time `json:"updatedAt,omitempty"`
}

func NewProjectView(p projectdomain.Project) ProjectView {
	return ProjectView{
		ID:         string(p.ID()),
		LocationID: string(p.LocationID()),
		Root:       p.Root(),
		Title:      p.Title(),
		Status:     string(p.Status()),
		CreatedAt:  cloneTime(p.CreatedAt()),
		UpdatedAt:  cloneTime(p.UpdatedAt()),
	}
}

type ProjectGetParams struct {
	ProjectID string `json:"projectId"`
}

type ProjectGetResult struct {
	Project ProjectView `json:"project"`
}

type ProjectCreateParams struct {
	LocationID string `json:"locationId,omitempty"`
	Root       string `json:"root"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
}

type ProjectCreateResult struct {
	Project ProjectView `json:"project"`
}

type ProjectSaveParams struct {
	ProjectID  string `json:"projectId"`
	LocationID string `json:"locationId,omitempty"`
	Root       string `json:"root"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
}

type ProjectSaveResult struct {
	Project ProjectView `json:"project"`
}

type ProjectArchiveParams struct {
	ProjectID string `json:"projectId"`
}

type ProjectArchiveResult struct {
	Project ProjectView `json:"project"`
}

type ProjectDeleteParams struct {
	ProjectID string `json:"projectId"`
}

type ProjectListParams struct {
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Offset int    `json:"offset,omitempty"`
}

type ProjectListResult struct {
	Projects []ProjectView `json:"projects"`
}

type ProjectSpaceListParams struct {
	ProjectID string `json:"projectId"`
}

type ProjectSpaceView struct {
	ProjectID string            `json:"projectId"`
	SpaceID   string            `json:"spaceId"`
	Status    string            `json:"status"`
	SortOrder int               `json:"sortOrder"`
	Pinned    bool              `json:"pinned"`
	Title     string            `json:"title,omitempty"`
	SpaceOpen bool              `json:"spaceOpen"`
	Members   []SpaceMemberView `json:"members,omitempty"`
}

type SpaceMemberView struct {
	MemberID string `json:"memberId"`
	Label    string `json:"label,omitempty"`
}

func NewProjectSpaceView(space projectapp.ProjectSpaceView) ProjectSpaceView {
	members := make([]SpaceMemberView, 0, len(space.Members))
	for _, member := range space.Members {
		members = append(members, SpaceMemberView{
			MemberID: string(member.MemberID),
			Label:    member.Label,
		})
	}
	return ProjectSpaceView{
		ProjectID: string(space.ProjectID),
		SpaceID:   string(space.SpaceID),
		Status:    space.Status,
		SortOrder: space.SortOrder,
		Pinned:    space.Pinned,
		Title:     space.Title,
		SpaceOpen: space.SpaceOpen,
		Members:   members,
	}
}

type ProjectSpaceListResult struct {
	Spaces []ProjectSpaceView `json:"spaces"`
}

type ClusterListParams struct {
	ProjectID string `json:"projectId"`
}

type ClusterSaveParams struct {
	ClusterID string `json:"clusterId"`
	ProjectID string `json:"projectId"`
	Name      string `json:"name"`
	Status    string `json:"status,omitempty"`
}

type ClusterSaveResult struct {
	Cluster ClusterView `json:"cluster"`
}

type ClusterView struct {
	ID        string             `json:"id"`
	ProjectID string             `json:"projectId"`
	Name      string             `json:"name"`
	Status    string             `json:"status"`
	Spaces    []ClusterSpaceView `json:"spaces"`
}

type ClusterSpaceView struct {
	ClusterID string `json:"clusterId"`
	SpaceID   string `json:"spaceId"`
	SortOrder int    `json:"sortOrder"`
	Pinned    bool   `json:"pinned"`
}

func NewClusterView(view projectapp.ClusterView) ClusterView {
	spaces := make([]ClusterSpaceView, 0, len(view.Spaces))
	for _, ref := range view.Spaces {
		spaces = append(spaces, NewClusterSpaceView(ref))
	}
	return ClusterView{
		ID:        string(view.ID),
		ProjectID: string(view.ProjectID),
		Name:      view.Name,
		Status:    string(view.Status),
		Spaces:    spaces,
	}
}

func NewClusterSpaceView(ref cluster.SpaceRefRecord) ClusterSpaceView {
	return ClusterSpaceView{
		ClusterID: string(ref.ClusterID),
		SpaceID:   string(ref.SpaceID),
		SortOrder: ref.SortOrder,
		Pinned:    ref.Pinned,
	}
}

type ClusterListResult struct {
	Clusters []ClusterView `json:"clusters"`
}

type ClusterSpaceSaveParams struct {
	ClusterID string `json:"clusterId"`
	ProjectID string `json:"projectId"`
	SpaceID   string `json:"spaceId"`
	SortOrder int    `json:"sortOrder"`
	Pinned    bool   `json:"pinned"`
}

type ClusterSpaceSaveResult struct {
	Space ClusterSpaceView `json:"space"`
}

type ClusterSpaceRemoveParams struct {
	ClusterID string `json:"clusterId"`
	ProjectID string `json:"projectId"`
	SpaceID   string `json:"spaceId"`
}

func cloneTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	cp := t
	return &cp
}
