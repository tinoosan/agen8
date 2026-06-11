package skillinstaller

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// pluginSkillsRelDir is the plugin-marketplace copy of the skill tree. It is kept
// byte-identical to the //go:embed embedded tree by hand — there is no generator.
// TestEmbeddedSkillsMatchPluginCopies is the only thing preventing a one-sided edit
// from silently shipping a stale skill.
const pluginSkillsRelDir = "plugins/agen8/skills"

// repoRoot resolves the module root from this test file's own location, so the
// test works regardless of the process working directory (go test ./..., -C, IDE).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test file path via runtime.Caller")
	}
	// thisFile = <root>/internal/skillinstaller/embedded_sync_test.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

// TestEmbeddedSkillsMatchPluginCopies asserts the embedded skill tree (compiled
// into the installer binary) and the plugin marketplace copy are byte-identical
// across every file, in both directions. The trees are maintained by hand, so this
// guard turns a silent drift into a failed build.
func TestEmbeddedSkillsMatchPluginCopies(t *testing.T) {
	pluginRoot := filepath.Join(repoRoot(t), filepath.FromSlash(pluginSkillsRelDir))

	// Forward: every embedded file has a byte-identical plugin counterpart.
	embeddedFiles := map[string]struct{}{}
	walkErr := fs.WalkDir(embedded, embeddedRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(embeddedRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		embeddedFiles[rel] = struct{}{}

		want, err := embedded.ReadFile(path)
		if err != nil {
			t.Fatalf("read embedded %s: %v", rel, err)
		}
		got, err := os.ReadFile(filepath.Join(pluginRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("plugin copy missing for embedded skill file %q: %v\n  -> copy %s/%s to %s/%s",
				rel, err, embeddedRoot, rel, pluginSkillsRelDir, rel)
			return nil
		}
		if string(got) != string(want) {
			t.Errorf("skill file %q differs between embedded source and plugin copy; the two must stay byte-identical\n  -> reconcile %s/%s and %s/%s",
				rel, embeddedRoot, rel, pluginSkillsRelDir, rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk embedded tree: %v", walkErr)
	}

	// Reverse: every plugin file has an embedded counterpart. Catches a file added
	// to the plugin copy but not the embedded source — which would never ship in
	// the installer binary.
	walkErr = filepath.WalkDir(pluginRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(pluginRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := embeddedFiles[rel]; !ok {
			t.Errorf("plugin skill file %q has no embedded counterpart; it will not ship in the installer\n  -> add %s/%s or remove %s/%s",
				rel, embeddedRoot, rel, pluginSkillsRelDir, rel)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk plugin tree: %v", walkErr)
	}
}
