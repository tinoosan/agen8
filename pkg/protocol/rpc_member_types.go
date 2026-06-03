package protocol

import "time"

type MemberView struct {
	ID             string     `json:"id"`
	UserID         string     `json:"userId,omitempty"`
	ProjectID      string     `json:"projectId,omitempty"`
	SpaceID        string     `json:"spaceId"`
	ChannelID      string     `json:"channelId"`
	DisplayName    string     `json:"displayName"`
	MemberType     string     `json:"memberType"`
	LifecycleState string     `json:"lifecycleState"`
	HarnessKind    string     `json:"harnessKind"`
	Model          string     `json:"model"`
	Effort         string     `json:"effort"`
	PermissionMode string     `json:"harnessPermissionMode,omitempty"`
	ConfigRef      string     `json:"harnessConfigRef,omitempty"`
	RegisteredAt   time.Time  `json:"registeredAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	LastSeenAt     *time.Time `json:"lastSeenAt,omitempty"`
}

type MemberRegisterParams struct {
	SpaceID             string `json:"spaceId"`
	ProjectID           string `json:"projectId,omitempty"`
	DisplayName         string `json:"displayName,omitempty"`
	RequestedMemberType string `json:"requestedMemberType"`
	HarnessKind         string `json:"harnessKind"`
	Model               string `json:"model"`
	Effort              string `json:"effort"`
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
