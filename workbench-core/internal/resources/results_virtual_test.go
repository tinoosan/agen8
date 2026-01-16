package resources_test

import (
	"bytes"
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestVirtualResultsResource_ListAndRead(t *testing.T) {
	resultsStore := store.NewInMemoryResultsStore()
	if err := resultsStore.PutCall("abc", []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("PutCall: %v", err)
	}
	if err := resultsStore.PutArtifact("abc", "stdout.txt", "text/plain", []byte("hello\n")); err != nil {
		t.Fatalf("PutArtifact: %v", err)
	}

	res, err := resources.NewVirtualResultsResource(resultsStore)
	if err != nil {
		t.Fatalf("NewVirtualResultsResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountResults, res)

	entries, err := fs.List("/results")
	if err != nil {
		t.Fatalf("List /results: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.Path == "/results/abc" && e.IsDir {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected /results/abc in list, got %+v", entries)
	}

	callEntries, err := fs.List("/results/abc")
	if err != nil {
		t.Fatalf("List /results/abc: %v", err)
	}
	wantPaths := map[string]bool{
		"/results/abc/response.json": true,
		"/results/abc/stdout.txt":    true,
	}
	for _, e := range callEntries {
		delete(wantPaths, e.Path)
	}
	if len(wantPaths) != 0 {
		t.Fatalf("missing entries: %+v", wantPaths)
	}

	b, err := fs.Read("/results/abc/response.json")
	if err != nil {
		t.Fatalf("Read response.json: %v", err)
	}
	if !bytes.Equal(b, []byte(`{"ok":true}`)) {
		t.Fatalf("unexpected response bytes: %q", string(b))
	}

	ab, err := fs.Read("/results/abc/stdout.txt")
	if err != nil {
		t.Fatalf("Read artifact: %v", err)
	}
	if string(ab) != "hello\n" {
		t.Fatalf("unexpected artifact bytes: %q", string(ab))
	}

	if err := fs.Write("/results/abc/response.json", []byte("nope")); err == nil {
		t.Fatalf("expected results write to be rejected")
	}
}
