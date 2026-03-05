package skills

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tinoosan/agen8/pkg/types"
)

func TestSkillsResource_WriteAddsSkill(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewManager([]string{tmp})
	mgr.WritableRoot = tmp
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	res := NewResource(mgr)

	if _, ok := mgr.Get("new-skill"); ok {
		t.Fatalf("expected no initial skill")
	}

	content := []byte("---\nname: new-skill\n---\n# Instructions\n")
	if err := res.Write("new-skill/SKILL.md", content); err != nil {
		t.Fatalf("write: %v", err)
	}

	skill, ok := mgr.Get("new-skill")
	if !ok {
		t.Fatalf("expected new skill to be discovered")
	}
	if skill.Path != filepath.Join(tmp, "new-skill", "SKILL.md") {
		t.Fatalf("unexpected skill path %q", skill.Path)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "new-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("unexpected file contents: %q", data)
	}
}

func TestSkillsResource_WritePreventsTraversal(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewManager([]string{tmp})
	mgr.WritableRoot = tmp
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	res := NewResource(mgr)

	if err := res.Write("../evil/SKILL.md", []byte("bad")); err == nil {
		t.Fatalf("expected traversal to fail")
	}
}

func TestSkillsResource_Append(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewManager([]string{tmp})
	mgr.WritableRoot = tmp
	res := NewResource(mgr)

	if err := res.Write("append-skill/SKILL.md", []byte("first\n")); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	if err := res.Append("append-skill/SKILL.md", []byte("second\n")); err != nil {
		t.Fatalf("append: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "append-skill", "SKILL.md"))
	if err != nil {
		t.Fatalf("read appended file: %v", err)
	}
	got := string(data)
	if got != "first\nsecond\n" {
		t.Fatalf("expected concatenated contents, got %q", got)
	}

	if _, ok := mgr.Get("append-skill"); !ok {
		t.Fatalf("expected manager to discover appended skill")
	}
}

func TestSkillsResource_ListFilteredByProfile(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSkill(t, tmp, "allowed")
	mustWriteSkill(t, tmp, "blocked")

	mgr := NewManager([]string{tmp})
	mgr.WritableRoot = tmp
	mgr.AllowedSkills = []string{"allowed"}
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	res := NewResource(mgr)

	entries, err := res.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Path)
	}
	if !reflect.DeepEqual(got, []string{"allowed"}) {
		t.Fatalf("unexpected list paths: %v", got)
	}

	if _, err := res.Read("blocked/SKILL.md"); err == nil {
		t.Fatalf("expected blocked skill read to fail")
	}
}

func TestSkillsResource_WriteBlockedWhenDisallowed(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewManager([]string{tmp})
	mgr.WritableRoot = tmp
	mgr.AllowedSkills = []string{"allowed"}
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	res := NewResource(mgr)

	content := []byte("---\nname: blocked\n---\n# Instructions\n")
	if err := res.Write("blocked/SKILL.md", content); err == nil {
		t.Fatalf("expected write to blocked skill to fail")
	}
	if err := res.Append("blocked/SKILL.md", []byte("x")); err == nil {
		t.Fatalf("expected append to blocked skill to fail")
	}
}

func TestSkillsResource_EnforceAllowlistWithEmptyList_HidesAllSkills(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSkill(t, tmp, "allowed")

	mgr := NewManager([]string{tmp})
	mgr.WritableRoot = tmp
	mgr.EnforceAllowlist = true
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	res := NewResource(mgr)

	entries, err := res.List("")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no visible skills, got %d", len(entries))
	}

	if _, err := res.Read("allowed/SKILL.md"); err == nil {
		t.Fatalf("expected read to fail when strict allowlist is empty")
	}
}

func TestSkillsResource_SearchRootAndSkillSubpath(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSkill(t, tmp, "coding")
	if err := os.MkdirAll(filepath.Join(tmp, "coding", "scripts"), 0o755); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "coding", "scripts", "helper.py"), []byte("print('needle')\n"), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	mgr := NewManager([]string{tmp})
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	res := NewResource(mgr)

	rootResp, err := res.Search(context.Background(), "", types.SearchRequest{
		Query:           "needle",
		Limit:           10,
		IncludeMetadata: true,
		PreviewLines:    1,
	})
	if err != nil {
		t.Fatalf("root search: %v", err)
	}
	if rootResp.Total == 0 || len(rootResp.Results) == 0 {
		t.Fatalf("expected root search results, got %+v", rootResp)
	}
	if got := rootResp.Results[0].Path; got != "/skills/coding/scripts/helper.py" {
		t.Fatalf("unexpected root result path %q", got)
	}
	if rootResp.Results[0].SizeBytes == nil || rootResp.Results[0].PreviewMatch == "" {
		t.Fatalf("expected metadata and preview in root result, got %+v", rootResp.Results[0])
	}

	subResp, err := res.Search(context.Background(), "coding/scripts", types.SearchRequest{
		Query: "needle",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("subpath search: %v", err)
	}
	if subResp.Total != 1 || len(subResp.Results) != 1 {
		t.Fatalf("expected one subpath result, got %+v", subResp)
	}
	if got := subResp.Results[0].Path; got != "/skills/coding/scripts/helper.py" {
		t.Fatalf("unexpected subpath result path %q", got)
	}
}
