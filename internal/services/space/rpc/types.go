package rpc

import (
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

// SpaceView is the space service RPC read model.
type SpaceView struct {
	ID            string                     `json:"id"`
	ProjectID     string                     `json:"projectId,omitempty"`
	UserID        string                     `json:"userId,omitempty"`
	Title         string                     `json:"title,omitempty"`
	Status        string                     `json:"status,omitempty"`
	PlanMode      string                     `json:"planMode,omitempty"`
	Customization *domain.SpaceCustomization `json:"customization,omitempty"`
	CreatedAt     *time.Time                 `json:"createdAt,omitempty"`
	UpdatedAt     *time.Time                 `json:"updatedAt,omitempty"`
}

func NewSpaceView(space domain.SpaceRecord) SpaceView {
	return SpaceView{
		ID:            string(space.ID),
		ProjectID:     string(space.ProjectID),
		UserID:        space.UserID,
		Title:         space.Title,
		Status:        space.Status,
		PlanMode:      space.PlanMode,
		Customization: space.Customization,
		CreatedAt:     cloneTime(space.CreatedAt),
		UpdatedAt:     cloneTime(space.UpdatedAt),
	}
}

type SpaceCreateParams struct {
	SpaceID   string `json:"spaceId,omitempty"`
	ProjectID string `json:"projectId"`
	Title     string `json:"title,omitempty"`
	PlanMode  string `json:"planMode,omitempty"`
}

type SpaceCreateResult struct {
	Space SpaceView `json:"space"`
}

type SpaceGetParams struct {
	SpaceID string `json:"spaceId"`
}

type SpaceGetResult struct {
	Space SpaceView `json:"space"`
}

type SpaceListParams struct {
	SpaceID   string `json:"spaceId,omitempty"`
	ProjectID string `json:"projectId,omitempty"`
	Status    string `json:"status,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

type SpaceListResult struct {
	Spaces     []SpaceView `json:"spaces"`
	TotalCount int         `json:"totalCount"`
}

type SpaceUpdateParams struct {
	SpaceID       string                     `json:"spaceId"`
	Title         string                     `json:"title,omitempty"`
	PlanMode      string                     `json:"planMode,omitempty"`
	Customization *domain.SpaceCustomization `json:"customization,omitempty"`
}

type SpaceUpdateResult struct {
	Space SpaceView `json:"space"`
}

type SpaceCloseParams struct {
	SpaceID string `json:"spaceId"`
}

type SpaceCloseResult struct {
	Space SpaceView `json:"space"`
}

type SpaceReopenParams struct {
	SpaceID string `json:"spaceId"`
}

type SpaceReopenResult struct {
	Space SpaceView `json:"space"`
}

type SpaceDeleteParams struct {
	SpaceID string `json:"spaceId"`
}

type SpaceDeleteResult struct {
	SpaceID string `json:"spaceId"`
}

type MemberView struct {
	ID             string     `json:"id"`
	UserID         string     `json:"userId,omitempty"`
	ProjectID      string     `json:"projectId,omitempty"`
	SpaceID        string     `json:"spaceId"`
	ChannelID      string     `json:"channelId,omitempty"`
	DisplayName    string     `json:"displayName,omitempty"`
	MemberType     string     `json:"memberType"`
	LifecycleState string     `json:"lifecycleState"`
	HarnessKind    string     `json:"harnessKind,omitempty"`
	Model          string     `json:"model,omitempty"`
	Effort         string     `json:"effort,omitempty"`
	PermissionMode string     `json:"harnessPermissionMode,omitempty"`
	ConfigRef      string     `json:"harnessConfigRef,omitempty"`
	RegisteredAt   time.Time  `json:"registeredAt,omitempty"`
	UpdatedAt      time.Time  `json:"updatedAt,omitempty"`
	LastSeenAt     *time.Time `json:"lastSeenAt,omitempty"`
}

func NewMemberView(member member.Record) MemberView {
	return MemberView{
		ID:             string(member.ID),
		UserID:         member.UserID,
		ProjectID:      string(member.ProjectID),
		SpaceID:        string(member.SpaceID),
		ChannelID:      string(member.ChannelID),
		DisplayName:    member.DisplayName,
		MemberType:     member.MemberType,
		LifecycleState: member.LifecycleState,
		HarnessKind:    member.HarnessKind,
		Model:          member.Model,
		Effort:         member.Effort,
		PermissionMode: member.PermissionMode,
		ConfigRef:      member.ConfigRef,
		RegisteredAt:   member.RegisteredAt,
		UpdatedAt:      member.UpdatedAt,
		LastSeenAt:     member.LastSeenAt,
	}
}

type MemberRegisterParams struct {
	SpaceID             string `json:"spaceId"`
	ProjectID           string `json:"projectId,omitempty"`
	DisplayName         string `json:"displayName,omitempty"`
	RequestedMemberType string `json:"requestedMemberType,omitempty"`
	HarnessKind         string `json:"harnessKind,omitempty"`
	Model               string `json:"model,omitempty"`
	Effort              string `json:"effort,omitempty"`
	PermissionMode      string `json:"harnessPermissionMode,omitempty"`
	ConfigRef           string `json:"harnessConfigRef,omitempty"`
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
	SpaceID        string `json:"spaceId,omitempty"`
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

type MemberUpdateConfigParams struct {
	MemberID       string `json:"memberId"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	HarnessKind    string `json:"harnessKind"`
	PermissionMode string `json:"harnessPermissionMode,omitempty"`
	ConfigRef      string `json:"harnessConfigRef,omitempty"`
}

type MemberUpdateConfigResult struct {
	Member MemberView `json:"member"`
}

type MemberRemoveParams struct {
	MemberID string `json:"memberId"`
}

type MemberRemoveResult struct {
	Member MemberView `json:"member"`
}

func cloneTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}
