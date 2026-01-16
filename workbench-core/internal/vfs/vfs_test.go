package vfs_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type fakeResource struct {
	listFn   func(string) ([]vfs.Entry, error)
	readFn   func(string) ([]byte, error)
	writeFn  func(string, []byte) error
	appendFn func(string, []byte) error
}

func (r fakeResource) List(path string) ([]vfs.Entry, error) {
	if r.listFn == nil {
		return nil, errors.New("not implemented")
	}
	return r.listFn(path)
}

func (r fakeResource) Read(path string) ([]byte, error) {
	if r.readFn == nil {
		return nil, errors.New("not implemented")
	}
	return r.readFn(path)
}

func (r fakeResource) Write(path string, data []byte) error {
	if r.writeFn == nil {
		return errors.New("not implemented")
	}
	return r.writeFn(path, data)
}

func (r fakeResource) Append(path string, data []byte) error {
	if r.appendFn == nil {
		return errors.New("not implemented")
	}
	return r.appendFn(path, data)
}

func TestResolve(t *testing.T) {
	fs := vfs.NewFS()
	fs.Mount("workspace", fakeResource{})

	t.Run("Empty", func(t *testing.T) {
		if _, _, _, err := fs.Resolve(""); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("NoLeadingSlash", func(t *testing.T) {
		if _, _, _, err := fs.Resolve("workspace/a"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("RootMissingMount", func(t *testing.T) {
		if _, _, _, err := fs.Resolve("/"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("UnknownMount", func(t *testing.T) {
		if _, _, _, err := fs.Resolve("/nope/a"); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("ValidMountRoot", func(t *testing.T) {
		mn, _, subpath, err := fs.Resolve("/workspace")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if mn != "workspace" || subpath != "" {
			t.Fatalf("got mn=%q subpath=%q", mn, subpath)
		}
	})

	t.Run("ValidSubpath", func(t *testing.T) {
		mn, _, subpath, err := fs.Resolve("/workspace/a/b")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if mn != "workspace" || subpath != "a/b" {
			t.Fatalf("got mn=%q subpath=%q", mn, subpath)
		}
	})
}

func TestListRoot_IsStableAndPrefixed(t *testing.T) {
	fs := vfs.NewFS()
	fs.Mount("b", fakeResource{})
	fs.Mount("a", fakeResource{})

	entries, err := fs.List("/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Path)
		if !e.IsDir {
			t.Fatalf("expected IsDir=true for %q", e.Path)
		}
	}
	want := []string{"/a", "/b"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestNotFoundPathFails(t *testing.T) {
	fs := vfs.NewFS()
	fs.Mount("workspace", fakeResource{})

	if _, err := fs.List("/nope"); err == nil {
		t.Fatalf("expected error for unknown mount")
	}
	if _, err := fs.Read("/nope/x"); err == nil {
		t.Fatalf("expected error for unknown mount")
	}
}

func TestResolve_LongestPrefixWins(t *testing.T) {
	fs := vfs.NewFS()
	fs.Mount("a", fakeResource{})
	fs.Mount("a/b", fakeResource{})

	mn, _, subpath, err := fs.Resolve("/a/b/c")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if mn != "a/b" {
		t.Fatalf("expected mount %q got %q", "a/b", mn)
	}
	if subpath != "c" {
		t.Fatalf("expected subpath %q got %q", "c", subpath)
	}
}

func TestList_RewritesPathsWithMountPrefix(t *testing.T) {
	fs := vfs.NewFS()
	fs.Mount("m", fakeResource{
		listFn: func(subpath string) ([]vfs.Entry, error) {
			if subpath != "" {
				t.Fatalf("expected subpath '', got %q", subpath)
			}
			return []vfs.Entry{
				{Path: "", IsDir: true},
				{Path: "notes.md", IsDir: false},
				{Path: "dir/file.txt", IsDir: false},
			}, nil
		},
	})

	entries, err := fs.List("/m")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Path)
	}
	want := []string{"/m", "/m/notes.md", "/m/dir/file.txt"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestReadWriteAppend_WrapErrors(t *testing.T) {
	fs := vfs.NewFS()
	fs.Mount("m", fakeResource{
		readFn: func(subpath string) ([]byte, error) {
			return nil, errors.New("boom")
		},
		writeFn: func(subpath string, data []byte) error {
			return errors.New("boom")
		},
		appendFn: func(subpath string, data []byte) error {
			return errors.New("boom")
		},
	})

	if _, err := fs.Read("/m/a.txt"); err == nil || !strings.Contains(err.Error(), "read m:a.txt") {
		t.Fatalf("expected wrapped read error, got %v", err)
	}
	if err := fs.Write("/m/a.txt", []byte("x")); err == nil || !strings.Contains(err.Error(), "write m:a.txt") {
		t.Fatalf("expected wrapped write error, got %v", err)
	}
	if err := fs.Append("/m/a.txt", []byte("x")); err == nil || !strings.Contains(err.Error(), "append m:a.txt") {
		t.Fatalf("expected wrapped append error, got %v", err)
	}
}

func TestVFS_WithDirResourceRoundtrip(t *testing.T) {
	tmp := t.TempDir()
	dr, err := resources.NewDirResource(tmp, "workspace")
	if err != nil {
		t.Fatalf("NewDirResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount("workspace", dr)

	if err := fs.Write("/workspace/notes.md", []byte("hi")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	b, err := fs.Read("/workspace/notes.md")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(b) != "hi" {
		t.Fatalf("got %q", string(b))
	}

	entries, err := fs.List("/workspace")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 1 || entries[0].Path != "/workspace/notes.md" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestVFS_WithToolsResource_ReadOnlyManifest(t *testing.T) {
	tmp := t.TempDir()
	builtin := tools.StaticManifestProvider{
		Manifests: map[types.ToolID][]byte{
			types.ToolID("github.com.acme.stock"): []byte(`{"id":"github.com.acme.stock"}`),
		},
	}

	// Disk tool (valid custom manifest; DiskManifestProvider validates it).
	if err := os.MkdirAll(filepath.Join(tmp, "github.com.other.stock"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(tmp, "github.com.other.stock", "manifest.json"),
		[]byte(`{"id":"github.com.other.stock","version":"0.1.0","kind":"custom","displayName":"Other Stock","description":"Example disk tool","actions":[{"id":"quote.latest","displayName":"Latest Quote","description":"Fetch latest quote","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	disk := tools.NewDiskManifestProvider(tmp)
	reg := tools.NewCompositeToolManifestRegistry(builtin, disk)
	toolsRes, err := resources.NewVirtualToolsResource(reg)
	if err != nil {
		t.Fatalf("NewVirtualToolsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount("tools", toolsRes)

	entries, err := fs.List("/tools")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := make([]string, 0, len(entries))
	for _, e := range entries {
		got = append(got, e.Path)
	}
	want := []string{"/tools/github.com.acme.stock", "/tools/github.com.other.stock"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}

	b, err := fs.Read("/tools/github.com.acme.stock")
	if err != nil {
		t.Fatalf("Read builtin: %v", err)
	}
	if string(b) != `{"id":"github.com.acme.stock"}` {
		t.Fatalf("unexpected bytes: %q", string(b))
	}

	b, err = fs.Read("/tools/github.com.other.stock/manifest.json")
	if err != nil {
		t.Fatalf("Read disk: %v", err)
	}
	if !strings.Contains(string(b), `"id":"github.com.other.stock"`) {
		t.Fatalf("unexpected bytes: %q", string(b))
	}

	if _, err := fs.Read("/tools/github.com.other.stock/bin/x"); err == nil {
		t.Fatalf("expected nested path error")
	}

	if err := fs.Write("/tools/github.com.other.stock/manifest.json", []byte("x")); err == nil {
		t.Fatalf("expected write not supported")
	}
}
