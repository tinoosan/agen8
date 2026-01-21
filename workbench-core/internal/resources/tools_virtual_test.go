package resources_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestVirtualToolsResource_ListIncludesBuiltin(t *testing.T) {
	builtin, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := tools.NewCompositeToolManifestRegistry(builtin)
	res, err := resources.NewToolsResource(reg)
	if err != nil {
		t.Fatalf("NewToolsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTools, res)

	entries, err := fs.List("/tools")
	if err != nil {
		t.Fatalf("List /tools: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected builtin tools to be listed")
	}
	found := false
	for _, e := range entries {
		if e.Path == "/tools/builtin.shell" && e.IsDir {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected /tools/builtin.shell to be listed, got %+v", entries)
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

	disk := tools.NewDiskManifestProvider(dir)
	builtin, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := tools.NewCompositeToolManifestRegistry(builtin, disk)
	res, err := resources.NewToolsResource(reg)
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

func TestVirtualToolsResource_BuiltinOverridesDiskOnCollision(t *testing.T) {
	dir := t.TempDir()

	// Collision tool ID that is already registered as a builtin in this repo.
	toolID := "github.com.dupe.tool"
	if err := os.MkdirAll(filepath.Join(dir, toolID), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, toolID, "manifest.json"), []byte(`{"id":"github.com.dupe.tool","version":"0.1.0","kind":"custom","displayName":"Dupe Tool (disk)","description":"Should be hidden","actions":[{"id":"dupe.noop","displayName":"No-op","description":"noop","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`), 0644); err != nil {
		t.Fatal(err)
	}

	disk := tools.NewDiskManifestProvider(dir)
	builtin, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := tools.NewCompositeToolManifestRegistry(builtin, disk)
	res, err := resources.NewToolsResource(reg)
	if err != nil {
		t.Fatalf("NewToolsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountTools, res)

	b, err := fs.Read("/tools/" + toolID)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	// Ensure we got the builtin manifest, not the disk one.
	m, err := types.ParseBuiltinToolManifest(b)
	if err != nil {
		t.Fatalf("ParseBuiltinToolManifest: %v", err)
	}
	if m.ID.String() != toolID {
		t.Fatalf("unexpected tool id %q", m.ID.String())
	}
}

func TestVirtualToolsResource_ReadOnlyEnforced(t *testing.T) {
	builtin, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := tools.NewCompositeToolManifestRegistry(builtin)
	res, err := resources.NewToolsResource(reg)
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
	builtin, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		t.Fatalf("NewBuiltinManifestProvider: %v", err)
	}
	reg := tools.NewCompositeToolManifestRegistry(builtin)
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
