package project

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/caller"
	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	"github.com/tinoosan/agen8/internal/services/project/domain/member"
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
	registerFn func(context.Context, member.Record) (projectapp.RegisterMemberResult, error)
	updateFn   func(context.Context, member.ID, string) (member.Record, error)
	removeFn   func(context.Context, member.ID) (member.Record, error)
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

func (s stubMemberRegistrar) UpdateMember(ctx context.Context, id member.ID, displayName string) (member.Record, error) {
	if s.updateFn != nil {
		return s.updateFn(ctx, id, displayName)
	}
	return member.Record{ID: id, DisplayName: displayName, LifecycleState: member.LifecycleActive}, nil
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

func TestDecodeCreateMemberRequiresDisplayName(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_create"}`))
	if err == nil || !strings.Contains(err.Error(), "display_name is required") {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestDecodeRejectsLegacyMemberUpdateConfig(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_update_config","member_id":"member-1","model":"gpt-5","effort":"high","harness_kind":"codex"}`))
	if err == nil || !strings.Contains(err.Error(), `unsupported action "member_update_config"`) {
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

// TestDecodeRejectsTrailingJSON feeds trailing JSON after a valid object. The
// json.Unmarshal-based validateActionFields runs first and rejects any trailing
// token, so the input surfaces as the generic "invalid arguments" error and never
// reaches a trailing-JSON guard. This proves the old guard was unreachable; it was
// removed as dead code (see dec-21debbd9). Trailing input is still rejected loudly.
func TestDecodeRejectsTrailingJSON(t *testing.T) {
	_, err := decode(json.RawMessage(`{"action":"member_list"} {}`))
	if err == nil || !strings.Contains(err.Error(), "invalid arguments") {
		t.Fatalf("unexpected err=%v", err)
	}
	if strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing-JSON guard was expected unreachable, but its message surfaced: %v", err)
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
	for _, field := range []string{"harness_kind", "model", "effort", "permission_mode", "config_ref"} {
		if _, ok := decoded.Properties[field]; ok {
			t.Fatalf("%s should not be present in schema", field)
		}
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
				if req.Model != "" || req.Effort != "" || req.PermissionMode != "" || req.ConfigRef != "" {
					t.Fatalf("runtime fields should not be caller supplied: %+v", req)
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
	if !strings.Contains(guidance, "registered") || !strings.Contains(guidance, "Agen8 derives") {
		t.Fatalf("guidance=%q", guidance)
	}
}

func TestHandleRegisterReturnsAlreadyRegisteredGuidance(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		ContextRegistrar: stubContextRegistrar{
			registerFn: func(_ context.Context, req RegisterContextRequest) (RegisterContextResult, error) {
				return RegisterContextResult{
					ProjectID:         "project-1",
					ProjectRoot:       req.ProjectRoot,
					LocationID:        "local",
					MemberID:          "member-session",
					DisplayName:       "Atlas (Backend Engineer)",
					MemberType:        member.TypeCoordinator,
					ChannelID:         "channel:project-1:member:member-session",
					Token:             req.Token,
					URL:               "http://127.0.0.1:7777/mcp?token=" + req.Token,
					MCPServers:        []string{"agen8"},
					AlreadyRegistered: true,
				}, nil
			},
		},
		MCPToken:    "ak_test_token",
		HarnessKind: "codex",
	}, json.RawMessage(`{"action":"register","project_root":"/repo","display_name":"Kepler (Backend Engineer)"}`))
	if err != nil {
		t.Fatalf("handle register: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["alreadyRegistered"] != true {
		t.Fatalf("alreadyRegistered=%v want true", structured["alreadyRegistered"])
	}
	guidance, _ := structured["guidance"].(string)
	if !strings.Contains(guidance, "already registered as Atlas (Backend Engineer) (member-session)") || !strings.Contains(guidance, "member_update") {
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
				if resolved.MemberID != string(id) {
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
					HarnessKind:    "codex",
					Model:          "gpt-5",
					Effort:         "high",
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
	encoded, err := json.Marshal(structured)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	// harnessKind is auto-determined server-side and is meant to be visible (it
	// drives the roster + harness leaderboard). model/effort stay hidden: they are
	// not auto-determined, so exposing them would surface fabricated or stale data.
	if !strings.Contains(string(encoded), `"harnessKind":"codex"`) {
		t.Fatalf("member_list should expose harnessKind, got %s", encoded)
	}
	for _, forbidden := range []string{"model", "effort"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("member_list leaked %s in %s", forbidden, encoded)
		}
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
				if resolved.MemberID != string(id) {
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
				if rosterMember.HarnessKind != "codex" || rosterMember.Model != "" || rosterMember.Effort != "" {
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
		HarnessKind:   "codex",
	}, json.RawMessage(`{"action":"member_create","display_name":"Backend lead"}`))
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
	// member_create returns a lean ack: the model supplied the display name, so
	// it only needs the new id (and lifecycle state) back.
	memberResult, ok := structured["member"].(memberAck)
	if !ok {
		t.Fatalf("member payload type %T", structured["member"])
	}
	if memberResult.ID != "member-worker" {
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

func TestHandleMemberUpdateUsesRegistrar(t *testing.T) {
	handler := NewHandler()
	result, err := handler.Handle(context.Background(), CallContext{
		Members: stubMemberDirectory{
			getFn: func(_ context.Context, id member.ID) (member.Record, error) {
				return member.Record{ID: id, ProjectID: "project-1", MemberType: member.TypeCoordinator, LifecycleState: member.LifecycleActive}, nil
			},
		},
		Registrar: stubMemberRegistrar{
			updateFn: func(_ context.Context, id member.ID, displayName string) (member.Record, error) {
				if id != "member-target" || displayName != "Target Renamed" {
					t.Fatalf("update args id=%q displayName=%q", id, displayName)
				}
				return member.Record{ID: id, DisplayName: displayName, MemberType: member.TypeWorker, LifecycleState: member.LifecycleActive}, nil
			},
		},
		ProjectID:     "project-1",
		ActorMemberID: "member-actor",
	}, json.RawMessage(`{"action":"member_update","member_id":"member-target","display_name":"Target Renamed"}`))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	structured := result.Structured.(map[string]any)
	if structured["action"] != "member_update" {
		t.Fatalf("action=%v want member_update", structured["action"])
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
		HarnessKind:   "codex",
	}, json.RawMessage(`{"action":"member_create","display_name":"Peer"}`))
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
