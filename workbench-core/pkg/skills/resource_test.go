package skills

import (
	"os"
	"path/filepath"
	"testing"
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
	if err := res.Write("new-skill.md", content); err != nil {
		t.Fatalf("write: %v", err)
	}

	skill, ok := mgr.Get("new-skill")
	if !ok {
		t.Fatalf("expected new skill to be discovered")
	}
	if skill.Path != filepath.Join(tmp, "new-skill.md") {
		t.Fatalf("unexpected skill path %q", skill.Path)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "new-skill.md"))
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

	if err := res.Write("../evil.md", []byte("bad")); err == nil {
		t.Fatalf("expected traversal to fail")
	}
}

func TestSkillsResource_Append(t *testing.T) {
	tmp := t.TempDir()
	mgr := NewManager([]string{tmp})
	mgr.WritableRoot = tmp
	res := NewResource(mgr)

	if err := res.Write("append-skill.md", []byte("first\n")); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	if err := res.Append("append-skill.md", []byte("second\n")); err != nil {
		t.Fatalf("append: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "append-skill.md"))
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
