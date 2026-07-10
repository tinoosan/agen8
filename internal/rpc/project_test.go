package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/internal/caller"
	"github.com/tinoosan/agen8/internal/core/types"
	projectapp "github.com/tinoosan/agen8/internal/services/project/app"
	projectinfra "github.com/tinoosan/agen8/internal/services/project/infra"
	projectrpc "github.com/tinoosan/agen8/internal/services/project/rpc"
	storagedb "github.com/tinoosan/agen8/internal/storage/db"
)

func TestRegisterProjectDispatchCreateAndList(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	if err := RegisterProject(reg, svc, nil, nil); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.create",
		"params": {
			"root": "/tmp/project-1",
			"title": "Project One"
		}
	}`))
	if err != nil {
		t.Fatalf("Handle project.create returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.create response error=%+v", resp.Error)
	}
	var saved projectrpc.ProjectCreateResult
	if err := json.Unmarshal(resp.Result, &saved); err != nil {
		t.Fatalf("unmarshal project.create result: %v", err)
	}
	if saved.Project.ID == "" || saved.Project.Status != "open" {
		t.Fatalf("project.create result=%+v", saved.Project)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "project.list",
		"params": {}
	}`))
	if err != nil {
		t.Fatalf("Handle project.list returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.list response error=%+v", resp.Error)
	}
	var listed projectrpc.ProjectListResult
	if err := json.Unmarshal(resp.Result, &listed); err != nil {
		t.Fatalf("unmarshal project.list result: %v", err)
	}
	if len(listed.Projects) != 1 || listed.Projects[0].ID != saved.Project.ID {
		t.Fatalf("project.list result=%+v", listed.Projects)
	}
}

