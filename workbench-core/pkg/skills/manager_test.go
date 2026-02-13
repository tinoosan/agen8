package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestManager_AllowedSkills(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSkill(t, tmp, "A")
	mustWriteSkill(t, tmp, "B")
	mustWriteSkill(t, tmp, "C")

	mgr := NewManager([]string{tmp})
	mgr.AllowedSkills = []string{"A", "C"}
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	entries := mgr.Entries()
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Dir)
	}
	want := []string{"A", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("entries mismatch: got %v want %v", got, want)
	}

	if _, ok := mgr.Get("B"); ok {
		t.Fatalf("expected B to be hidden")
	}
	if _, ok := mgr.Get("A"); !ok {
		t.Fatalf("expected A to be visible")
	}
}

func TestManager_ScriptsManifest(t *testing.T) {
	tmp := t.TempDir()
	mustWriteSkill(t, tmp, "alpha")
	mustWriteSkill(t, tmp, "beta")
	mustWriteSkill(t, tmp, "gamma")

	// alpha has scripts
	mustWriteFile(t, filepath.Join(tmp, "alpha", "scripts", "b.sh"), []byte("#!/usr/bin/env bash\n"))
	mustWriteFile(t, filepath.Join(tmp, "alpha", "scripts", "a.py"), []byte("print('ok')\n"))
	mustWriteFile(t, filepath.Join(tmp, "alpha", "scripts", ".hidden"), []byte("ignore\n"))
	if err := os.MkdirAll(filepath.Join(tmp, "alpha", "scripts", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}

	// gamma has scripts
	mustWriteFile(t, filepath.Join(tmp, "gamma", "scripts", "only.py"), []byte("print('x')\n"))

	mgr := NewManager([]string{tmp})
	if err := mgr.Scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	manifest := mgr.ScriptsManifest()
	if len(manifest) != 2 {
		t.Fatalf("expected 2 skills in manifest, got %d", len(manifest))
	}

	if manifest[0].Skill != "alpha" || manifest[1].Skill != "gamma" {
		t.Fatalf("unexpected skill order: %+v", manifest)
	}

	alphaNames := []string{manifest[0].Scripts[0].Name, manifest[0].Scripts[1].Name}
	if !reflect.DeepEqual(alphaNames, []string{"a.py", "b.sh"}) {
		t.Fatalf("unexpected alpha scripts: %v", alphaNames)
	}
	alphaRels := []string{manifest[0].Scripts[0].Rel, manifest[0].Scripts[1].Rel}
	if !reflect.DeepEqual(alphaRels, []string{"scripts/a.py", "scripts/b.sh"}) {
		t.Fatalf("unexpected alpha rels: %v", alphaRels)
	}

	if len(manifest[1].Scripts) != 1 || manifest[1].Scripts[0].Name != "only.py" || manifest[1].Scripts[0].Rel != "scripts/only.py" {
		t.Fatalf("unexpected gamma scripts: %+v", manifest[1].Scripts)
	}
}

func mustWriteSkill(t *testing.T, root, name string) {
	t.Helper()
	content := []byte("---\nname: " + name + "\ndescription: test\n---\n# Instructions\n")
	mustWriteFile(t, filepath.Join(root, name, "SKILL.md"), content)
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
