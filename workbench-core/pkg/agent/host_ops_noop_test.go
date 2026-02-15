package agent

import (
	"context"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func TestHostOpExecutor_NoopEchoesText(t *testing.T) {
	exec := &HostOpExecutor{FS: vfs.NewFS()}
	resp := exec.Exec(context.Background(), types.HostOpRequest{
		Op:   types.HostOpNoop,
		Text: "hello",
	})
	if !resp.Ok {
		t.Fatalf("expected noop success, got error: %s", resp.Error)
	}
	if resp.Text != "hello" {
		t.Fatalf("resp.Text=%q, want %q", resp.Text, "hello")
	}
}
