package domain

import (
	"fmt"
	"time"
)

type SessionStatus string

const (
	SessionActive   SessionStatus = "active"
	SessionInactive SessionStatus = "inactive"
)

type InactiveReason string

const (
	ReasonShutdown        InactiveReason = "shutdown"
	ReasonCrashed         InactiveReason = "crashed"
	ReasonCanceled        InactiveReason = "canceled"
	ReasonConfigChanged   InactiveReason = "config_changed"
	ReasonMemberSuspended InactiveReason = "member_suspended"
	ReasonMemberRemoved   InactiveReason = "member_removed"
)

type Session struct {
	ID                      string
	ProjectID               string
	LocationID              string
	MemberID                string
	SpaceID                 string
	ChannelID               string
	DisplayName             string
	MemberType              string
	LifecycleState          string
	Status                  SessionStatus
	InactiveReason          InactiveReason
	InactiveError           string
	ActivatedAt             time.Time
	DeactivatedAt           *time.Time
	TokensIn                int64
	TokensOut               int64
	Kind                    string
	Model                   string
	Effort                  string
	PermissionMode          string
	ConfigRef               string
	Ref                     string
	Workdir                 string
	SystemPrompt            string
	MCPToken                string
	MCPServers              []string
	ClaudeChannelURL        string
	ClaudeChannelInstanceID string
	ClaudeChannelStartedAt  *time.Time
}

type RuntimeContext struct {
	ProjectID      string
	LocationID     string
	MemberID       string
	SpaceID        string
	ChannelID      string
	DisplayName    string
	MemberType     string
	LifecycleState string
	HarnessKind    string
	Model          string
	Effort         string
	PermissionMode string
	ConfigRef      string
	SessionRef     string
	Workdir        string
	SystemPrompt   string
	MCPToken       string
	MCPServers     []string
}

func NewSession(id string, runtimeContext RuntimeContext, now time.Time) (*Session, error) {
	if id == "" {
		return nil, fmt.Errorf("id is required")
	}
	memberID := runtimeContext.MemberID
	if memberID == "" {
		return nil, fmt.Errorf("memberID is required")
	}
	if runtimeContext.ProjectID == "" {
		return nil, fmt.Errorf("projectID is required")
	}
	if runtimeContext.LocationID == "" {
		return nil, fmt.Errorf("locationID is required")
	}
	spaceID := runtimeContext.SpaceID
	if spaceID == "" {
		return nil, fmt.Errorf("spaceID is required")
	}
	if runtimeContext.ChannelID == "" {
		return nil, fmt.Errorf("channelID is required")
	}
	if runtimeContext.DisplayName == "" {
		return nil, fmt.Errorf("displayName is required")
	}
	if runtimeContext.MemberType == "" {
		return nil, fmt.Errorf("memberType is required")
	}
	if runtimeContext.LifecycleState == "" {
		return nil, fmt.Errorf("lifecycleState is required")
	}
	kind := runtimeContext.HarnessKind
	if kind == "" {
		return nil, fmt.Errorf("kind is required")
	}
	model := runtimeContext.Model
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}
	effort := runtimeContext.Effort
	if effort == "" {
		return nil, fmt.Errorf("effort is required")
	}
	permissionMode := runtimeContext.PermissionMode
	if permissionMode == "" {
		permissionMode = compatibilityPermissionMode(kind)
	}
	if permissionMode == "" {
		return nil, fmt.Errorf("permissionMode is required")
	}
	if runtimeContext.SystemPrompt == "" {
		return nil, fmt.Errorf("systemPrompt is required")
	}
	if runtimeContext.Workdir == "" {
		return nil, fmt.Errorf("workdir is required")
	}
	return &Session{
		ID:             id,
		ProjectID:      runtimeContext.ProjectID,
		LocationID:     runtimeContext.LocationID,
		MemberID:       memberID,
		SpaceID:        spaceID,
		ChannelID:      runtimeContext.ChannelID,
		DisplayName:    runtimeContext.DisplayName,
		MemberType:     runtimeContext.MemberType,
		LifecycleState: runtimeContext.LifecycleState,
		Status:         SessionActive,
		ActivatedAt:    now,
		Kind:           kind,
		Model:          model,
		Effort:         effort,
		PermissionMode: permissionMode,
		ConfigRef:      runtimeContext.ConfigRef,
		Ref:            runtimeContext.SessionRef,
		Workdir:        runtimeContext.Workdir,
		SystemPrompt:   runtimeContext.SystemPrompt,
		MCPToken:       runtimeContext.MCPToken,
		MCPServers:     append([]string(nil), runtimeContext.MCPServers...),
	}, nil
}

func (s *Session) Deactivate(reason InactiveReason, errDetail string, now time.Time) error {
	if s.Status != SessionActive {
		return fmt.Errorf("cannot deactivate session %q: status is %q, not active", s.ID, s.Status)
	}
	if reason == "" {
		return fmt.Errorf("inactive reason is required")
	}
	s.Status = SessionInactive
	s.InactiveReason = reason
	s.InactiveError = errDetail
	s.DeactivatedAt = &now
	return nil
}

func (s *Session) Reactivate(now time.Time) error {
	if s.Status != SessionInactive {
		return fmt.Errorf("cannot reactivate session %q: status is %q, not inactive", s.ID, s.Status)
	}
	s.Status = SessionActive
	s.InactiveReason = ""
	s.InactiveError = ""
	s.DeactivatedAt = nil
	s.ActivatedAt = now
	return nil
}

func (s *Session) UpdateConfig(model, effort string) error {
	permissionMode := s.PermissionMode
	if permissionMode == "" {
		permissionMode = compatibilityPermissionMode(s.Kind)
	}
	return s.UpdateRuntimeConfigValues(model, effort, permissionMode, s.ConfigRef)
}

