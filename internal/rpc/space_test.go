package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
	spacerpc "github.com/tinoosan/agen8-mcp-server/internal/services/space/rpc"
)

var rpcSpaceTestNow = time.Date(2026, 5, 16, 9, 0, 0, 0, time.UTC)

func TestRegisterSpaceDispatchCreate(t *testing.T) {
	svc := newRPCSpaceService()
	reg := NewRegistry()
	if err := RegisterSpace(reg, svc); err != nil {
		t.Fatalf("RegisterSpace returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "space.create",
		"params": {
			"spaceId": "space-1",
			"projectId": "project-1",
			"title": "Backend",
			"planMode": "manual"
		}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result spacerpc.SpaceCreateResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if result.Space.ID != "space-1" || result.Space.ProjectID != "project-1" || result.Space.Title != "Backend" {
		t.Fatalf("space result=%+v", result.Space)
	}
}

func TestRegisterSpaceMapsInvalidParams(t *testing.T) {
	svc := newRPCSpaceService()
	reg := NewRegistry()
	if err := RegisterSpace(reg, svc); err != nil {
		t.Fatalf("RegisterSpace returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "space.create",
		"params": { "title": "Missing project" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error=%+v want invalid params", resp.Error)
	}
}

func newRPCSpaceService() *spaceapp.Service {
	svc, err := spaceapp.NewService(newRPCSpaceRepo(), newRPCSpaceMemberRepo(), spacedomain.FixedClock{T: rpcSpaceTestNow}, caller.ContextResolver{}, rpcSpaceConfigValidator{}, rpcSpaceEventPublisher{}, nil)
	if err != nil {
		panic(err)
	}
	return svc
}

type rpcSpaceConfigValidator struct{}

func (rpcSpaceConfigValidator) ValidateConfig(_, _, _ string) error { return nil }

type rpcSpaceEventPublisher struct{}

func (rpcSpaceEventPublisher) Publish(string, any) error { return nil }

type rpcSpaceMemberRepo struct {
	members map[string]member.Record
}

func newRPCSpaceMemberRepo(members ...member.Record) *rpcSpaceMemberRepo {
	repo := &rpcSpaceMemberRepo{members: map[string]member.Record{}}
	for _, rosterMember := range members {
		repo.members[string(rosterMember.ID)] = rosterMember
	}
	return repo
}

func (r *rpcSpaceMemberRepo) Get(_ context.Context, id string) (member.Record, error) {
	rosterMember, ok := r.members[id]
	if !ok {
		return member.Record{}, member.ErrNotFound
	}
	return rosterMember, nil
}

func (r *rpcSpaceMemberRepo) List(_ context.Context, filter member.Filter) ([]member.Record, error) {
	out := make([]member.Record, 0, len(r.members))
	for _, rosterMember := range r.members {
		if filter.SpaceID != "" && string(rosterMember.SpaceID) != filter.SpaceID {
			continue
		}
		if filter.UserID != "" && rosterMember.UserID != filter.UserID {
			continue
		}
		out = append(out, rosterMember)
	}
	return out, nil
}

func (r *rpcSpaceMemberRepo) Create(_ context.Context, rosterMember member.Record) error {
	r.members[string(rosterMember.ID)] = rosterMember
	return nil
}

func (r *rpcSpaceMemberRepo) Update(_ context.Context, rosterMember member.Record) error {
	r.members[string(rosterMember.ID)] = rosterMember
	return nil
}

type rpcSpaceRepo struct {
	spaces map[string]spacedomain.SpaceRecord
}

func newRPCSpaceRepo(spaces ...spacedomain.SpaceRecord) *rpcSpaceRepo {
	repo := &rpcSpaceRepo{spaces: map[string]spacedomain.SpaceRecord{}}
	for _, space := range spaces {
		repo.spaces[string(space.ID)] = space
	}
	return repo
}

func (r *rpcSpaceRepo) Get(_ context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
	space, ok := r.spaces[string(id)]
	if !ok {
		return spacedomain.SpaceRecord{}, fmt.Errorf("space %s not found", id)
	}
	return space, nil
}

func (r *rpcSpaceRepo) List(_ context.Context, filter spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error) {
	out := make([]spacedomain.SpaceRecord, 0, len(r.spaces))
	for _, space := range r.spaces {
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

func (r *rpcSpaceRepo) Create(_ context.Context, space spacedomain.SpaceRecord) error {
	if space.ID == "" {
		return fmt.Errorf("space id is required")
	}
	r.spaces[string(space.ID)] = space
	return nil
}

func (r *rpcSpaceRepo) Update(_ context.Context, space spacedomain.SpaceRecord) error {
	if _, ok := r.spaces[string(space.ID)]; !ok {
		return fmt.Errorf("space %s not found", space.ID)
	}
	r.spaces[string(space.ID)] = space
	return nil
}

func (r *rpcSpaceRepo) Delete(_ context.Context, id spacedomain.SpaceID) error {
	delete(r.spaces, string(id))
	return nil
}
