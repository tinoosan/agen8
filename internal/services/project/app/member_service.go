package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/core/types"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

type RegisterMemberResult struct {
	Member            member.Record
	GrantedMemberType string
}

type UpsertExternalHarnessMemberParams struct {
	ID               member.ID
	UserID           string
	ProjectID        string
	ChannelID        string
	NativeSessionRef string
	DisplayName      string
	MemberType       string
	HarnessKind      string
	Model            string
	Effort           string
	PermissionMode   string
	ConfigRef        string
}

func (s *Service) UpsertExternalHarnessMember(ctx context.Context, p UpsertExternalHarnessMemberParams) (member.Record, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return member.Record{}, err
	}
	projectID := types.ProjectID(strings.TrimSpace(p.ProjectID))
	if projectID == "" {
		return member.Record{}, fmt.Errorf("projectId is required")
	}
	if err := s.requireRosterWriteAccess(ctx, caller, projectID); err != nil {
		return member.Record{}, err
	}
	memberID := member.ID(strings.TrimSpace(string(p.ID)))
	if memberID == "" {
		return member.Record{}, fmt.Errorf("memberId is required")
	}
	memberType := strings.TrimSpace(p.MemberType)
	if memberType == "" {
		memberType = member.TypeCoordinator
	}
	if err := member.ValidateMemberType(memberType); err != nil {
		return member.Record{}, err
	}
	permissionMode := strings.TrimSpace(p.PermissionMode)
	if permissionMode == "" {
		permissionMode = s.defaultPermissionMode(p.HarnessKind)
	}
	if err := s.validateRuntimeConfig(p.HarnessKind, p.Model, p.Effort, permissionMode, p.ConfigRef); err != nil {
		return member.Record{}, err
	}
	now := s.clock.Now()
	rosterMember := member.Record{
		ID:               memberID,
		UserID:           strings.TrimSpace(p.UserID),
		ProjectID:        string(projectID),
		ChannelID:        strings.TrimSpace(p.ChannelID),
		NativeSessionRef: strings.TrimSpace(p.NativeSessionRef),
		DisplayName:      strings.TrimSpace(p.DisplayName),
		MemberType:       memberType,
		LifecycleState:   member.LifecycleActive,
		HarnessKind:      strings.TrimSpace(p.HarnessKind),
		Model:            strings.TrimSpace(p.Model),
		Effort:           strings.TrimSpace(p.Effort),
		PermissionMode:   permissionMode,
		ConfigRef:        strings.TrimSpace(p.ConfigRef),
		RegisteredAt:     now,
		UpdatedAt:        now,
	}
	if rosterMember.ChannelID == "" {
		rosterMember.ChannelID = memberChannelID(projectID, memberID)
	}
	if rosterMember.DisplayName == "" {
		rosterMember.DisplayName = strings.ReplaceAll(memberType, "_", " ")
	}
	existing, err := s.members.Get(ctx, string(memberID))
	if err != nil && !errors.Is(err, member.ErrNotFound) {
		return member.Record{}, fmt.Errorf("load external harness member: %w", err)
	}
	if err == nil {
		rosterMember.RegisteredAt = existing.RegisteredAt
		if rosterMember.RegisteredAt.IsZero() {
			rosterMember.RegisteredAt = now
		}
		if err := s.members.Update(ctx, rosterMember); err != nil {
			return member.Record{}, fmt.Errorf("update external harness member: %w", err)
		}
		return s.withResolvedPermissionMode(rosterMember), nil
	}
	if err := s.members.Create(ctx, rosterMember); err != nil {
		return member.Record{}, fmt.Errorf("create external harness member: %w", err)
	}
	return s.withResolvedPermissionMode(rosterMember), nil
}