func TestRegisterProjectReturnsBestEffortSetupMetadata(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	postCreate := func(_ context.Context, userID, projectID, title, locationID, root string) projectrpc.ProjectSetupResult {
		if userID != "user-1" || title != "Project One" || locationID != "local" || root != "/tmp/project-1" {
			t.Fatalf("post-create args=(%q,%q,%q,%q,%q)", userID, projectID, title, locationID, root)
		}
		if projectID == "" {
			t.Fatal("post-create project id is empty")
		}
		return projectrpc.ProjectSetupResult{
			Attempted:      true,
			HooksInstalled: true,
			Warnings:       []string{"Claude MCP could not be configured on the daemon machine."},
		}
	}
	if err := RegisterProject(reg, svc, postCreate, nil); err != nil {
		t.Fatalf("RegisterProject: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})
	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc":"2.0","id":"1","method":"project.create",
		"params":{"locationId":"local","root":"/tmp/project-1","title":"Project One"}
	}`))
	if err != nil {
		t.Fatalf("project.create: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.create response error=%+v", resp.Error)
	}
	var created projectrpc.ProjectCreateResult
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		t.Fatalf("decode project.create: %v", err)
	}
	if created.Project.ID == "" {
		t.Fatal("project was not created")
	}
	if created.Setup == nil || !created.Setup.Attempted || !created.Setup.HooksInstalled || created.Setup.ClaudeMCPConfigured {
		t.Fatalf("setup=%+v", created.Setup)
	}
	if len(created.Setup.Warnings) != 1 {
		t.Fatalf("setup warnings=%v", created.Setup.Warnings)
	}
}

func TestRegisterProjectConfigureClaudeMCP(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	var gotUserID, gotProjectID, gotTitle, gotRoot string
	configure := func(ctx context.Context, userID, projectID, projectTitle, root string) (projectrpc.ProjectClaudeMCPConfigureResult, error) {
		gotProjectID = projectID
		gotUserID, gotTitle, gotRoot = userID, projectTitle, root
		return projectrpc.ProjectClaudeMCPConfigureResult{
			Installed:  true,
			Path:       filepath.Join(root, ".claude", "settings.local.json"),
			ServerName: "agen8",
			URL:        "http://127.0.0.1:7777/mcp",
		}, nil
	}
	if err := RegisterProject(reg, svc, nil, configure); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.create",
		"params": {
			"root": "/tmp/project-1",
			"title": "Project One"
		}
	}`))
	if err != nil {
		t.Fatalf("Handle project.create returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.create response error=%+v", resp.Error)
	}
	var created projectrpc.ProjectCreateResult
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		t.Fatalf("unmarshal project.create result: %v", err)
	}

	raw, err = server.Handle(ctx, []byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "project.claudeMCP.configure",
		"params": { "projectId": %q }
	}`, created.Project.ID)))
	if err != nil {
		t.Fatalf("Handle project.claudeMCP.configure returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.claudeMCP.configure response error=%+v", resp.Error)
	}
	var configured projectrpc.ProjectClaudeMCPConfigureResult
	if err := json.Unmarshal(resp.Result, &configured); err != nil {
		t.Fatalf("unmarshal configure result: %v", err)
	}
	if gotUserID != "user-1" || gotProjectID != created.Project.ID || gotTitle != "Project One" || gotRoot != "/tmp/project-1" {
		t.Fatalf("callback got user=%q project=%q title=%q root=%q", gotUserID, gotProjectID, gotTitle, gotRoot)
	}
	if configured.ProjectID != created.Project.ID || !configured.Installed || configured.ServerName != "agen8" {
		t.Fatalf("configured=%+v", configured)
	}
}

func TestRegisterProjectMapsInvalidParams(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	if err := RegisterProject(reg, svc, nil, nil); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.get",
		"params": {}
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidParams {
		t.Fatalf("response error=%+v want invalid params", resp.Error)
	}
}

func TestRegisterProjectRelocatesOnlyAfterDirectoryValidation(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	validatedLocation := ""
	validatedRoot := ""
	validator := func(_ context.Context, locationID, root string) error {
		validatedLocation = locationID
		validatedRoot = root
		if root == "/missing" {
			return fmt.Errorf("directory does not exist")
		}
		return nil
	}
	if err := RegisterProject(reg, svc, nil, nil, validator); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	createdRaw, err := server.Handle(ctx, []byte(`{"jsonrpc":"2.0","id":"1","method":"project.create","params":{"root":"/old","title":"Stable"}}`))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	createdResponse := decodeRPCResponse(t, createdRaw)
	var created projectrpc.ProjectCreateResult
	if err := json.Unmarshal(createdResponse.Result, &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	relocateRaw, err := server.Handle(ctx, []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"project.relocate","params":{"projectId":%q,"root":"/new"}}`, created.Project.ID)))
	if err != nil {
		t.Fatalf("relocate: %v", err)
	}
	relocateResponse := decodeRPCResponse(t, relocateRaw)
	if relocateResponse.Error != nil {
		t.Fatalf("relocate response error=%+v", relocateResponse.Error)
	}
	var relocated projectrpc.ProjectRelocateResult
	if err := json.Unmarshal(relocateResponse.Result, &relocated); err != nil {
		t.Fatalf("decode relocate: %v", err)
	}
	if relocated.Project.ID != created.Project.ID || relocated.Project.Root != "/new" {
		t.Fatalf("relocated=%+v", relocated.Project)
	}
	if validatedLocation != "local" || validatedRoot != "/new" {
		t.Fatalf("validator location=%q root=%q", validatedLocation, validatedRoot)
	}

	missingRaw, err := server.Handle(ctx, []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":"3","method":"project.relocate","params":{"projectId":%q,"root":"/missing"}}`, created.Project.ID)))
	if err != nil {
		t.Fatalf("relocate missing: %v", err)
	}
	missingResponse := decodeRPCResponse(t, missingRaw)
	if missingResponse.Error == nil || missingResponse.Error.Code != CodeInvalidParams {
		t.Fatalf("missing response error=%+v", missingResponse.Error)
	}
	stored, err := svc.GetProject(caller.ContextWithCaller(context.Background(), caller.Caller{UserID: "user-1"}), types.ProjectID(created.Project.ID))
	if err != nil || stored.Root() != "/new" {
		t.Fatalf("failed validation changed root=%q err=%v", stored.Root(), err)
	}
}

