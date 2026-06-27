package rpc

import (
	"time"

	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
	projectdomain "github.com/tinoosan/agen8/internal/services/project/domain/project"
)

type CustomizationView struct {
	Icon  string `json:"icon,omitempty"`
	Color string `json:"color,omitempty"`
}

type ProjectView struct {
	ID            string             `json:"id"`
	LocationID    string             `json:"locationId"`
	Root          string             `json:"root"`
	Title         string             `json:"title,omitempty"`
	Status        string             `json:"status"`
	Customization *CustomizationView `json:"customization,omitempty"`
	CreatedAt     *time.Time         `json:"createdAt,omitempty"`
	UpdatedAt     *time.Time         `json:"updatedAt,omitempty"`
}

func NewProjectView(p projectdomain.Project) ProjectView {
	return ProjectView{
		ID:            string(p.ID()),
		LocationID:    string(p.LocationID()),
		Root:          p.Root(),
		Title:         p.Title(),
		Status:        string(p.Status()),
		Customization: newCustomizationView(p.Customization()),
		CreatedAt:     cloneTime(p.CreatedAt()),
		UpdatedAt:     cloneTime(p.UpdatedAt()),
	}
}

func newCustomizationView(c *projectdomain.Customization) *CustomizationView {
	if c == nil {
		return nil
	}
	return &CustomizationView{Icon: c.Icon, Color: c.Color}
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
	// HooksInstalled reports whether the daemon auto-provisioned the attention
	// hooks for the new project (nil when no provisioner is wired).
	HooksInstalled *bool `json:"hooksInstalled,omitempty"`
}

type ProjectClaudeMCPConfigureParams struct {
	ProjectID string `json:"projectId"`
}

type ProjectClaudeMCPConfigureResult struct {
	ProjectID  string `json:"projectId"`
	Installed  bool   `json:"installed"`
	Path       string `json:"path,omitempty"`
	ServerName string `json:"serverName,omitempty"`
	URL        string `json:"url,omitempty"`
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

// ProjectUpdateParams edits an owned project's user-facing fields. Title and
// Customization are pointers so an omitted field is left unchanged on the
// server (nil = "leave alone", present = "set to this").
type ProjectUpdateParams struct {
	ProjectID     string             `json:"projectId"`
	Title         *string            `json:"title,omitempty"`
	Customization *CustomizationView `json:"customization,omitempty"`
}

type ProjectUpdateResult struct {
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

type ProjectLinkTokenListParams struct {
	ProjectID string `json:"projectId"`
}

// LinkTokenSummaryView is the safe-to-display view of a minted link token: it
// carries the prefix and lifecycle timestamps but never the raw token or hash.
// Status collapses the (Active, RevokedAt, ExpiresAt) triple the server already
// evaluated into a single word the UI can render directly.
type LinkTokenSummaryView struct {
	ID          string     `json:"id"`
	Prefix      string     `json:"prefix"`
	ProjectID   string     `json:"projectId"`
	WorkspaceID string     `json:"workspaceId,omitempty"`
	Label       string     `json:"label,omitempty"`
	Status      string     `json:"status"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
}

func newLinkTokenSummaryView(s projectapp.LinkTokenSummary) LinkTokenSummaryView {
	status := "active"
	if !s.Active {
		// A revoked token has an explicit RevokedAt; anything else inactive aged
		// past its expiry.
		if s.RevokedAt != nil {
			status = "revoked"
		} else {
			status = "expired"
		}
	}
	return LinkTokenSummaryView{
		ID:          s.ID,
		Prefix:      s.Prefix,
		ProjectID:   s.ProjectID,
		WorkspaceID: s.WorkspaceID,
		Label:       s.Label,
		Status:      status,
		ExpiresAt:   cloneTimePtr(s.ExpiresAt),
		RevokedAt:   cloneTimePtr(s.RevokedAt),
		CreatedAt:   cloneTime(s.CreatedAt),
	}
}

type ProjectLinkTokenListResult struct {
	Tokens []LinkTokenSummaryView `json:"tokens"`
}

type ProjectLinkTokenRevokeParams struct {
	ProjectID string `json:"projectId"`
	TokenID   string `json:"tokenId"`
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
		// Canonical so legacy "claude" / "claude-cli" rows reach the web as
		// "claude-code" (no migration needed); consumers match exact strings.
		HarnessKind:    member.CanonicalHarnessKind(m.HarnessKind),
		LifecycleState: m.LifecycleState,
		RegisteredAt:   m.RegisteredAt,
		UpdatedAt:      m.UpdatedAt,
		LastSeenAt:     m.LastSeenAt,
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

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	cp := *t
	return &cp
}
