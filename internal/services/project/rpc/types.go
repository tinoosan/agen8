package rpc

import (
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
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

type LinkTokenCreateParams struct {
	ProjectID string `json:"projectId"`
	Label     string `json:"label,omitempty"`
}

// LinkTokenCreateResult carries the minted wlt_ token. Token is the raw secret,
// returned once at mint time; the client must surface it immediately and never
// expect it again. Prefix/ID are safe to persist for display and later revocation.
type LinkTokenCreateResult struct {
	ID          string     `json:"id"`
	Prefix      string     `json:"prefix"`
	Token       string     `json:"token"`
	ProjectID   string     `json:"projectId"`
	WorkspaceID string     `json:"workspaceId,omitempty"`
	Label       string     `json:"label,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
}

type MemberView struct {
	ID               string     `json:"id"`
	UserID           string     `json:"userId,omitempty"`
	ProjectID        string     `json:"projectId"`
	NativeSessionRef string     `json:"nativeSessionRef,omitempty"`
	ChannelID        string     `json:"channelId,omitempty"`
	DisplayName      string     `json:"displayName,omitempty"`
	MemberType       string     `json:"memberType"`
	HarnessKind      string     `json:"harnessKind,omitempty"`
	LifecycleState   string     `json:"lifecycleState"`
	RegisteredAt     time.Time  `json:"registeredAt,omitempty"`
	UpdatedAt        time.Time  `json:"updatedAt,omitempty"`
	LastSeenAt       *time.Time `json:"lastSeenAt,omitempty"`
}

func NewMemberView(m member.Record) MemberView {
	return MemberView{
		ID:               string(m.ID),
		UserID:           m.UserID,
		ProjectID:        string(m.ProjectID),
		NativeSessionRef: m.NativeSessionRef,
		ChannelID:        string(m.ChannelID),
		DisplayName:      m.DisplayName,
		MemberType:       m.MemberType,
		HarnessKind:      m.HarnessKind,
		LifecycleState:   m.LifecycleState,
		RegisteredAt:     m.RegisteredAt,
		UpdatedAt:        m.UpdatedAt,
		LastSeenAt:       m.LastSeenAt,
	}
}

type MemberRegisterParams struct {
	ProjectID   string `json:"projectId"`
	DisplayName string `json:"displayName,omitempty"`
}

type MemberRegisterResult struct {
	Member            MemberView `json:"member"`
	GrantedMemberType string     `json:"grantedMemberType"`
}

type MemberGetParams struct {
	MemberID string `json:"memberId"`
}

type MemberGetResult struct {
	Member MemberView `json:"member"`
}

type MemberListParams struct {
	ProjectID      string `json:"projectId,omitempty"`
	UserID         string `json:"userId,omitempty"`
	MemberType     string `json:"memberType,omitempty"`
	LifecycleState string `json:"lifecycleState,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	Offset         int    `json:"offset,omitempty"`
}

type MemberListResult struct {
	Members []MemberView `json:"members"`
}

type MemberUpdateParams struct {
	MemberID    string `json:"memberId"`
	DisplayName string `json:"displayName"`
}

type MemberUpdateResult struct {
	Member MemberView `json:"member"`
}

type MemberRemoveParams struct {
	MemberID string `json:"memberId"`
}

type MemberRemoveResult struct {
	Member MemberView `json:"member"`
}

func cloneTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	cp := t
	return &cp
}