func TestRegisterProjectArchiveThenDelete(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	if err := RegisterProject(reg, svc, nil, nil); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ctx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.create",
		"params": { "root": "/tmp/archive-me", "title": "Archive Me" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.create returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.create response error=%+v", resp.Error)
	}
	var created projectrpc.ProjectCreateResult
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		t.Fatalf("unmarshal project.create result: %v", err)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "project.delete",
		"params": { "projectId": "`+created.Project.ID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.delete returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error == nil {
		t.Fatalf("expected delete before archive to fail")
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "3",
		"method": "project.archive",
		"params": { "projectId": "`+created.Project.ID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.archive returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.archive response error=%+v", resp.Error)
	}
	var archived projectrpc.ProjectArchiveResult
	if err := json.Unmarshal(resp.Result, &archived); err != nil {
		t.Fatalf("unmarshal project.archive result: %v", err)
	}
	if archived.Project.Status != "archived" {
		t.Fatalf("archived project=%+v", archived.Project)
	}

	raw, err = server.Handle(ctx, []byte(`{
		"jsonrpc": "2.0",
		"id": "4",
		"method": "project.delete",
		"params": { "projectId": "`+created.Project.ID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.delete returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.delete response error=%+v", resp.Error)
	}
}

func TestRegisterProjectMemberRPCWorksAfterMCPRehome(t *testing.T) {
	svc := newRPCProjectService(t)
	root := filepath.Join(t.TempDir(), "repo")
	ctx := context.Background()

	legacy, err := svc.RegisterMCPContext(ctx, projectapp.RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "local",
		ProjectRoot: root,
		DisplayName: "codex",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("legacy register: %v", err)
	}
	registered, err := svc.RegisterMCPContext(ctx, projectapp.RegisterMCPContextInput{
		Token:       "ak_test_token",
		UserID:      "user-1",
		ProjectRoot: root,
		DisplayName: "Codex backend engineer",
		HarnessKind: "codex",
		SessionID:   "session-1",
	})
	if err != nil {
		t.Fatalf("register real user: %v", err)
	}
	if registered.MemberID != legacy.MemberID {
		t.Fatalf("member id changed from %q to %q", legacy.MemberID, registered.MemberID)
	}

	reg := NewRegistry()
	if err := RegisterProject(reg, svc, nil, nil); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	rpcCtx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(rpcCtx, []byte(`{
		"jsonrpc": "2.0",
		"id": "member-get",
		"method": "project.member.get",
		"params": { "memberId": "`+registered.MemberID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.member.get returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.member.get response error=%+v", resp.Error)
	}
	var got projectrpc.MemberGetResult
	if err := json.Unmarshal(resp.Result, &got); err != nil {
		t.Fatalf("unmarshal project.member.get result: %v", err)
	}
	if got.Member.DisplayName != "codex" || got.Member.UserID != "user-1" {
		t.Fatalf("member.get result=%+v", got.Member)
	}
	encodedMemberGet, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal member.get result: %v", err)
	}
	// harnessKind is auto-determined server-side and is exposed on purpose (roster +
	// harness leaderboard). model/effort and the runtime-config fields stay hidden:
	// they are not auto-determined, so surfacing them would leak fabricated/stale data.
	if !strings.Contains(string(encodedMemberGet), `"harnessKind":"codex"`) {
		t.Fatalf("member.get should expose harnessKind, got %s", encodedMemberGet)
	}
	for _, forbidden := range []string{"model", "effort", "harnessPermissionMode", "harnessConfigRef"} {
		if strings.Contains(string(encodedMemberGet), forbidden) {
			t.Fatalf("member.get leaked %s in %s", forbidden, encodedMemberGet)
		}
	}

	raw, err = server.Handle(rpcCtx, []byte(`{
		"jsonrpc": "2.0",
		"id": "member-list",
		"method": "project.member.list",
		"params": { "projectId": "`+registered.ProjectID+`" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.member.list returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.member.list response error=%+v", resp.Error)
	}
	var listed projectrpc.MemberListResult
	if err := json.Unmarshal(resp.Result, &listed); err != nil {
		t.Fatalf("unmarshal project.member.list result: %v", err)
	}
	if len(listed.Members) != 1 || listed.Members[0].ID != registered.MemberID || listed.Members[0].DisplayName != "codex" {
		t.Fatalf("member.list result=%+v", listed.Members)
	}
	encodedMemberList, err := json.Marshal(listed)
	if err != nil {
		t.Fatalf("marshal member.list result: %v", err)
	}
	if !strings.Contains(string(encodedMemberList), `"harnessKind":"codex"`) {
		t.Fatalf("member.list should expose harnessKind, got %s", encodedMemberList)
	}
	for _, forbidden := range []string{"model", "effort", "harnessPermissionMode", "harnessConfigRef"} {
		if strings.Contains(string(encodedMemberList), forbidden) {
			t.Fatalf("member.list leaked %s in %s", forbidden, encodedMemberList)
		}
	}

	raw, err = server.Handle(rpcCtx, []byte(`{
		"jsonrpc": "2.0",
		"id": "member-update",
		"method": "project.member.update",
		"params": { "memberId": "`+registered.MemberID+`", "displayName": "Kepler (Backend Engineer)" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.member.update returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.member.update response error=%+v", resp.Error)
	}
	var updated projectrpc.MemberUpdateResult
	if err := json.Unmarshal(resp.Result, &updated); err != nil {
		t.Fatalf("unmarshal project.member.update result: %v", err)
	}
	if updated.Member.ID != registered.MemberID || updated.Member.DisplayName != "Kepler (Backend Engineer)" {
		t.Fatalf("member.update result=%+v", updated.Member)
	}
}

func TestRegisterProjectDispatchLinkTokenCreate(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	if err := RegisterProject(reg, svc, nil, nil); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	ownerCtx := ContextWithIdentity(context.Background(), Identity{UserID: "user-1"})

	raw, err := server.Handle(ownerCtx, []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.create",
		"params": { "root": "/tmp/link-me", "title": "Link Me" }
	}`))
	if err != nil {
		t.Fatalf("Handle project.create returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.create response error=%+v", resp.Error)
	}
	var created projectrpc.ProjectCreateResult
	if err := json.Unmarshal(resp.Result, &created); err != nil {
		t.Fatalf("unmarshal project.create result: %v", err)
	}

	raw, err = server.Handle(ownerCtx, []byte(fmt.Sprintf(`{
		"jsonrpc": "2.0",
		"id": "2",
		"method": "project.linkToken.create",
		"params": { "projectId": %q, "label": "laptop" }
	}`, created.Project.ID)))
	if err != nil {
		t.Fatalf("Handle project.linkToken.create returned error: %v", err)
	}
	resp = decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("project.linkToken.create response error=%+v", resp.Error)
	}
	var result projectrpc.LinkTokenCreateResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal project.linkToken.create result: %v", err)
	}
	if !strings.HasPrefix(result.Token, "wlt_") {
		t.Fatalf("link token=%q want wlt_ prefix", result.Token)
	}
	if result.ProjectID != created.Project.ID {
		t.Fatalf("result.ProjectID=%q want %q", result.ProjectID, created.Project.ID)
	}
	if result.ID == "" {
		t.Fatalf("result.ID is empty: %+v", result)
	}
}

func TestRegisterProjectLinkTokenCreateRequiresIdentity(t *testing.T) {
	svc := newRPCProjectService(t)
	reg := NewRegistry()
	if err := RegisterProject(reg, svc, nil, nil); err != nil {
		t.Fatalf("RegisterProject returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	// No identity in context: the withProjectCaller guard must reject it before
	// any token is minted.
	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "project.linkToken.create",
		"params": { "projectId": "any-project" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInvalidRequest {
		t.Fatalf("response error=%+v want invalid request for missing identity", resp.Error)
	}
}

func newRPCProjectService(t *testing.T) *projectapp.Service {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:  storagedb.DriverSQLite,
		DataDir: filepath.Join(t.TempDir(), "data"),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	projects, err := projectinfra.NewSQLiteRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	members, err := projectinfra.NewMemberSQLiteRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewMemberSQLiteRepository: %v", err)
	}
	workspaces, err := projectinfra.NewWorkspaceSQLiteRepository(handle.DB())
	if err != nil {
		t.Fatalf("NewWorkspaceSQLiteRepository: %v", err)
	}
	svc, err := projectapp.NewService(projectapp.Config{
		Projects:   projects,
		Members:    members,
		Workspaces: workspaces,
		LinkTokens: rpcProjectLinkTokenIssuer{},
		Caller:     caller.ContextResolver{},
		Configs:    rpcProjectConfigValidator{},
		Events:     rpcProjectEventPublisher{},
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

type rpcProjectConfigValidator struct{}

func (rpcProjectConfigValidator) ValidateConfig(string, string, string) error { return nil }

type rpcProjectEventPublisher struct{}

func (rpcProjectEventPublisher) Publish(string, any) error { return nil }

type rpcProjectLinkTokenIssuer struct{}

func (rpcProjectLinkTokenIssuer) IssueLinkToken(_ context.Context, req projectapp.LinkTokenRequest) (projectapp.LinkTokenIssued, error) {
	token := "wlt_" + req.ProjectID + "secret"
	prefix := token
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	return projectapp.LinkTokenIssued{
		ID:        "link_token_" + req.ProjectID,
		Prefix:    prefix,
		Token:     token,
		UserID:    req.UserID,
		ProjectID: req.ProjectID,
		Label:     req.Label,
	}, nil
}

func (rpcProjectLinkTokenIssuer) ListLinkTokens(_ context.Context, _ string) ([]projectapp.LinkTokenSummary, error) {
	return nil, nil
}

func (rpcProjectLinkTokenIssuer) RevokeLinkToken(_ context.Context, _ string) error {
	return nil
}