func (s *Service) RegisterMember(ctx context.Context, rosterMember member.Record) (RegisterMemberResult, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return RegisterMemberResult{}, err
	}
	projectID := types.ProjectID(strings.TrimSpace(rosterMember.ProjectID))
	if projectID == "" {
		return RegisterMemberResult{}, fmt.Errorf("projectId is required")
	}
	if err := s.requireRosterWriteAccess(ctx, caller, projectID); err != nil {
		return RegisterMemberResult{}, err
	}
	if strings.TrimSpace(rosterMember.PermissionMode) == "" {
		rosterMember.PermissionMode = s.defaultPermissionMode(rosterMember.HarnessKind)
	}
	if err := s.validateRuntimeConfig(rosterMember.HarnessKind, rosterMember.Model, rosterMember.Effort, rosterMember.PermissionMode, rosterMember.ConfigRef); err != nil {
		return RegisterMemberResult{}, err
	}

	requestedType := strings.TrimSpace(rosterMember.MemberType)
	if requestedType == "" {
		requestedType = member.TypeCoordinator
	}
	if err := member.ValidateMemberType(requestedType); err != nil {
		return RegisterMemberResult{}, err
	}

	grantedType := requestedType

	rosterMember.ID = member.ID("member-" + uuid.NewString())
	rosterMember.ProjectID = string(projectID)
	rosterMember.ChannelID = memberChannelID(projectID, rosterMember.ID)
	rosterMember.MemberType = grantedType
	rosterMember.LifecycleState = member.LifecycleActive

	if strings.TrimSpace(rosterMember.DisplayName) == "" {
		rosterMember.DisplayName = strings.ReplaceAll(grantedType, "_", " ")
	}

	now := s.clock.Now()
	rosterMember.RegisteredAt = now
	rosterMember.UpdatedAt = now

	if err := s.members.Create(ctx, rosterMember); err != nil {
		return RegisterMemberResult{}, fmt.Errorf("create project member: %w", err)
	}

	created, err := s.members.Get(ctx, string(rosterMember.ID))
	if err != nil {
		return RegisterMemberResult{}, fmt.Errorf("load created project member: %w", err)
	}
	created = s.withResolvedPermissionMode(created)
	if err := s.publishMemberLifecycle(eventbus.SpaceMemberEventRegistered, created); err != nil {
		return RegisterMemberResult{}, fmt.Errorf("publish project member registered: %w", err)
	}

	s.logger.InfoContext(ctx, "project member registered", "project_id", created.ProjectID, "member_id", created.ID, "member_type", created.MemberType)
	return RegisterMemberResult{
		Member:            created,
		GrantedMemberType: grantedType,
	}, nil
}

func (s *Service) GetMember(ctx context.Context, id member.ID) (member.Record, error) {
	idStr := strings.TrimSpace(string(id))
	if idStr == "" {
		return member.Record{}, fmt.Errorf("memberId is required")
	}
	loaded, err := s.members.Get(ctx, idStr)
	if err != nil {
		return member.Record{}, err
	}
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return member.Record{}, err
	}
	if err := s.requireRosterReadAccess(ctx, caller, types.ProjectID(loaded.ProjectID)); err != nil {
		return member.Record{}, err
	}
	return s.withResolvedPermissionMode(loaded), nil
}

func (s *Service) DisplayName(ctx context.Context, id member.ID) (string, error) {
	rosterMember, err := s.GetMember(ctx, id)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rosterMember.DisplayName), nil
}

func (s *Service) UpdateMember(ctx context.Context, id member.ID, displayName string) (member.Record, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return member.Record{}, fmt.Errorf("displayName is required")
	}
	loaded, err := s.GetMember(ctx, id)
	if err != nil {
		return member.Record{}, err
	}
	loaded.DisplayName = displayName
	loaded.UpdatedAt = s.clock.Now().UTC()
	if err := s.members.Update(ctx, loaded); err != nil {
		return member.Record{}, fmt.Errorf("update project member: %w", err)
	}
	if err := s.publishMemberLifecycle(eventbus.SpaceMemberEventIdentityChanged, loaded); err != nil {
		return member.Record{}, fmt.Errorf("publish project member identity changed: %w", err)
	}
	s.logger.InfoContext(ctx, "project member updated", "project_id", loaded.ProjectID, "member_id", loaded.ID)
	return loaded, nil
}

