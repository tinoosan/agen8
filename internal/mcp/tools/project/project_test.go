package project

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/agen8-mcp-server/internal/caller"
	projectapp "github.com/tinoosan/agen8-mcp-server/internal/services/project/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/project/domain/member"
)

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
		ProjectID:      "project-1",
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
	registerFn     func(context.Context, member.Record) (projectapp.RegisterMemberResult, error)
	updateConfigFn func(context.Context, member.ID, string, string, string, ...string) (member.Record, error)
	removeFn       func(context.Context, member.ID) (member.Record, error)
}

func (s stubMemberRegistrar) RegisterMember(ctx context.Context, rosterMember member.Record) (projectapp.RegisterMemberResult, error) {
	if s.registerFn != nil {
		return s.registerFn(ctx, rosterMember)
	}
	rosterMember.ID = "member-created"
	rosterMember.ChannelID = "channel:" + rosterMember.ProjectID + ":member:member-created"
	rosterMember.LifecycleState = member.LifecycleActive
	return projectapp.RegisterMemberResult{Member: rosterMember, GrantedMemberType: rosterMember.MemberType}, nil
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

type stubContextRegistrar struct {
	registerFn func(context.Context, RegisterContextRequest) (RegisterContextResult, error)
}

func (s stubContextRegistrar) RegisterMCPContext(ctx context.Context, req RegisterContextRequest) (RegisterContextResult, error) {
	if s.registerFn != nil {
		return s.registerFn(ctx, req)
	}
	return RegisterContextResult{
		ProjectID:   "project-1",
		ProjectRoot: req.ProjectRoot,
		LocationID:  "local",
		MemberID:    "member-session",
		DisplayName: req.DisplayName,
		MemberType:  member.TypeWorker,
		ChannelID:   "channel:project-1:member:member-session",
		Token:       req.Token,
		URL:         "http://127.0.0.1:7777/mcp?token=" + req.Token,
		MCPServers:  []string{"agen8"},
	}, nil
}

func TestDecodeRejectsMissingAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":""}`))
	if err == nil || !strings.Contains(err.Error(), "action is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsMessageAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"bogus"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported action "bogus"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNullField(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_get","member_id":null}`))
	if err == nil || !strings.Contains(err.Error(), `field "member_id" must be omitted instead of null`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsFieldForWrongAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_get","display_name":"x","member_id":"member-1"}`))
	if err == nil || !strings.Contains(err.Error(), `field "display_name" is not valid for action "member_get"`) {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeCreateMemberRequiresRuntimeFields(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_create","harness_kind":"codex","effort":"medium"}`))
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsNonStringAction(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":123}`))
	if err == nil || !strings.Contains(err.Error(), "action must be a string") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsMalformedJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_list"`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestSchemaOmitsSpaceID(t *testing.T) {
	schema := NewHandler().Schema()
	var decoded struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	if _, ok := decoded.Properties["space_id"]; ok {
		t.Fatalf("space_id should not be present in schema")
	}
}

func TestHandleRegisterPassesDisplayNameAndReturnsGuidance(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		ContextRegistrar: stubContextRegistrar{
			registerFn: func(_ context.Context, req RegisterContextRequest) (RegisterContextResult, error) {
				if req.DisplayName != "backend engineer" {
					t.Fatalf("display name=%q want backend engineer", req.DisplayName)
				}
				if req.ProjectRoot != "/repo" {
					t.Fatalf("project root=%q want /repo", req.ProjectRoot)
				}
				return RegisterContextResult{
					ProjectID:   "project-1",
					ProjectRoot: req.ProjectRoot,
					LocationID:  "local",
					MemberID:    "member-session",
					DisplayName: req.DisplayName,
					MemberType:  member.TypeCoordinator,
					ChannelID:   "channel:project-1:member:member-session",
					Token:       req.Token,
					URL:         "http://127.0.0.1:7777/mcp?token=" + req.Token,
					MCPServers:  []string{"agen8"},
				}, nil
			},
		},
		MCPToken:    "ak_test_token",
		HarnessKind: "codex",
	}, json.RawMessage(`{"action":"register","project_root":"/repo","display_name":"backend engineer"}`))
	if err != nil {
		t.Fatalf("handle register: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["displayName"] != "backend engineer" {
		t.Fatalf("displayName=%v want backend engineer", structured["displayName"])
	}
	guidance, _ := structured["guidance"].(string)
	if !strings.Contains(guidance, "display_name") || !strings.Contains(guidance, "graph") {
		t.Fatalf("guidance=%q", guidance)
	}
}

func TestHandleMemberListUsesProjectScopedDirectory(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		Members: stubMemberDirectory{
			getFn: func(ctx context.Context, id member.ID) (member.Record, error) {
				resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
				if err != nil {
					t.Fatalf("actor lookup missing caller: %v", err)
				}
				if resolved.MemberID != id {
					t.Fatalf("caller=%+v want member %s", resolved, id)
				}
				return member.Record{
					ID:             id,
					UserID:         "user-1",
					ProjectID:      "project-1",
					DisplayName:    string(id),
					MemberType:     member.TypeWorker,
					LifecycleState: member.LifecycleActive,
				}, nil
			},
			listFn: func(ctx context.Context, filter member.Filter) ([]member.Record, error) {
				resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
				if err != nil {
					t.Fatalf("list missing caller: %v", err)
				}
				if string(resolved.ProjectID) != "project-1" {
					t.Fatalf("caller project=%q want project-1", resolved.ProjectID)
				}
				if filter.ProjectID != "project-1" {
					t.Fatalf("filter project id=%q want project-1", filter.ProjectID)
				}
				if filter.LifecycleState != member.LifecycleActive {
					t.Fatalf("filter lifecycle=%q want active", filter.LifecycleState)
				}
				return []member.Record{{
					ID:             "member-worker",
					ProjectID:      "project-1",
					ChannelID:      "channel:project-1:member:member-worker",
					DisplayName:    "Worker",
					MemberType:     member.TypeWorker,
					LifecycleState: member.LifecycleActive,
				}}, nil
			},
		},
		ProjectID:     "project-1",
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

func TestHandleCreateMemberUsesSessionActorAndRegistrar(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		ProjectID: "project-1",
		Members: stubMemberDirectory{
			getFn: func(ctx context.Context, id member.ID) (member.Record, error) {
				resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
				if err != nil {
					t.Fatalf("actor lookup missing caller: %v", err)
				}
				if resolved.MemberID != id {
					t.Fatalf("caller=%+v want member %s", resolved, id)
				}
				return member.Record{
					ID:             id,
					UserID:         "user-1",
					ProjectID:      "project-1",
					DisplayName:    "Coordinator",
					MemberType:     member.TypeCoordinator,
					LifecycleState: member.LifecycleActive,
				}, nil
			},
		},
		Registrar: stubMemberRegistrar{
			registerFn: func(ctx context.Context, rosterMember member.Record) (projectapp.RegisterMemberResult, error) {
				resolved, err := (caller.ContextResolver{}).ResolveCaller(ctx)
				if err != nil {
					t.Fatalf("register missing caller: %v", err)
				}
				if resolved.MemberID != "member-coordinator" || string(resolved.ProjectID) != "project-1" {
					t.Fatalf("register caller=%+v", resolved)
				}
				if rosterMember.ProjectID != "project-1" {
					t.Fatalf("project id=%q want project-1", rosterMember.ProjectID)
				}
				if rosterMember.DisplayName != "Backend lead" {
					t.Fatalf("display name=%q want Backend lead", rosterMember.DisplayName)
				}
				if rosterMember.MemberType != member.TypeCoordinator {
					t.Fatalf("member type=%q want coordinator", rosterMember.MemberType)
				}
				if rosterMember.HarnessKind != "codex" || rosterMember.Model != "gpt-5" || rosterMember.Effort != "medium" {
					t.Fatalf("runtime config=%+v", rosterMember)
				}
				if rosterMember.PermissionMode != "" || rosterMember.ConfigRef != "" {
					t.Fatalf("permission fields should be service-owned: %+v", rosterMember)
				}
				created := rosterMember
				created.ID = "member-worker"
				created.ChannelID = "channel:project-1:member:member-worker"
				created.LifecycleState = member.LifecycleActive
				return projectapp.RegisterMemberResult{Member: created, GrantedMemberType: member.TypeCoordinator}, nil
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
					return member.Record{ID: id, ProjectID: "project-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
				case "member-target":
					return member.Record{ID: id, ProjectID: "project-1", DisplayName: "Target", MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive}, nil
				default:
					t.Fatalf("member id=%q", id)
					return member.Record{}, nil
				}
			},
		},
		ProjectID:     "project-1",
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
				return member.Record{ID: id, ProjectID: "project-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
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
		ProjectID:     "project-1",
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

func TestHandleMemberCreateAllowsActiveProjectMemberActor(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		ProjectID: "project-1",
		Members: stubMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				return member.Record{ID: id, ProjectID: "project-1", MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive}, nil
			},
		},
		Registrar:     stubMemberRegistrar{},
		ActorMemberID: "member-worker",
	}, json.RawMessage(`{"action":"member_create","harness_kind":"codex","model":"gpt-5","effort":"medium"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["action"] != "member_create" {
		t.Fatalf("action=%v want member_create", structured["action"])
	}
}

func TestHandleMemberRemoveUsesRegistrar(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		Members: stubMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				return member.Record{ID: id, ProjectID: "project-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
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
		ProjectID:     "project-1",
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
