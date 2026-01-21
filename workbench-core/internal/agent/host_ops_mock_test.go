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

func TestHostOpExecutor_FSRead_CoreToolManifest_NotFound(t *testing.T) {
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

	if _, err := fs.Read("/tools/builtin.shell"); err == nil {
		t.Fatalf("expected core tool manifest to be absent, got %+v", err)
	}

	exec := &agent.HostOpExecutor{
		FS:              fs,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    256 * 1024,
	}
	resp := exec.Exec(context.Background(), types.HostOpRequest{Op: types.HostOpFSRead, Path: "/tools/builtin.shell"})
	if resp.Ok {
		t.Fatalf("expected core tool manifest read to fail, got ok response")
	}
	if resp.Error == "" {
		t.Fatalf("expected error when reading missing manifest")
	}
}
