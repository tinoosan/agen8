package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func TestGuardMiddleware_ShortCircuits(t *testing.T) {
	called := false
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		called = true
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})
	exec := ChainExecutor(base, &guardMiddleware{
		guard: func(req types.HostOpRequest) *types.HostOpResponse {
			return &types.HostOpResponse{Op: req.Op, Ok: false, Error: "blocked"}
		},
	})
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSRead, Path: "/workspace"})
	if called {
		t.Fatalf("expected base executor not to run")
	}
	if resp.Ok || resp.Error != "blocked" {
		t.Fatalf("expected blocked response, got %+v", resp)
	}
}

func TestMakeTeamWorkerWriteGuard_RejectsRolePrefixedWorkspaceWrites(t *testing.T) {
	guard := makeTeamWorkerWriteGuard("frontend-engineer")
	if guard == nil {
		t.Fatalf("expected guard")
	}
	resp := guard(nil, types.HostOpRequest{Op: types.HostOpFSWrite, Path: "/workspace/frontend-engineer/report.md", Text: "x"})
	if resp == nil || resp.Ok {
		t.Fatalf("expected write rejection, got %+v", resp)
	}
	resp = guard(nil, types.HostOpRequest{Op: types.HostOpFSAppend, Path: "/workspace/frontend-engineer/notes.md", Text: "x"})
	if resp == nil || resp.Ok {
		t.Fatalf("expected append rejection, got %+v", resp)
	}
	resp = guard(nil, types.HostOpRequest{Op: types.HostOpFSEdit, Path: "/workspace/frontend-engineer/notes.md"})
	if resp == nil || resp.Ok {
		t.Fatalf("expected edit rejection, got %+v", resp)
	}
	resp = guard(nil, types.HostOpRequest{Op: types.HostOpFSPatch, Path: "/workspace/frontend-engineer/notes.md", Text: "---"})
	if resp == nil || resp.Ok {
		t.Fatalf("expected patch rejection, got %+v", resp)
	}
}

func TestMakeTeamWorkerWriteGuard_AllowsCanonicalWorkspacePaths(t *testing.T) {
	guard := makeTeamWorkerWriteGuard("frontend-engineer")
	if guard == nil {
		t.Fatalf("expected guard")
	}
	if resp := guard(nil, types.HostOpRequest{Op: types.HostOpFSWrite, Path: "/workspace/report.md", Text: "x"}); resp != nil {
		t.Fatalf("expected canonical workspace write allowed, got %+v", resp)
	}
	if resp := guard(nil, types.HostOpRequest{Op: types.HostOpFSWrite, Path: "/workspace/docs/report.md", Text: "x"}); resp != nil {
		t.Fatalf("expected nested non-role path allowed, got %+v", resp)
	}
}

func TestDiffMiddleware_EmitsPatchPreview(t *testing.T) {
	dir := t.TempDir()
	res, err := resources.NewWorkdirResource(dir)
	if err != nil {
		t.Fatalf("workdir resource: %v", err)
	}
	fs := vfs.NewFS()
	if err := fs.Mount(vfs.MountProject, res); err != nil {
		t.Fatalf("mount project: %v", err)
	}

	base := &agent.HostOpExecutor{FS: fs, DefaultMaxBytes: 4096}

	var got events.Event
	exec := NewExecutor(base, ExecutorOptions{
		Emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				got = ev
			}
		},
		FS: fs,
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: "/project/hello.txt",
		Text: "hello",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	preview := got.Data["patchPreview"]
	if !strings.Contains(preview, "+hello") {
		t.Fatalf("expected patch preview to include write diff, got %q", preview)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("temp dir should exist: %v", err)
	}
}

