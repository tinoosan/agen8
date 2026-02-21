package vfs_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tinoosan/agen8/pkg/vfs"
)

type fakeResource struct {
	listFn   func(string) ([]vfs.Entry, error)
	readFn   func(string) ([]byte, error)
	writeFn  func(string, []byte) error
	appendFn func(string, []byte) error
}

func (r fakeResource) SupportsNestedList() bool {
	return true
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

func mustMount(t *testing.T, fs *vfs.FS, name string, r vfs.Resource) {
	t.Helper()
	if err := fs.Mount(name, r); err != nil {
		t.Fatalf("mount %s: %v", name, err)
	}
}

func TestResolve(t *testing.T) {
	fs := vfs.NewFS()
	mustMount(t, fs, vfs.MountWorkspace, fakeResource{})

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
		if mn != vfs.MountWorkspace || subpath != "" {
			t.Fatalf("got mn=%q subpath=%q", mn, subpath)
		}
	})

	t.Run("ValidSubpath", func(t *testing.T) {
		mn, _, subpath, err := fs.Resolve("/workspace/a/b")
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if mn != vfs.MountWorkspace || subpath != "a/b" {
			t.Fatalf("got mn=%q subpath=%q", mn, subpath)
		}
	})
}

func TestListRoot_IsStableAndPrefixed(t *testing.T) {
	fs := vfs.NewFS()
	mustMount(t, fs, "b", fakeResource{})
	mustMount(t, fs, "a", fakeResource{})

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
	mustMount(t, fs, vfs.MountWorkspace, fakeResource{})

	if _, err := fs.List("/nope"); err == nil {
		t.Fatalf("expected error for unknown mount")
	}
	if _, err := fs.Read("/nope/x"); err == nil {
		t.Fatalf("expected error for unknown mount")
	}
}

func TestResolve_LongestPrefixWins(t *testing.T) {
	fs := vfs.NewFS()
	mustMount(t, fs, "a", fakeResource{})
	mustMount(t, fs, "a/b", fakeResource{})

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
	mustMount(t, fs, "m", fakeResource{
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
	mustMount(t, fs, "m", fakeResource{
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
