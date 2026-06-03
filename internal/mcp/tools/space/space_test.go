package space

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	spaceapp "github.com/tinoosan/agen8-mcp-server/internal/services/space/app"
	spacedomain "github.com/tinoosan/agen8-mcp-server/internal/services/space/domain"
	"github.com/tinoosan/agen8-mcp-server/internal/services/space/domain/member"
)

type stubSpaceReader struct {
	getFn  func(context.Context, spacedomain.SpaceID) (spacedomain.SpaceRecord, error)
	listFn func(context.Context, spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error)
}

func (s stubSpaceReader) Get(ctx context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return spacedomain.SpaceRecord{ID: id, Title: "Runtime", Status: spacedomain.SpaceStatusOpen}, nil
}

func (s stubSpaceReader) List(ctx context.Context, filter spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error) {
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

type stubMemberDirectory struct {
	getFn  func(context.Context, member.ID) (member.Record, error)
	listFn func(context.Context, member.Filter) ([]member.Record, error)
}

func (s stubMemberDirectory) GetMember(ctx context.Context, id member.ID) (member.Record, error) {
	if s.getFn != nil {
		return s.getFn(ctx, id)
	}
	return member.Record{
		ID:             id,
		UserID:         "user-1",
		SpaceID:        "space-runtime",
		DisplayName:    string(id),
		MemberType:     member.TypeWorker,
		LifecycleState: member.LifecycleActive,
	}, nil
}

func (s stubMemberDirectory) ListMembers(ctx context.Context, filter member.Filter) ([]member.Record, error) {
	if s.listFn != nil {
		return s.listFn(ctx, filter)
	}
	return nil, nil
}

type stubMemberRegistrar struct {
	registerFn     func(context.Context, member.Record) (spaceapp.RegisterMemberResult, error)
	updateConfigFn func(context.Context, member.ID, string, string, string, ...string) (member.Record, error)
	removeFn       func(context.Context, member.ID) (member.Record, error)
}

func (s stubMemberRegistrar) RegisterMember(ctx context.Context, rosterMember member.Record) (spaceapp.RegisterMemberResult, error) {
	if s.registerFn != nil {
		return s.registerFn(ctx, rosterMember)
	}
	rosterMember.ID = "member-created"
	rosterMember.ChannelID = "channel:" + rosterMember.SpaceID + ":member:member-created"
	rosterMember.LifecycleState = member.LifecycleActive
	return spaceapp.RegisterMemberResult{Member: rosterMember, GrantedMemberType: rosterMember.MemberType}, nil
}

func (s stubMemberRegistrar) UpdateMemberConfig(ctx context.Context, id member.ID, model, effort, harnessKind string, permissionFields ...string) (member.Record, error) {
	if s.updateConfigFn != nil {
		return s.updateConfigFn(ctx, id, model, effort, harnessKind, permissionFields...)
	}
	return member.Record{ID: id, Model: model, Effort: effort, HarnessKind: harnessKind, LifecycleState: member.LifecycleActive}, nil
}

func (s stubMemberRegistrar) RemoveMember(ctx context.Context, id member.ID) (member.Record, error) {
	if s.removeFn != nil {
		return s.removeFn(ctx, id)
	}
	return member.Record{ID: id, LifecycleState: member.LifecycleRemoved}, nil
}

func TestDecodeRejectsMissingAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":""}`))
	if err == nil || !strings.Contains(err.Error(), "action is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsMessageAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"message"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported action "message"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_list","space_id":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "space_id" must be omitted instead of null`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsFieldForWrongAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_get","space_id":"space-1","member_id":"member-1"}`))
	if err == nil || !strings.Contains(err.Error(), `field "space_id" is not valid for action "member_get"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeCreateMemberRequiresRuntimeFields(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_create","harness_kind":"codex","effort":"medium"}`))
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestSchemaAllowsOmittedSpaceID(t *testing.T) {
	schema := NewHandler().Schema()
	var decoded struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	for _, field := range decoded.Required {
		if field == "space_id" {
			t.Fatalf("space_id should be optional in schema")
		}
	}
}

