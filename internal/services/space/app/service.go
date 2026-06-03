package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/eventbus"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type Service struct {
	spaces  domain.Repository
	members member.Repository
	clock   domain.Clock
	caller  caller.Resolver
	configs ConfigValidator
	events  EventPublisher
	logger  *slog.Logger
}

type Caller = caller.Caller

type ConfigValidator interface {
	ValidateConfig(harnessKind, model, effort string) error
}

type runtimeConfigValidator interface {
	ValidateRuntimeConfig(harnessKind, model, effort, permissionMode, configRef string) error
}

type permissionModeDefaults interface {
	DefaultPermissionMode(harnessKind string) string
	CompatibilityPermissionMode(harnessKind string) string
}

type EventPublisher interface {
	Publish(topic string, event any) error
}

func NewService(spaces domain.Repository, members member.Repository, clock domain.Clock, caller caller.Resolver, configs ConfigValidator, events EventPublisher, logger *slog.Logger) (*Service, error) {
	if spaces == nil {
		return nil, fmt.Errorf("space repository is required")
	}
	if members == nil {
		return nil, fmt.Errorf("space member repository is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("space clock is required")
	}
	if caller == nil {
		return nil, fmt.Errorf("space caller resolver is required")
	}
	if configs == nil {
		return nil, fmt.Errorf("space member config validator is required")
	}
	if events == nil {
		return nil, fmt.Errorf("space event publisher is required")
	}
	if logger == nil {
		logger = slog.Default().With("service", "space")
	}
	return &Service{
		spaces:  spaces,
		members: members,
		clock:   clock,
		caller:  caller,
		configs: configs,
		events:  events,
		logger:  logger,
	}, nil
}

func (s *Service) publishMemberLifecycle(eventType string, rosterMember member.Record) error {
	if s.events == nil {
		return fmt.Errorf("space event publisher is required")
	}
	return s.events.Publish(eventbus.TopicSpaceMemberLifecycle, eventbus.SpaceMemberLifecycleEvent{
		UserID:         rosterMember.UserID,
		ProjectID:      rosterMember.ProjectID,
		SpaceID:        rosterMember.SpaceID,
		MemberID:       string(rosterMember.ID),
		ChannelID:      rosterMember.ChannelID,
		DisplayName:    rosterMember.DisplayName,
		MemberType:     rosterMember.MemberType,
		EventType:      eventType,
		LifecycleState: rosterMember.LifecycleState,
		HarnessKind:    rosterMember.HarnessKind,
		Model:          rosterMember.Model,
		Effort:         rosterMember.Effort,
		PermissionMode: rosterMember.PermissionMode,
		ConfigRef:      rosterMember.ConfigRef,
		Timestamp:      s.clock.Now().UTC(),
	})
}

func (s *Service) Create(ctx context.Context, space domain.SpaceRecord) (domain.SpaceRecord, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.SpaceRecord{}, err
	}
	agg := domain.WrapSpace(space)
	inner := agg.Inner()
	if caller.UserID != "" {
		inner.UserID = caller.UserID
	}
	inner.CreatedAt = s.clock.Now()
	inner.UpdatedAt = inner.CreatedAt
	if err := s.spaces.Create(ctx, inner); err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("create space: %w", err)
	}
	s.logger.InfoContext(ctx, "space created", "space_id", inner.ID, "project_id", inner.ProjectID, "user_id", inner.UserID)
	return inner, nil
}

func (s *Service) Get(ctx context.Context, id domain.SpaceID) (domain.SpaceRecord, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.SpaceRecord{}, err
	}
	loaded, err := s.spaces.Get(ctx, id)
	if err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("load space: %w", err)
	}
	if err := requireVisibleSpace(caller, loaded); err != nil {
		return domain.SpaceRecord{}, err
	}
	return loaded, nil
}

func (s *Service) List(ctx context.Context, filter domain.SpaceFilter) ([]domain.SpaceRecord, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return nil, err
	}
	if caller.UserID != "" {
		filter.UserID = caller.UserID
	} else if caller.SpaceID != "" {
		filter.SpaceID = string(caller.SpaceID)
	}
	return s.spaces.List(ctx, filter)
}

func (s *Service) Close(ctx context.Context, id domain.SpaceID) (domain.SpaceRecord, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.SpaceRecord{}, err
	}
	loaded, err := s.spaces.Get(ctx, id)
	if err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("load space: %w", err)
	}
	if err := requireOwnedSpace(caller, loaded); err != nil {
		return domain.SpaceRecord{}, err
	}
	agg := domain.WrapSpace(loaded)
	next, err := agg.Close(s.clock.Now())
	if err != nil {
		return domain.SpaceRecord{}, err
	}
	inner := next.Inner()
	if err := s.spaces.Update(ctx, inner); err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("update space: %w", err)
	}
	s.logger.InfoContext(ctx, "space closed", "space_id", inner.ID, "user_id", inner.UserID)
	return inner, nil
}

