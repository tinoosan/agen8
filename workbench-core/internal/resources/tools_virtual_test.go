package resources_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	internaltools "github.com/tinoosan/workbench-core/internal/tools"
	pkgtools "github.com/tinoosan/workbench-core/pkg/tools"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

func TestVirtualToolsResource_ListDoesNotIncludeCoreTools(t *testing.T) {
	builtin, err := internaltools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := pkgtools.NewCompositeToolManifestRegistry(builtin)
	res, err := internaltools.NewToolsResource(reg)
	if err != nil {
		t.Fatalf("NewToolsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTools, res)

	entries, err := fs.List("/tools")
	if err != nil {
		t.Fatalf("List /tools: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no tools listed, got %+v", entries)
	}
	for _, e := range entries {
		if strings.Contains(e.Path, "builtin.shell") || strings.Contains(e.Path, "builtin.http") || strings.Contains(e.Path, "builtin.trace") {
			t.Fatalf("core tools should not be listed, got %q", e.Path)
		}
	}
}

func TestVirtualToolsResource_DiskToolsAppearWhenPresent(t *testing.T) {
	dir := t.TempDir()

	// Valid custom tool manifest.
	toolID := "github.com.other.stock"
	if err := os.MkdirAll(filepath.Join(dir, toolID), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, toolID, "manifest.json"), []byte(`{"id":"github.com.other.stock","version":"0.1.0","kind":"custom","displayName":"Other Stock","description":"Example disk tool","actions":[{"id":"quote.latest","displayName":"Latest Quote","description":"Fetch latest quote","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	disk := pkgtools.NewDiskManifestProvider(dir)
	builtin, err := internaltools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := pkgtools.NewCompositeToolManifestRegistry(builtin, disk)
	res, err := internaltools.NewToolsResource(reg)
	if err != nil {
		t.Fatalf("NewToolsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTools, res)

	entries, err := fs.List("/tools")
	if err != nil {
		t.Fatalf("List /tools: %v", err)
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		paths = append(paths, e.Path)
	}
	if !contains(paths, "/tools/"+toolID) {
		t.Fatalf("expected disk tool to be listed, got %v", paths)
	}

	b, err := fs.Read("/tools/" + toolID)
	if err != nil {
		t.Fatalf("Read manifest: %v", err)
	}
	if len(b) == 0 {
		t.Fatalf("expected manifest bytes")
	}
}

func TestVirtualToolsResource_DiskAppliesWhenNoBuiltin(t *testing.T) {
	dir := t.TempDir()

	// Collision tool ID that used to be registered as a builtin in this repo.
	toolID := "builtin.shell"
	if err := os.MkdirAll(filepath.Join(dir, toolID), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, toolID, "manifest.json"), []byte(`{"id":"builtin.shell","version":"0.1.0","kind":"custom","displayName":"Dupe Tool (disk)","description":"Should be hidden","actions":[{"id":"dupe.noop","displayName":"No-op","description":"noop","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	disk := pkgtools.NewDiskManifestProvider(dir)
	builtin, err := internaltools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := pkgtools.NewCompositeToolManifestRegistry(builtin, disk)
	res, err := internaltools.NewToolsResource(reg)
	if err != nil {
		t.Fatalf("NewToolsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTools, res)

	b, err := fs.Read("/tools/" + toolID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Ensure we got the disk manifest since builtin manifests are no longer exposed.
	m, err := pkgtools.ParseUserToolManifest(b)
	if err != nil {
		t.Fatalf("ParseUserToolManifest: %v", err)
	}
	if m.ID.String() != toolID {
		t.Fatalf("unexpected tool id %q", m.ID.String())
	}
}

func TestVirtualToolsResource_ReadOnlyEnforced(t *testing.T) {
	builtin, err := internaltools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := pkgtools.NewCompositeToolManifestRegistry(builtin)
	res, err := internaltools.NewToolsResource(reg)
	if err != nil {
		t.Fatalf("NewToolsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTools, res)

	if err := fs.Write("/tools/x", []byte("nope")); err == nil {
		t.Fatalf("expected tools write to fail")
	}
}

func contains(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

func TestCompositeToolManifestRegistry_ListDeterministic(t *testing.T) {
	builtin, err := internaltools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := pkgtools.NewCompositeToolManifestRegistry(builtin)
	ids1, err := reg.ListToolIDs(nil)
	if err != nil {
		t.Fatalf("ListToolIDs: %v", err)
	}
	ids2, err := reg.ListToolIDs(nil)
	if err != nil {
		t.Fatalf("ListToolIDs: %v", err)
	}
	if !reflect.DeepEqual(ids1, ids2) {
		t.Fatalf("expected deterministic ids")
	}
}