func (s *Service) ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return nil, err
	}
	filter.ProjectID = strings.TrimSpace(filter.ProjectID)
	filter.UserID = strings.TrimSpace(filter.UserID)
	filter.HarnessKind = strings.TrimSpace(filter.HarnessKind)
	filter.NativeSessionRef = strings.TrimSpace(filter.NativeSessionRef)
	if caller.MemberID != "" && caller.ProjectID != "" {
		if filter.ProjectID != "" && filter.ProjectID != string(caller.ProjectID) {
			return nil, fmt.Errorf("project members are not visible to caller")
		}
		filter.ProjectID = string(caller.ProjectID)
	}
	if filter.ProjectID == "" && filter.UserID == "" {
		return nil, fmt.Errorf("projectId or userId is required")
	}
	if filter.ProjectID != "" {
		if err := s.requireRosterReadAccess(ctx, caller, types.ProjectID(filter.ProjectID)); err != nil {
			return nil, err
		}
	}
	if filter.MemberType != "" {
		if err := member.ValidateMemberType(filter.MemberType); err != nil {
			return nil, err
		}
	}
	if filter.LifecycleState != "" {
		if err := member.ValidateLifecycleState(filter.LifecycleState); err != nil {
			return nil, err
		}
	}
	if filter.Limit <= 0 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	members, err := s.members.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	for i := range members {
		members[i] = s.withResolvedPermissionMode(members[i])
	}
	return members, nil
}

func (s *Service) withResolvedPermissionMode(rosterMember member.Record) member.Record {
	if strings.TrimSpace(rosterMember.PermissionMode) != "" {
		return rosterMember
	}
	rosterMember.PermissionMode = s.compatibilityPermissionMode(rosterMember.HarnessKind)
	return rosterMember
}

func (s *Service) validateRuntimeConfig(harnessKind, model, effort, permissionMode, configRef string) error {
	if validator, ok := s.configs.(runtimeConfigValidator); ok {
		return validator.ValidateRuntimeConfig(harnessKind, model, effort, permissionMode, configRef)
	}
	return s.configs.ValidateConfig(harnessKind, model, effort)
}

func (s *Service) defaultPermissionMode(harnessKind string) string {
	if defaults, ok := s.configs.(permissionModeDefaults); ok {
		return defaults.DefaultPermissionMode(harnessKind)
	}
	return strings.TrimSpace(harnessKind) + "/default"
}

func (s *Service) compatibilityPermissionMode(harnessKind string) string {
	if defaults, ok := s.configs.(permissionModeDefaults); ok {
		return defaults.CompatibilityPermissionMode(harnessKind)
	}
	return strings.TrimSpace(harnessKind) + "/default"
}

func (s *Service) RemoveMember(ctx context.Context, id member.ID) (member.Record, error) {
	loaded, err := s.GetMember(ctx, id)
	if err != nil {
		return member.Record{}, err
	}
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return member.Record{}, err
	}
	if err := s.requireRosterWriteAccess(ctx, caller, types.ProjectID(loaded.ProjectID)); err != nil {
		return member.Record{}, err
	}
	agg := member.WrapMember(loaded)
	removed, err := agg.Remove(s.clock.Now())
	if err != nil {
		return member.Record{}, err
	}
	inner := removed.Inner()
	if err := s.members.Update(ctx, inner); err != nil {
		return member.Record{}, fmt.Errorf("update project member: %w", err)
	}
	if err := s.publishMemberLifecycle(eventbus.SpaceMemberEventRemoved, inner); err != nil {
		return member.Record{}, fmt.Errorf("publish project member removed: %w", err)
	}
	s.logger.InfoContext(ctx, "project member removed", "project_id", inner.ProjectID, "member_id", inner.ID)
	return inner, nil
}

func (s *Service) requireRosterReadAccess(ctx context.Context, caller Caller, projectID types.ProjectID) error {
	p, err := s.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	return requireVisibleProject(caller, p)
}

func (s *Service) requireRosterWriteAccess(ctx context.Context, caller Caller, projectID types.ProjectID) error {
	p, err := s.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("load project: %w", err)
	}
	if caller.UserID != "" {
		return requireOwnedProject(caller, p)
	}
	if caller.MemberID == "" {
		return fmt.Errorf("project member roster requires a user or member caller")
	}
	actor, err := s.members.Get(ctx, string(caller.MemberID))
	if err != nil {
		return fmt.Errorf("load caller member: %w", err)
	}
	if types.ProjectID(actor.ProjectID) != projectID {
		return fmt.Errorf("caller member %s does not belong to project %s", actor.ID, projectID)
	}
	if actor.LifecycleState != member.LifecycleActive {
		return fmt.Errorf("caller member %s is not active", actor.ID)
	}
	return nil
}

func memberChannelID(projectID types.ProjectID, memberID member.ID) string {
	return "channel:" + string(projectID) + ":member:" + string(memberID)
}
