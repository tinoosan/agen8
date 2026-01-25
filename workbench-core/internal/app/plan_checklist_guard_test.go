package app

import (
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func newPlanFS(t *testing.T) *vfs.FS {
	t.Helper()
	base := t.TempDir()
	res, err := resources.NewDirResource(base, "plan")
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}
	fs := vfs.NewFS()
	fs.Mount("plan", res)
	return fs
}

func TestPlanChecklistHeadValidation(t *testing.T) {
	fs := newPlanFS(t)
	bad := types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: "/plan/HEAD.md",
		Text: "not a checklist",
	}
	resp := enforcePlanChecklist(fs, bad)
	if resp == nil || resp.ErrorCode != "plan_checklist_invalid" {
		t.Fatalf("expected plan_checklist_invalid, got %#v", resp)
	}

	good := types.HostOpRequest{
		Op:   types.HostOpFSWrite,
		Path: "/plan/HEAD.md",
		Text: "- [ ] Step 1",
	}
	resp = enforcePlanChecklist(fs, good)
	if resp != nil {
		t.Fatalf("expected nil for valid checklist, got %#v", resp)
	}
}

func TestPlanChecklistWarningWhenStale(t *testing.T) {
	fs := newPlanFS(t)
	if err := fs.Write("/plan/HEAD.md", []byte("- [x] Done")); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
	req := types.HostOpRequest{
		Op: types.HostOpShellExec,
	}
	resp := enforcePlanChecklist(fs, req)
	if resp == nil || resp.ErrorCode != "plan_checklist_required" {
		t.Fatalf("expected plan_checklist_required, got %#v", resp)
	}
	b, err := fs.Read("/plan/HEAD.md")
	if err != nil {
		t.Fatalf("read checklist: %v", err)
	}
	if !strings.Contains(string(b), "Add new checklist items") {
		t.Fatalf("expected warning appended, got %q", string(b))
	}
}
