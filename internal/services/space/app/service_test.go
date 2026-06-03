package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type serviceTestRepo struct {
	spaces      map[domain.SpaceID]domain.SpaceRecord
	listFilters []domain.SpaceFilter
	deleted     []domain.SpaceID
}

func newServiceTestRepo(spaces ...domain.SpaceRecord) *serviceTestRepo {
	out := &serviceTestRepo{spaces: map[domain.SpaceID]domain.SpaceRecord{}}
	for _, space := range spaces {
		out.spaces[space.ID] = space
	}
	return out
}

func (r *serviceTestRepo) Get(_ context.Context, id domain.SpaceID) (domain.SpaceRecord, error) {
	space, ok := r.spaces[id]
	if !ok {
		return domain.SpaceRecord{}, fmt.Errorf("space not found")
	}
	return space, nil
}

func (r *serviceTestRepo) List(_ context.Context, filter domain.SpaceFilter) ([]domain.SpaceRecord, error) {
	r.listFilters = append(r.listFilters, filter)
	var out []domain.SpaceRecord
	for _, space := range r.spaces {
		if filter.SpaceID != "" && string(space.ID) != filter.SpaceID {
			continue
		}
		if filter.UserID != "" && space.UserID != filter.UserID {
			continue
		}
		if filter.ProjectID != "" && string(space.ProjectID) != filter.ProjectID {
			continue
		}
		if filter.Status != "" && space.Status != filter.Status {
			continue
		}
		out = append(out, space)
	}
	return out, nil
}

func (r *serviceTestRepo) Create(_ context.Context, space domain.SpaceRecord) error {
	r.spaces[space.ID] = space
	return nil
}

func (r *serviceTestRepo) Update(_ context.Context, space domain.SpaceRecord) error {
	r.spaces[space.ID] = space
	return nil
}

func (r *serviceTestRepo) Delete(_ context.Context, id domain.SpaceID) error {
	r.deleted = append(r.deleted, id)
	delete(r.spaces, id)
	return nil
}

type serviceTestMemberRepo struct {
	members map[string]member.Record
}

type serviceTestConfigValidator struct{}

func (serviceTestConfigValidator) ValidateConfig(harnessKind, model, effort string) error {
	if harnessKind == "" {
		return fmt.Errorf("harnessKind is required")
	}
	if model == "" {
		return fmt.Errorf("model is required")
	}
	if effort == "" {
		return fmt.Errorf("effort is required")
	}
	if harnessKind == "claude-cli" && strings.HasPrefix(model, "gpt-") {
		return fmt.Errorf("unsupported model")
	}
	return nil
}

func newServiceTestMemberRepo(members ...member.Record) *serviceTestMemberRepo {
	out := &serviceTestMemberRepo{members: map[string]member.Record{}}
	for _, rosterMember := range members {
		out.members[string(rosterMember.ID)] = rosterMember
	}
	return out
}

func (r *serviceTestMemberRepo) Get(_ context.Context, id string) (member.Record, error) {
	rosterMember, ok := r.members[id]
	if !ok {
		return member.Record{}, member.ErrNotFound
	}
	return rosterMember, nil
}

func (r *serviceTestMemberRepo) List(_ context.Context, filter member.Filter) ([]member.Record, error) {
	var out []member.Record
	for _, rosterMember := range r.members {
		if filter.SpaceID != "" && string(rosterMember.SpaceID) != filter.SpaceID {
			continue
		}
		if filter.ProjectID != "" && string(rosterMember.ProjectID) != filter.ProjectID {
			continue
		}
		if filter.UserID != "" && rosterMember.UserID != filter.UserID {
			continue
		}
		if filter.MemberType != "" && rosterMember.MemberType != filter.MemberType {
			continue
		}
		if filter.LifecycleState != "" && rosterMember.LifecycleState != filter.LifecycleState {
			continue
		}
		out = append(out, rosterMember)
	}
	return out, nil
}

func (r *serviceTestMemberRepo) Create(_ context.Context, rosterMember member.Record) error {
	r.members[string(rosterMember.ID)] = rosterMember
	return nil
}

func (r *serviceTestMemberRepo) Update(_ context.Context, rosterMember member.Record) error {
	r.members[string(rosterMember.ID)] = rosterMember
	return nil
}