func TestHandleMembersDefaultsToRuntimeSpace(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		Spaces: stubSpaceReader{
			getFn: func(_ context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
				if id != "space-runtime" {
					t.Fatalf("space id=%q want space-runtime", id)
				}
				return spacedomain.SpaceRecord{ID: id, Title: "Runtime", Status: spacedomain.SpaceStatusOpen}, nil
			},
		},
		Members: stubMemberDirectory{
			getFn: func(ctx context.Context, id member.ID) (member.Record, error) {
				resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
				if err != nil {
					t.Fatalf("actor lookup missing caller: %v", err)
				}
				if resolved.MemberID != id || resolved.SpaceID != "space-runtime" {
					t.Fatalf("caller=%+v want member %s in space-runtime", resolved, id)
				}
				return member.Record{
					ID:             id,
					UserID:         "user-1",
					SpaceID:        "space-runtime",
					DisplayName:    string(id),
					MemberType:     member.TypeWorker,
					LifecycleState: member.LifecycleActive,
				}, nil
			},
			listFn: func(_ context.Context, filter member.Filter) ([]member.Record, error) {
				if filter.SpaceID != "space-runtime" {
					t.Fatalf("filter space id=%q want space-runtime", filter.SpaceID)
				}
				if filter.LifecycleState != member.LifecycleActive {
					t.Fatalf("filter lifecycle=%q want active", filter.LifecycleState)
				}
				return []member.Record{{
					ID:             "member-worker",
					SpaceID:        "space-runtime",
					ChannelID:      "channel:space-runtime:member:member-worker",
					DisplayName:    "Worker",
					MemberType:     member.TypeWorker,
					LifecycleState: member.LifecycleActive,
				}}, nil
			},
		},
		SpaceID:       "space-runtime",
		ActorMemberID: "member-worker",
	}, json.RawMessage(`{"action":"member_list"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured, ok := result.Structured.(map[string]any)
	if !ok {
		t.Fatalf("structured type %T", result.Structured)
	}
	if structured["action"] != "member_list" {
		t.Fatalf("action=%v want member_list", structured["action"])
	}
	if structured["count"] != 1 {
		t.Fatalf("count=%v want 1", structured["count"])
	}
}

func TestHandleListUsesProjectScopedSpaceService(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		ProjectID: "project-1",
		Spaces: stubSpaceReader{
			listFn: func(_ context.Context, filter spacedomain.SpaceFilter) ([]spacedomain.SpaceRecord, error) {
				if filter.ProjectID != "project-1" {
					t.Fatalf("project id=%q want project-1", filter.ProjectID)
				}
				if filter.Status != spacedomain.SpaceStatusOpen {
					t.Fatalf("status=%q want open", filter.Status)
				}
				return []spacedomain.SpaceRecord{{
					ID:        "space-1",
					ProjectID: "project-1",
					Title:     "Planning",
					Status:    spacedomain.SpaceStatusOpen,
				}}, nil
			},
		},
	}, json.RawMessage(`{"action":"list"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured, ok := result.Structured.(map[string]any)
	if !ok {
		t.Fatalf("structured type %T", result.Structured)
	}
	if structured["action"] != "list" {
		t.Fatalf("structured action=%v want list", structured["action"])
	}
	if structured["count"] != 1 {
		t.Fatalf("count=%v want 1", structured["count"])
	}
}

func TestHandleCreateMemberUsesSessionActorAndRegistrar(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		ProjectID: "project-1",
		SpaceID:   "space-runtime",
		Spaces: stubSpaceReader{
			getFn: func(_ context.Context, id spacedomain.SpaceID) (spacedomain.SpaceRecord, error) {
				if id != "space-runtime" {
					t.Fatalf("space id=%q want space-runtime", id)
				}
				return spacedomain.SpaceRecord{
					ID:        id,
					ProjectID: "project-1",
					Title:     "Runtime",
					Status:    spacedomain.SpaceStatusOpen,
				}, nil
			},
		},
		Members: stubMemberDirectory{
			getFn: func(ctx context.Context, id member.ID) (member.Record, error) {
				resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
				if err != nil {
					t.Fatalf("actor lookup missing caller: %v", err)
				}
				if resolved.MemberID != id || resolved.SpaceID != "space-runtime" {
					t.Fatalf("caller=%+v want member %s in space-runtime", resolved, id)
				}
				return member.Record{
					ID:             id,
					UserID:         "user-1",
					SpaceID:        "space-runtime",
					DisplayName:    "Coordinator",
					MemberType:     member.TypeCoordinator,
					LifecycleState: member.LifecycleActive,
				}, nil
			},
		},
		Registrar: stubMemberRegistrar{
			registerFn: func(ctx context.Context, rosterMember member.Record) (spaceapp.RegisterMemberResult, error) {
				resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
				if err != nil {
					t.Fatalf("register missing caller: %v", err)
				}
				if resolved.MemberID != "member-coordinator" || resolved.SpaceID != "space-runtime" {
					t.Fatalf("register caller=%+v", resolved)
				}
				if rosterMember.SpaceID != "space-runtime" {
					t.Fatalf("space id=%q want space-runtime", rosterMember.SpaceID)
				}
				if rosterMember.ProjectID != "project-1" {
					t.Fatalf("project id=%q want project-1", rosterMember.ProjectID)
				}
				if rosterMember.DisplayName != "Backend lead" {
					t.Fatalf("display name=%q want Backend lead", rosterMember.DisplayName)
				}
				if rosterMember.MemberType != member.TypeWorker {
					t.Fatalf("member type=%q want worker", rosterMember.MemberType)
				}
				if rosterMember.HarnessKind != "codex" || rosterMember.Model != "gpt-5" || rosterMember.Effort != "medium" {
					t.Fatalf("runtime config=%+v", rosterMember)
				}
				if rosterMember.PermissionMode != "" || rosterMember.ConfigRef != "" {
					t.Fatalf("permission fields should be service-owned: %+v", rosterMember)
				}
				created := rosterMember
				created.ID = "member-worker"
				created.ChannelID = "channel:space-runtime:member:member-worker"
				created.LifecycleState = member.LifecycleActive
				return spaceapp.RegisterMemberResult{Member: created, GrantedMemberType: member.TypeWorker}, nil
			},
		},
		ActorMemberID: "member-coordinator",
	}, json.RawMessage(`{"action":"member_create","display_name":"Backend lead","harness_kind":"codex","model":"gpt-5","effort":"medium"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured, ok := result.Structured.(map[string]any)
	if !ok {
		t.Fatalf("structured type %T", result.Structured)
	}
	if structured["action"] != "member_create" {
		t.Fatalf("action=%v want member_create", structured["action"])
	}
	memberResult, ok := structured["member"].(memberEntry)
	if !ok {
		t.Fatalf("member payload type %T", structured["member"])
	}
	if memberResult.ID != "member-worker" || memberResult.DisplayName != "Backend lead" {
		t.Fatalf("member result=%+v", memberResult)
	}
}

func TestHandleMemberGetUsesMemberDirectory(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		Members: stubMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				switch id {
				case "member-actor":
					return member.Record{ID: id, SpaceID: "space-runtime", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
				case "member-target":
					return member.Record{ID: id, SpaceID: "space-runtime", DisplayName: "Target", MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive}, nil
				default:
					t.Fatalf("member id=%q", id)
					return member.Record{}, nil
				}
			},
		},
		SpaceID:       "space-runtime",
		ActorMemberID: "member-actor",
	}, json.RawMessage(`{"action":"member_get","member_id":"member-target"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["action"] != "member_get" {
		t.Fatalf("action=%v want member_get", structured["action"])
	}
}

func TestHandleMemberUpdateConfigUsesRegistrar(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		Members: stubMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				return member.Record{ID: id, SpaceID: "space-runtime", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
			},
		},
		Registrar: stubMemberRegistrar{
			updateConfigFn: func(_ context.Context, id member.ID, model, effort, harnessKind string, permissionFields ...string) (member.Record, error) {
				if id != "member-target" || model != "gpt-5" || effort != "high" || harnessKind != "codex" {
					t.Fatalf("update args id=%q model=%q effort=%q harness=%q", id, model, effort, harnessKind)
				}
				if len(permissionFields) != 0 {
					t.Fatalf("permission fields=%v", permissionFields)
				}
				return member.Record{ID: id, DisplayName: "Target", MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive, Model: model, Effort: effort, HarnessKind: harnessKind}, nil
			},
		},
		SpaceID:       "space-runtime",
		ActorMemberID: "member-actor",
	}, json.RawMessage(`{"action":"member_update_config","member_id":"member-target","harness_kind":"codex","model":"gpt-5","effort":"high"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["action"] != "member_update_config" {
		t.Fatalf("action=%v want member_update_config", structured["action"])
	}
}

func TestHandleMemberCreateRequiresCoordinatorActor(t *testing.T) {
	handler := NewHandler()
	_, err := handler.Handle(context.Background(), CallContext{
		SpaceID: "space-runtime",
		Spaces:  stubSpaceReader{},
		Members: stubMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				return member.Record{ID: id, SpaceID: "space-runtime", MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive}, nil
			},
		},
		Registrar:     stubMemberRegistrar{},
		ActorMemberID: "member-worker",
	}, json.RawMessage(`{"action":"member_create","harness_kind":"codex","model":"gpt-5","effort":"medium"}`))
	if err == nil || !strings.Contains(err.Error(), "requires a coordinator member") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestHandleMemberRemoveUsesRegistrar(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		Members: stubMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				return member.Record{ID: id, SpaceID: "space-runtime", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
			},
		},
		Registrar: stubMemberRegistrar{
			removeFn: func(_ context.Context, id member.ID) (member.Record, error) {
				if id != "member-target" {
					t.Fatalf("remove id=%q want member-target", id)
				}
				return member.Record{ID: id, DisplayName: "Target", MemberType: member.TypeWorker, LifecycleState: member.LifecycleRemoved}, nil
			},
		},
		SpaceID:       "space-runtime",
		ActorMemberID: "member-actor",
	}, json.RawMessage(`{"action":"member_remove","member_id":"member-target"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["action"] != "member_remove" {
		t.Fatalf("action=%v want member_remove", structured["action"])
	}
}
