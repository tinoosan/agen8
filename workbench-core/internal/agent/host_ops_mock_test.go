package agent_test

import (
	"context"
	"testing"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestHostOpExecutor_FSRead_ToolsManifest_NotTruncatedByDefault(t *testing.T) {
	builtin, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := tools.NewCompositeToolManifestRegistry(builtin)
	toolsRes, err := resources.NewToolsResource(reg)
	if err != nil {
		t.Fatalf("NewToolsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTools, toolsRes)

	full, err := fs.Read("/tools/builtin.git")
	if err != nil {
		t.Fatalf("Read full manifest: %v", err)
	}
	if len(full) <= 4096 {
		t.Skipf("manifest is only %d bytes; truncation regression test not meaningful", len(full))
	}

	exec := &agent.HostOpExecutor{
		FS:              fs,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    256 * 1024,
	}
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSRead, Path: "/tools/builtin.git"})
	if !resp.Ok {
		t.Fatalf("expected ok=true, got error=%q", resp.Error)
	}
	if resp.Truncated {
		t.Fatalf("expected truncated=false for tool manifest read; bytesLen=%d textLen=%d", resp.BytesLen, len(resp.Text))
	}
	if resp.BytesLen != len(full) {
		t.Fatalf("expected bytesLen=%d, got %d", len(full), resp.BytesLen)
	}
	if len(resp.Text) != len(full) {
		t.Fatalf("expected text length %d, got %d (bytesB64 len=%d)", len(full), len(resp.Text), len(resp.BytesB64))
	}
}