func TestEventMiddleware_EmailReqDataIncludesToAndSubject(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:     &seq,
		metaKey: opContextKey{},
	})

	input, err := json.Marshal(map[string]string{
		"to":      "team@example.com",
		"subject": "Build completed",
		"body":    "done",
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpEmail,
		Input: input,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := strings.TrimSpace(gotReq.Data["to"]); got != "team@example.com" {
		t.Fatalf("expected reqData.to to be set, got %q", got)
	}
	if got := strings.TrimSpace(gotReq.Data["subject"]); got != "Build completed" {
		t.Fatalf("expected reqData.subject to be set, got %q", got)
	}
	if _, ok := gotReq.Data["body"]; ok {
		t.Fatalf("expected reqData.body to be omitted")
	}
}

func TestEventMiddleware_FSReadMaxBytesFromOperation(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:       types.HostOpFSRead,
		Path:     "/workspace/file.txt",
		MaxBytes: 123,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := gotReq.Data["maxBytes"]; got != "123" {
		t.Fatalf("expected reqData.maxBytes=123, got %q", got)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSRead,
		Path: "/workspace/file.txt",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if _, ok := gotReq.Data["maxBytes"]; ok {
		t.Fatalf("expected reqData.maxBytes to be omitted when zero")
	}
}

func TestEventMiddleware_HTTPFetchRequestEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	var gotReq events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpHTTPFetch,
		URL:    "https://example.com",
		Method: "post",
		Body:   strings.Repeat("x", maxHTTPBodyPreviewBytes+100),
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := gotReq.Data["method"]; got != "POST" {
		t.Fatalf("expected method normalization to POST, got %q", got)
	}
	if got := gotReq.StoreData["method"]; got != "POST" {
		t.Fatalf("expected store method normalization to POST, got %q", got)
	}
	if got := gotReq.Data["bodyTruncated"]; got != "true" {
		t.Fatalf("expected bodyTruncated=true, got %q", got)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpHTTPFetch,
		URL:  "https://example.com",
		Body: "Authorization: Bearer sk-SECRET",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := gotReq.Data["method"]; got != "GET" {
		t.Fatalf("expected empty method default GET, got %q", got)
	}
	if got := gotReq.Data["body"]; got != "<omitted>" {
		t.Fatalf("expected redacted request body, got %q", got)
	}
	if got := gotReq.StoreData["body"]; got != "<omitted>" {
		t.Fatalf("expected redacted stored request body, got %q", got)
	}
}

func TestEventMiddleware_HTTPFetchResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{
			Op:       req.Op,
			Ok:       true,
			Status:   201,
			FinalURL: "https://example.com/final",
			Body:     strings.Repeat("z", 1200),
		}
	})

	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:  types.HostOpHTTPFetch,
		URL: "https://example.com",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := gotResp.Data["status"]; got != "201" {
		t.Fatalf("expected status=201, got %q", got)
	}
	if got := gotResp.Data["finalUrl"]; got != "https://example.com/final" {
		t.Fatalf("expected finalUrl to be set, got %q", got)
	}
	if got := gotResp.Data["bodyTruncated"]; got != "true" {
		t.Fatalf("expected bodyTruncated=true, got %q", got)
	}
	if got := gotResp.StoreData["status"]; got != "201" {
		t.Fatalf("expected store status=201, got %q", got)
	}
}

func TestDispatchMiddleware_UnregisteredOpFallsBackToBase(t *testing.T) {
	called := false
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		called = true
		return types.HostOpResponse{Op: req.Op, Ok: true}
	})

	exec := ChainExecutor(base, &dispatchMiddleware{
		operations: newHostOperationRegistry(nil),
	})

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSSearch,
		Path: "/workspace",
	})
	if !called {
		t.Fatalf("expected base executor to run for unregistered op")
	}
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
}

func TestEventMiddleware_AgentSpawnNoopIsReclassifiedAndEnriched(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		return types.HostOpResponse{Op: req.Op, Ok: true, Text: "child done"}
	})

	var gotReq events.Event
	var gotResp events.Event
	seq := uint64(0)
	exec := ChainExecutor(base, &eventMiddleware{
		emit: func(ctx context.Context, ev events.Event) {
			if ev.Type == "agent.op.request" {
				gotReq = ev
			}
			if ev.Type == "agent.op.response" {
				gotResp = ev
			}
		},
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
	})

	input, err := json.Marshal(map[string]any{
		"goal":            "Compute 40+2",
		"model":           "gpt-5-mini",
		"maxTokens":       128,
		"backgroundCount": 2,
		"currentDepth":    0,
		"maxDepth":        3,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}

	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpNoop,
		Action: "agent_spawn",
		Input:  input,
		Text:   "child done",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if got := strings.TrimSpace(gotReq.Data["op"]); got != "agent_spawn" {
		t.Fatalf("expected request op to be reclassified as agent_spawn, got %q", got)
	}
	if got := strings.TrimSpace(gotReq.Data["goal"]); got != "Compute 40+2" {
		t.Fatalf("expected request goal to be set, got %q", got)
	}
	if got := strings.TrimSpace(gotResp.Data["op"]); got != "agent_spawn" {
		t.Fatalf("expected response op to be reclassified as agent_spawn, got %q", got)
	}
	if got := strings.TrimSpace(gotResp.Data["outputPreview"]); got != "child done" {
		t.Fatalf("expected response outputPreview to be set, got %q", got)
	}
}
