package tools_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
)

func TestBuiltinOpen_Open_OK(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hello"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var gotAbs string
	inv := tools.NewBuiltinOpenInvokerWithOpener(root, func(ctx context.Context, absPath string) error {
		_ = ctx
		gotAbs = absPath
		return nil
	})

	runner := tools.Runner{
		Results: store.NewInMemoryResultsStore(),
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.open"): inv,
		},
	}

	resp, err := runner.Run(context.Background(), types.ToolID("builtin.open"), "open", json.RawMessage(`{"path":"README.md"}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp)
	}
	if gotAbs != filepath.Join(root, "README.md") {
		t.Fatalf("opener got %q, want %q", gotAbs, filepath.Join(root, "README.md"))
	}
}

func TestBuiltinOpen_Open_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	inv := tools.NewBuiltinOpenInvokerWithOpener(root, func(ctx context.Context, absPath string) error {
		t.Fatalf("unexpected opener call: %q", absPath)
		return nil
	})
	runner := tools.Runner{
		Results: store.NewInMemoryResultsStore(),
		ToolRegistry: tools.MapRegistry{
			types.ToolID("builtin.open"): inv,
		},
	}
	resp, err := runner.Run(context.Background(), types.ToolID("builtin.open"), "open", json.RawMessage(`{"path":"../secrets.txt"}`), 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected ok=false, got %+v", resp)
	}
	if resp.Error == nil || resp.Error.Code != "invalid_input" {
		t.Fatalf("expected invalid_input, got %+v", resp.Error)
	}
}
