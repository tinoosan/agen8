package app

import (
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/pkg/resources"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
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
			Path: "/plan/CHECKLIST.md",
		Text: "not a checklist",
	}
	resp := enforcePlanChecklist(fs, bad)
	if resp == nil || resp.ErrorCode != "plan_checklist_invalid" {
		t.Fatalf("expected plan_checklist_invalid, got %#v", resp)
	}

	good := types.HostOpRequest{
		Op:   types.HostOpFSWrite,
			Path: "/plan/CHECKLIST.md",
		Text: "- [ ] Step 1",
	}
	resp = enforcePlanChecklist(fs, good)
	if resp != nil {
		t.Fatalf("expected nil for valid checklist, got %#v", resp)
	}
}

func TestPlanChecklistWarningWhenStale(t *testing.T) {
	fs := newPlanFS(t)
	if err := fs.Write("/plan/CHECKLIST.md", []byte("- [x] Done")); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
	req := types.HostOpRequest{
		Op: types.HostOpShellExec,
	}
	resp := enforcePlanChecklist(fs, req)
	if resp == nil || resp.ErrorCode != "plan_checklist_required" {
		t.Fatalf("expected plan_checklist_required, got %#v", resp)
	}
	b, err := fs.Read("/plan/CHECKLIST.md")
	if err != nil {
		t.Fatalf("read checklist: %v", err)
	}
	if !strings.Contains(string(b), "Add new checklist items") {
		t.Fatalf("expected warning appended, got %q", string(b))
	}
}

func TestEnsurePlanGateCreatesDefaults(t *testing.T) {
	fs := newPlanFS(t)
	if err := EnsurePlanGate(fs); err != nil {
		t.Fatalf("EnsurePlanGate: %v", err)
	}
	head, err := fs.Read(planHeadPath)
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	if strings.TrimSpace(string(head)) == "" {
		t.Fatalf("expected non-empty plan head")
	}
	checklist, err := fs.Read(planChecklistPath)
	if err != nil {
		t.Fatalf("read checklist: %v", err)
	}
	status := planChecklistStatusFromText(string(checklist))
	if !status.valid || !status.hasItems || !status.hasOpen {
		t.Fatalf("expected valid checklist with open items, got %#v", status)
	}
}

func TestEnsurePlanGateIsIdempotent(t *testing.T) {
	fs := newPlanFS(t)
	customHead := "# Custom Plan\n\nDetails"
	customChecklist := "- [ ] Custom step"
	if err := fs.Write(planHeadPath, []byte(customHead)); err != nil {
		t.Fatalf("seed head: %v", err)
	}
	if err := fs.Write(planChecklistPath, []byte(customChecklist)); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
	if err := EnsurePlanGate(fs); err != nil {
		t.Fatalf("EnsurePlanGate: %v", err)
	}
	head, err := fs.Read(planHeadPath)
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	if string(head) != customHead {
		t.Fatalf("expected head unchanged, got %q", string(head))
	}
	checklist, err := fs.Read(planChecklistPath)
	if err != nil {
		t.Fatalf("read checklist: %v", err)
	}
	if string(checklist) != customChecklist {
		t.Fatalf("expected checklist unchanged, got %q", string(checklist))
	}
}

func TestEnsurePlanGateReplacesInvalidChecklist(t *testing.T) {
	fs := newPlanFS(t)
	if err := fs.Write(planChecklistPath, []byte("not a checklist")); err != nil {
		t.Fatalf("seed checklist: %v", err)
	}
	if err := EnsurePlanGate(fs); err != nil {
		t.Fatalf("EnsurePlanGate: %v", err)
	}
	checklist, err := fs.Read(planChecklistPath)
	if err != nil {
		t.Fatalf("read checklist: %v", err)
	}
	status := planChecklistStatusFromText(string(checklist))
	if !status.valid || !status.hasItems {
		t.Fatalf("expected valid checklist, got %#v", status)
	}
}
