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