func (s *Session) UpdateRuntimeConfigValues(model, effort, permissionMode, configRef string) error {
	if s.Status != SessionActive {
		return fmt.Errorf("cannot update session %q config: status is %q, not active", s.ID, s.Status)
	}
	if model == "" {
		return fmt.Errorf("model is required")
	}
	if effort == "" {
		return fmt.Errorf("effort is required")
	}
	if permissionMode == "" {
		return fmt.Errorf("permissionMode is required")
	}
	s.Model = model
	s.Effort = effort
	s.PermissionMode = permissionMode
	s.ConfigRef = configRef
	return nil
}

func (s *Session) UpdateRuntimeContext(runtimeContext RuntimeContext) error {
	if s.Status != SessionActive {
		return fmt.Errorf("cannot update session %q runtime context: status is %q, not active", s.ID, s.Status)
	}
	if runtimeContext.MemberID != s.MemberID {
		return fmt.Errorf("runtime context memberID %q does not match session memberID %q", runtimeContext.MemberID, s.MemberID)
	}
	if runtimeContext.ProjectID != s.ProjectID {
		return fmt.Errorf("runtime context projectID %q does not match session projectID %q", runtimeContext.ProjectID, s.ProjectID)
	}
	if runtimeContext.LocationID == "" {
		return fmt.Errorf("locationID is required")
	}
	if runtimeContext.SpaceID != s.SpaceID {
		return fmt.Errorf("runtime context spaceID %q does not match session spaceID %q", runtimeContext.SpaceID, s.SpaceID)
	}
	if runtimeContext.HarnessKind != s.Kind {
		return fmt.Errorf("runtime context harness kind %q does not match session kind %q", runtimeContext.HarnessKind, s.Kind)
	}
	if runtimeContext.ChannelID == "" {
		return fmt.Errorf("channelID is required")
	}
	if runtimeContext.DisplayName == "" {
		return fmt.Errorf("displayName is required")
	}
	if runtimeContext.MemberType == "" {
		return fmt.Errorf("memberType is required")
	}
	if runtimeContext.LifecycleState == "" {
		return fmt.Errorf("lifecycleState is required")
	}
	if runtimeContext.Model == "" {
		return fmt.Errorf("model is required")
	}
	if runtimeContext.Effort == "" {
		return fmt.Errorf("effort is required")
	}
	if runtimeContext.PermissionMode == "" {
		runtimeContext.PermissionMode = compatibilityPermissionMode(runtimeContext.HarnessKind)
	}
	if runtimeContext.PermissionMode == "" {
		return fmt.Errorf("permissionMode is required")
	}
	if runtimeContext.SystemPrompt == "" {
		return fmt.Errorf("systemPrompt is required")
	}
	if runtimeContext.Workdir == "" {
		return fmt.Errorf("workdir is required")
	}
	wasUnprovisioned := s.SystemPrompt == "" || len(s.MCPServers) == 0
	s.ChannelID = runtimeContext.ChannelID
	s.LocationID = runtimeContext.LocationID
	s.DisplayName = runtimeContext.DisplayName
	s.MemberType = runtimeContext.MemberType
	s.LifecycleState = runtimeContext.LifecycleState
	s.Model = runtimeContext.Model
	s.Effort = runtimeContext.Effort
	s.PermissionMode = runtimeContext.PermissionMode
	s.ConfigRef = runtimeContext.ConfigRef
	if runtimeContext.SessionRef != "" {
		s.Ref = runtimeContext.SessionRef
	}
	s.Workdir = runtimeContext.Workdir
	s.SystemPrompt = runtimeContext.SystemPrompt
	s.MCPToken = runtimeContext.MCPToken
	s.MCPServers = append([]string(nil), runtimeContext.MCPServers...)
	if wasUnprovisioned {
		s.Ref = ""
	}
	return nil
}

func compatibilityPermissionMode(kind string) string {
	switch kind {
	case "codex":
		return "codex/full-access"
	case "claude-cli":
		return "claude-code/bypass-permissions"
	default:
		return ""
	}
}

func (s *Session) AddUsage(tokensIn, tokensOut int64) {
	s.TokensIn += tokensIn
	s.TokensOut += tokensOut
}

func (s *Session) UpdateClaudeChannelRoute(rawURL string, instanceID string, startedAt time.Time) error {
	if s.Status != SessionActive {
		return fmt.Errorf("cannot update session %q claude channel url: status is %q, not active", s.ID, s.Status)
	}
	if rawURL == "" {
		return fmt.Errorf("claude channel url is required")
	}
	if instanceID == "" {
		return fmt.Errorf("claude channel instance id is required")
	}
	if startedAt.IsZero() {
		return fmt.Errorf("claude channel started at is required")
	}
	if s.ClaudeChannelStartedAt != nil && startedAt.Before(*s.ClaudeChannelStartedAt) {
		return fmt.Errorf("cannot replace newer claude channel instance %q started at %s with older instance %q started at %s", s.ClaudeChannelInstanceID, s.ClaudeChannelStartedAt.UTC().Format(time.RFC3339Nano), instanceID, startedAt.UTC().Format(time.RFC3339Nano))
	}
	s.ClaudeChannelURL = rawURL
	s.ClaudeChannelInstanceID = instanceID
	started := startedAt.UTC()
	s.ClaudeChannelStartedAt = &started
	return nil
}

func (s *Session) UpdateClaudeChannelURL(rawURL string) error {
	return s.UpdateClaudeChannelRoute(rawURL, "legacy-"+rawURL, time.Now().UTC())
}