func (s *Service) Reopen(ctx context.Context, id domain.SpaceID) (domain.SpaceRecord, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.SpaceRecord{}, err
	}
	loaded, err := s.spaces.Get(ctx, id)
	if err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("load space: %w", err)
	}
	if err := requireOwnedSpace(caller, loaded); err != nil {
		return domain.SpaceRecord{}, err
	}
	agg := domain.WrapSpace(loaded)
	next, err := agg.Reopen(s.clock.Now())
	if err != nil {
		return domain.SpaceRecord{}, err
	}
	inner := next.Inner()
	if err := s.spaces.Update(ctx, inner); err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("update space: %w", err)
	}
	s.logger.InfoContext(ctx, "space reopened", "space_id", inner.ID, "user_id", inner.UserID)
	return inner, nil
}

type UpdateParams struct {
	Title         *string
	PlanMode      *string
	Customization *domain.SpaceCustomization
}

func (s *Service) Update(ctx context.Context, id domain.SpaceID, params UpdateParams) (domain.SpaceRecord, error) {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return domain.SpaceRecord{}, err
	}
	loaded, err := s.spaces.Get(ctx, id)
	if err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("load space: %w", err)
	}
	if err := requireOwnedSpace(caller, loaded); err != nil {
		return domain.SpaceRecord{}, err
	}
	if params.Title != nil {
		loaded.Title = *params.Title
	}
	if params.PlanMode != nil {
		loaded.PlanMode = *params.PlanMode
	}
	if params.Customization != nil {
		loaded.Customization = mergeCustomization(loaded.Customization, params.Customization)
	}
	loaded.UpdatedAt = s.clock.Now()
	if err := s.spaces.Update(ctx, loaded); err != nil {
		return domain.SpaceRecord{}, fmt.Errorf("update space: %w", err)
	}
	s.logger.InfoContext(ctx, "space updated", "space_id", loaded.ID, "user_id", loaded.UserID)
	return loaded, nil
}

// mergeCustomization applies a partial patch onto an existing customization.
// Empty strings in the patch mean "preserve existing value". The sentinel
// "none" means "clear this field". Any other value is a new setting.
func mergeCustomization(existing, patch *domain.SpaceCustomization) *domain.SpaceCustomization {
	if existing == nil {
		existing = &domain.SpaceCustomization{}
	}
	merged := *existing
	if patch.Icon == "none" {
		merged.Icon = ""
	} else if patch.Icon != "" {
		merged.Icon = patch.Icon
	}
	if patch.Color == "none" {
		merged.Color = ""
	} else if patch.Color != "" {
		merged.Color = patch.Color
	}
	// If both fields are empty after merge, return nil to collapse.
	if merged.Icon == "" && merged.Color == "" {
		return nil
	}
	return &merged
}

func (s *Service) Delete(ctx context.Context, id domain.SpaceID) error {
	caller, err := s.resolveCaller(ctx)
	if err != nil {
		return err
	}
	loaded, err := s.spaces.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("load space: %w", err)
	}
	if err := requireOwnedSpace(caller, loaded); err != nil {
		return err
	}
	if err := s.spaces.Delete(ctx, id); err != nil {
		return err
	}
	s.logger.InfoContext(ctx, "space deleted", "space_id", id, "user_id", loaded.UserID)
	return nil
}

func (s *Service) resolveCaller(ctx context.Context) (Caller, error) {
	caller, err := s.caller.ResolveCaller(ctx)
	if err != nil {
		return Caller{}, fmt.Errorf("resolve space caller: %w", err)
	}
	caller = caller.Normalize()
	if caller.UserID == "" && caller.MemberID == "" {
		return Caller{}, fmt.Errorf("space caller user id or member id is required")
	}
	return caller, nil
}

func requireVisibleSpace(caller Caller, space domain.SpaceRecord) error {
	if caller.UserID != "" && strings.TrimSpace(space.UserID) == caller.UserID {
		return nil
	}
	if caller.SpaceID != "" && space.ID == caller.SpaceID {
		return nil
	}
	return fmt.Errorf("space %s is not visible to caller", space.ID)
}

func requireOwnedSpace(caller Caller, space domain.SpaceRecord) error {
	if caller.UserID != "" && strings.TrimSpace(space.UserID) == caller.UserID {
		return nil
	}
	return fmt.Errorf("space %s is not owned by caller", space.ID)
}
