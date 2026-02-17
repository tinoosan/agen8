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
		seq:        &seq,
		metaKey:    opContextKey{},
		operations: newHostOperationRegistry(nil),
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
		Op:   "custom_op",
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

func TestEventMiddleware_EmitsRequestTextForRepresentativeOps(t *testing.T) {
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

	emailInput, err := json.Marshal(map[string]string{
		"to":      "team@example.com",
		"subject": "Daily report",
		"body":    "done",
	})
	if err != nil {
		t.Fatalf("marshal email input: %v", err)
	}

	taskCreateInput, err := json.Marshal(map[string]any{
		"goal":       "Write integration tests",
		"taskId":     "task-1",
		"childRunId": "run-1",
	})
	if err != nil {
		t.Fatalf("marshal task_create input: %v", err)
	}

	tests := []struct {
		name string
		req  types.HostOpRequest
		want string
	}{
		{
			name: "shell_exec",
			req:  types.HostOpRequest{Op: types.HostOpShellExec, Argv: []string{"rg", "-n", "todo"}},
			want: "rg -n todo",
		},
		{
			name: "http_fetch",
			req:  types.HostOpRequest{Op: types.HostOpHTTPFetch, URL: "https://example.com", Method: "GET"},
			want: "GET https://example.com",
		},
		{
			name: "browser",
			req:  types.HostOpRequest{Op: types.HostOpBrowser, Input: json.RawMessage(`{"action":"navigate","url":"https://example.com"}`)},
			want: "browse.navigate https://example.com",
		},
		{
			name: "email",
			req:  types.HostOpRequest{Op: types.HostOpEmail, Input: emailInput},
			want: "Email team@example.com: Daily report",
		},
		{
			name: "task_create",
			req:  types.HostOpRequest{Op: types.HostOpToolResult, Tag: "task_create", Input: taskCreateInput},
			want: "Create task",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := exec.Exec(context.Background(), tc.req)
			if !resp.Ok {
				t.Fatalf("expected ok response, got %+v", resp)
			}
			if got := strings.TrimSpace(gotReq.Data["requestText"]); got != tc.want {
				t.Fatalf("requestText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEventMiddleware_EmitsResponseTextForRepresentativeOps(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		switch req.Op {
		case types.HostOpFSRead:
			return types.HostOpResponse{Op: req.Op, Ok: true, Truncated: true}
		case types.HostOpShellExec:
			return types.HostOpResponse{Op: req.Op, Ok: true, ExitCode: 0}
		case types.HostOpHTTPFetch:
			return types.HostOpResponse{Op: req.Op, Ok: true, Status: 200}
		case types.HostOpBrowser:
			return types.HostOpResponse{Op: "browser.navigate", Ok: true, Text: `{"title":"Example Domain"}`}
		case types.HostOpEmail:
			return types.HostOpResponse{Op: req.Op, Ok: true}
		default:
			return types.HostOpResponse{Op: req.Op, Ok: true}
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

	tests := []struct {
		name string
		req  types.HostOpRequest
		want string
	}{
		{name: "fs_read", req: types.HostOpRequest{Op: types.HostOpFSRead, Path: "/workspace/a.txt"}, want: "✓ truncated"},
		{name: "shell_exec", req: types.HostOpRequest{Op: types.HostOpShellExec, Argv: []string{"echo", "ok"}}, want: "✓ exit 0"},
		{name: "http_fetch", req: types.HostOpRequest{Op: types.HostOpHTTPFetch, URL: "https://example.com"}, want: "✓ 200"},
		{name: "browser.navigate", req: types.HostOpRequest{Op: types.HostOpBrowser, Input: json.RawMessage(`{"action":"navigate","url":"https://example.com"}`)}, want: `✓ navigated "Example Domain"`},
		{name: "email", req: types.HostOpRequest{Op: types.HostOpEmail, Input: json.RawMessage(`{"to":"team@example.com","subject":"Daily report","body":"done"}`)}, want: "✓ sent"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := exec.Exec(context.Background(), tc.req)
			if !resp.Ok {
				t.Fatalf("expected ok response, got %+v", resp)
			}
			if got := strings.TrimSpace(gotResp.Data["responseText"]); got != tc.want {
				t.Fatalf("responseText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEventMiddleware_RequestEnrichmentMovedToOperations(t *testing.T) {
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
		Op:    types.HostOpFSSearch,
		Path:  "/workspace",
		Query: "needle",
		Limit: 25,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["query"] != "needle" || gotReq.Data["limit"] != "25" {
		t.Fatalf("expected fs_search query/limit enrichment, got data=%v", gotReq.Data)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:     types.HostOpTrace,
		Action: "events.latest",
		Input:  json.RawMessage(`{"limit":5}`),
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["traceAction"] != "events.latest" || gotReq.Data["traceInput"] != `{"limit":5}` {
		t.Fatalf("expected trace request enrichment, got data=%v", gotReq.Data)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: "/workspace/data.json",
		Text: `{"x":1}`,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["textPreview"] == "" || gotReq.Data["textIsJSON"] != "true" {
		t.Fatalf("expected fs_write preview enrichment, got data=%v", gotReq.Data)
	}

	resp = exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpFSPatch,
		Path: "/workspace/a.txt",
		Text: "@@ -1 +1 @@\n-old\n+new\n",
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["patchPreview"] == "" {
		t.Fatalf("expected fs_patch preview enrichment, got data=%v", gotReq.Data)
	}
}

func TestEventMiddleware_BrowserRequestAndResponseEnrichment(t *testing.T) {
	base := types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		if req.Op == types.HostOpBrowser {
			return types.HostOpResponse{
				Op:   "browser.navigate",
				Ok:   true,
				Text: `{"sessionId":"sess-1","pageId":"p-1","title":"Example Domain","url":"https://example.com","count":2}`,
			}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}
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

	reqInput := json.RawMessage(`{"action":"navigate","sessionId":"sess-1","url":"https://example.com","selector":"#main","values":["a","b"],"filename":"shot.png"}`)
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:    types.HostOpBrowser,
		Input: reqInput,
	})
	if !resp.Ok {
		t.Fatalf("expected ok response, got %+v", resp)
	}
	if gotReq.Data["action"] != "navigate" || gotReq.Data["url"] != "https://example.com" || gotReq.Data["valuesCount"] != "2" {
		t.Fatalf("expected browser request enrichment, got data=%v", gotReq.Data)
	}
	if gotResp.Data["browserOp"] != "browser.navigate" || gotResp.Data["title"] == "" || gotResp.Data["count"] != "2" {
		t.Fatalf("expected browser response enrichment, got data=%v", gotResp.Data)
	}
}

func TestResolveOperationForResponse_Aliases(t *testing.T) {
	reg := newHostOperationRegistry(nil)

	if op := resolveOperationForResponse(reg, types.HostOpRequest{Op: types.HostOpBrowser}, types.HostOpResponse{Op: "browser.navigate"}); op == nil || op.Op() != types.HostOpBrowser {
		t.Fatalf("expected browser alias to resolve to browser operation, got %+v", op)
	}

	if op := resolveOperationForResponse(reg, types.HostOpRequest{Op: types.HostOpToolResult, Tag: "task_create"}, types.HostOpResponse{Op: "task_create"}); op == nil || op.Op() != types.HostOpToolResult {
		t.Fatalf("expected task_create alias to resolve to tool_result operation, got %+v", op)
	}

	if op := resolveOperationForResponse(reg, types.HostOpRequest{Op: types.HostOpShellExec}, types.HostOpResponse{Op: "unknown.op"}); op == nil || op.Op() != types.HostOpShellExec {
		t.Fatalf("expected fallback to request op, got %+v", op)
	}
}
