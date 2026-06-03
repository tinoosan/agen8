package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type RegisterMemberResult struct {
	Member            member.Record
	GrantedMemberType string
}

type UpsertExternalHarnessMemberParams struct {
	ID             member.ID
	UserID         string
	ProjectID      string
	SpaceID        string
	ChannelID      string
	DisplayName    string
	MemberType     string
	HarnessKind    string
	Model          string
	Effort         string
	PermissionMode string
	ConfigRef      string
}

func (s *Service) UpsertExternalHarnessMember(ctx context.Context, p UpsertExternalHarnessMemberParams) (member.Record, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return member.Record{}, err
	}
	spaceID := domain.SpaceID(strings.TrimSpace(p.SpaceID))
	if spaceID == "" {
		return member.Record{}, fmt.Errorf("spaceId is required")
	}
	if err := s.requireRosterWriteAccess(ctx, caller, spaceID); err != nil {
		return member.Record{}, err
	}
	memberID := member.ID(strings.TrimSpace(string(p.ID)))
	if memberID == "" {
		return member.Record{}, fmt.Errorf("memberId is required")
	}
	memberType := strings.TrimSpace(p.MemberType)
	if memberType == "" {
		memberType = member.TypeWorker
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
		ID:             memberID,
		UserID:         strings.TrimSpace(p.UserID),
		ProjectID:      strings.TrimSpace(p.ProjectID),
		SpaceID:        string(spaceID),
		ChannelID:      strings.TrimSpace(p.ChannelID),
		DisplayName:    strings.TrimSpace(p.DisplayName),
		MemberType:     memberType,
		LifecycleState: member.LifecycleActive,
		HarnessKind:    strings.TrimSpace(p.HarnessKind),
		Model:          strings.TrimSpace(p.Model),
		Effort:         strings.TrimSpace(p.Effort),
		PermissionMode: permissionMode,
		ConfigRef:      strings.TrimSpace(p.ConfigRef),
		RegisteredAt:   now,
		UpdatedAt:      now,
	}
	if rosterMember.ChannelID == "" {
		rosterMember.ChannelID = "channel:" + string(spaceID) + ":member:" + string(memberID)
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
	spaceID := domain.SpaceID(strings.TrimSpace(string(rosterMember.SpaceID)))
	if spaceID == "" {
		return RegisterMemberResult{}, fmt.Errorf("spaceId is required")
	}
	if err := s.requireRosterWriteAccess(ctx, caller, spaceID); err != nil {
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
		requestedType = member.TypeWorker
	}
	if err := member.ValidateMemberType(requestedType); err != nil {
		return RegisterMemberResult{}, err
	}

	if err := s.ensureCoordinatorSlotAvailable(ctx, string(spaceID), requestedType); err != nil {
		return RegisterMemberResult{}, err
	}
	grantedType := requestedType

	rosterMember.ID = member.ID("member-" + uuid.NewString())
	rosterMember.SpaceID = string(spaceID)
	rosterMember.ChannelID = "channel:" + string(spaceID) + ":member:" + string(rosterMember.ID)
	rosterMember.MemberType = grantedType
	rosterMember.LifecycleState = member.LifecycleActive

	if strings.TrimSpace(rosterMember.DisplayName) == "" {
		rosterMember.DisplayName = strings.ReplaceAll(grantedType, "_", " ")
	}

	now := s.clock.Now()
	rosterMember.RegisteredAt = now
	rosterMember.UpdatedAt = now

	if err := s.members.Create(ctx, rosterMember); err != nil {
		return RegisterMemberResult{}, fmt.Errorf("create space member: %w", err)
	}

	created, err := s.members.Get(ctx, string(rosterMember.ID))
	if err != nil {
		return RegisterMemberResult{}, fmt.Errorf("load created space member: %w", err)
	}
	created = s.withResolvedPermissionMode(created)
	if err := s.publishMemberLifecycle(eventbus.SpaceMemberEventRegistered, created); err != nil {
		return RegisterMemberResult{}, fmt.Errorf("publish space member registered: %w", err)
	}

	s.logger.InfoContext(ctx, "space member registered", "space_id", created.SpaceID, "member_id", created.ID, "member_type", created.MemberType)
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
	if err := s.requireRosterReadAccess(ctx, caller, domain.SpaceID(loaded.SpaceID)); err != nil {
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

func (s *Service) UpdateMemberConfig(ctx context.Context, id member.ID, model, effort, harnessKind string, permissionFields ...string) (member.Record, error) {
	permissionMode := ""
	configRef := ""
	if len(permissionFields) > 0 {
		permissionMode = permissionFields[0]
	}
	if len(permissionFields) > 1 {
		configRef = permissionFields[1]
	}
	if strings.TrimSpace(permissionMode) == "" {
		permissionMode = s.defaultPermissionMode(harnessKind)
	}
	if err := s.validateRuntimeConfig(harnessKind, model, effort, permissionMode, configRef); err != nil {
		return member.Record{}, err
	}
	loaded, err := s.GetMember(ctx, id)
	if err != nil {
		return member.Record{}, err
	}
	agg := member.WrapMember(loaded)
	updated, err := agg.UpdateRuntimeConfig(model, effort, harnessKind, permissionMode, configRef, s.clock.Now())
	if err != nil {
		return member.Record{}, err
	}
	inner := updated.Inner()
	if err := s.members.Update(ctx, inner); err != nil {
		return member.Record{}, fmt.Errorf("update space member config: %w", err)
	}
	if err := s.publishMemberLifecycle(eventbus.SpaceMemberEventConfigChanged, inner); err != nil {
		return member.Record{}, fmt.Errorf("publish space member config changed: %w", err)
	}
	s.logger.InfoContext(ctx, "space member config updated", "space_id", inner.SpaceID, "member_id", inner.ID)
	return inner, nil
}

func (s *Service) ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return nil, err
	}
	filter.SpaceID = strings.TrimSpace(filter.SpaceID)
	filter.ProjectID = strings.TrimSpace(filter.ProjectID)
	filter.UserID = strings.TrimSpace(filter.UserID)
	if caller.MemberID != "" && caller.SpaceID != "" {
		if filter.SpaceID != "" && filter.SpaceID != string(caller.SpaceID) {
			return nil, fmt.Errorf("space members are not visible to caller")
		}
		filter.SpaceID = string(caller.SpaceID)
	}
	if filter.SpaceID == "" && filter.ProjectID == "" && filter.UserID == "" {
		return nil, fmt.Errorf("spaceId, projectId, or userId is required")
	}
	if filter.SpaceID != "" {
		if err := s.requireRosterReadAccess(ctx, caller, domain.SpaceID(filter.SpaceID)); err != nil {
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
	if err := s.requireRosterWriteAccess(ctx, caller, domain.SpaceID(loaded.SpaceID)); err != nil {
		return member.Record{}, err
	}
	agg := member.WrapMember(loaded)
	removed, err := agg.Remove(s.clock.Now())
	if err != nil {
		return member.Record{}, err
	}
	inner := removed.Inner()
	if err := s.members.Update(ctx, inner); err != nil {
		return member.Record{}, fmt.Errorf("update space member: %w", err)
	}
	if err := s.publishMemberLifecycle(eventbus.SpaceMemberEventRemoved, inner); err != nil {
		return member.Record{}, fmt.Errorf("publish space member removed: %w", err)
	}
	s.logger.InfoContext(ctx, "space member removed", "space_id", inner.SpaceID, "member_id", inner.ID)
	return inner, nil
}

func (s *Service) ensureCoordinatorSlotAvailable(ctx context.Context, spaceID string, requested string) error {
	if !member.IsCoordinatorType(requested) {
		return nil
	}
	existing, err := s.members.List(ctx, member.Filter{
		SpaceID:        spaceID,
		LifecycleState: member.LifecycleActive,
		Limit:          500,
	})
	if err != nil {
		return fmt.Errorf("check existing space members: %w", err)
	}
	for _, rosterMember := range existing {
		if member.IsCoordinatorType(rosterMember.MemberType) {
			return fmt.Errorf("active coordinator already exists for space %s", spaceID)
		}
	}
	return nil
}

func (s *Service) requireRosterReadAccess(ctx context.Context, caller Caller, spaceID domain.SpaceID) error {
	space, err := s.spaces.Get(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("load space: %w", err)
	}
	return requireVisibleSpace(caller, space)
}

func (s *Service) requireRosterWriteAccess(ctx context.Context, caller Caller, spaceID domain.SpaceID) error {
	space, err := s.spaces.Get(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("load space: %w", err)
	}
	if caller.UserID != "" {
		return requireOwnedSpace(caller, space)
	}
	if caller.MemberID == "" {
		return fmt.Errorf("space member roster requires a user or member caller")
	}
	actor, err := s.members.Get(ctx, string(caller.MemberID))
	if err != nil {
		return fmt.Errorf("load caller member: %w", err)
	}
	if domain.SpaceID(actor.SpaceID) != spaceID {
		return fmt.Errorf("caller member %s does not belong to space %s", actor.ID, spaceID)
	}
	if actor.LifecycleState != member.LifecycleActive {
		return fmt.Errorf("caller member %s is not active", actor.ID)
	}
	if !member.IsCoordinatorType(actor.MemberType) {
		return fmt.Errorf("caller member %s is not a coordinator", actor.ID)
	}
	return nil
}