func TestGetRejectsSpaceOutsideCallerVisibility(t *testing.T) {
	repo := newServiceTestRepo(domain.SpaceRecord{ID: "space-1", UserID: "user-owner", Status: domain.SpaceStatusOpen})
	svc, err := NewService(repo, newServiceTestMemberRepo(), domain.FixedClock{T: time.Unix(1, 0).UTC()}, caller.ContextResolver{}, serviceTestConfigValidator{}, &testEventPublisher{}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-other"})

	_, err = svc.Get(ctx, "space-1")
	if err == nil {
		t.Fatalf("Get error=nil want visibility error")
	}
}

func TestGetAllowsMemberCallerForOwnSpace(t *testing.T) {
	repo := newServiceTestRepo(domain.SpaceRecord{ID: "space-1", UserID: "user-owner", Status: domain.SpaceStatusOpen})
	svc, err := NewService(repo, newServiceTestMemberRepo(), domain.FixedClock{T: time.Unix(1, 0).UTC()}, caller.ContextResolver{}, serviceTestConfigValidator{}, &testEventPublisher{}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{MemberID: "member-1", SpaceID: "space-1"})

	space, err := svc.Get(ctx, "space-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if space.ID != "space-1" {
		t.Fatalf("space id=%q want space-1", space.ID)
	}
}

func TestListScopesToCallerUser(t *testing.T) {
	repo := newServiceTestRepo(
		domain.SpaceRecord{ID: "space-1", UserID: "user-1", Status: domain.SpaceStatusOpen},
		domain.SpaceRecord{ID: "space-2", UserID: "user-2", Status: domain.SpaceStatusOpen},
	)
	svc, err := NewService(repo, newServiceTestMemberRepo(), domain.FixedClock{T: time.Unix(1, 0).UTC()}, caller.ContextResolver{}, serviceTestConfigValidator{}, &testEventPublisher{}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-1"})

	spaces, err := svc.List(ctx, domain.SpaceFilter{UserID: "user-2"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(spaces) != 1 || spaces[0].ID != "space-1" {
		t.Fatalf("spaces=%+v want only space-1", spaces)
	}
	if got := repo.listFilters[0].UserID; got != "user-1" {
		t.Fatalf("filter UserID=%q want user-1", got)
	}
}

func TestListScopesMemberOnlyCallerToOwnSpace(t *testing.T) {
	repo := newServiceTestRepo(
		domain.SpaceRecord{ID: "space-1", UserID: "user-1", Status: domain.SpaceStatusOpen},
		domain.SpaceRecord{ID: "space-2", UserID: "user-1", Status: domain.SpaceStatusOpen},
	)
	svc, err := NewService(repo, newServiceTestMemberRepo(), domain.FixedClock{T: time.Unix(1, 0).UTC()}, caller.ContextResolver{}, serviceTestConfigValidator{}, &testEventPublisher{}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{MemberID: "member-1", SpaceID: "space-1"})

	spaces, err := svc.List(ctx, domain.SpaceFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(spaces) != 1 || spaces[0].ID != "space-1" {
		t.Fatalf("spaces=%+v want only space-1", spaces)
	}
	if got := repo.listFilters[0].SpaceID; got != "space-1" {
		t.Fatalf("filter SpaceID=%q want space-1", got)
	}
}

func TestDeleteRequiresOwningUser(t *testing.T) {
	repo := newServiceTestRepo(domain.SpaceRecord{ID: "space-1", UserID: "user-owner", Status: domain.SpaceStatusOpen})
	svc, err := NewService(repo, newServiceTestMemberRepo(), domain.FixedClock{T: time.Unix(1, 0).UTC()}, caller.ContextResolver{}, serviceTestConfigValidator{}, &testEventPublisher{}, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx := caller.ContextWithCaller(context.Background(), caller.Caller{MemberID: "member-1", SpaceID: "space-1"})

	err = svc.Delete(ctx, "space-1")
	if err == nil {
		t.Fatalf("Delete error=nil want ownership error")
	}
	if len(repo.deleted) != 0 {
		t.Fatalf("deleted=%+v want none", repo.deleted)
	}
}
