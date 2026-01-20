package resources_test

import (
	"testing"

	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestVirtualStagingResource_ReadWriteAppendAndReadOnlyMain(t *testing.T) {
	cases := []struct {
		name     string
		mount    string
		mainFile string
		newRes   func(tmp string) (vfs.Resource, error)
	}{
		{
			name:     "memory",
			mount:    vfs.MountMemory,
			mainFile: "memory.md",
			newRes: func(tmp string) (vfs.Resource, error) {
				s, err := store.NewDiskMemoryStoreFromDir(tmp)
				if err != nil {
					return nil, err
				}
				return resources.NewMemoryResource(s)
			},
		},
		{
			name:     "profile",
			mount:    vfs.MountProfile,
			mainFile: "profile.md",
			newRes: func(tmp string) (vfs.Resource, error) {
				s, err := store.NewDiskProfileStoreFromDir(tmp)
				if err != nil {
					return nil, err
				}
				return resources.NewProfileResource(s)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := tc.newRes(t.TempDir())
			if err != nil {
				t.Fatalf("new resource: %v", err)
			}

			fs := vfs.NewFS()
			fs.Mount(tc.mount, r)

			entries, err := fs.List("/" + tc.mount)
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(entries) != 3 {
				t.Fatalf("expected 3 entries, got %d", len(entries))
			}
			wantEntries := []string{
				"/" + tc.mount + "/" + tc.mainFile,
				"/" + tc.mount + "/update.md",
				"/" + tc.mount + "/commits.jsonl",
			}
			for i := range wantEntries {
				if entries[i].Path != wantEntries[i] {
					t.Fatalf("entry[%d].Path: got %q, want %q", i, entries[i].Path, wantEntries[i])
				}
			}

			if err := fs.Write("/"+tc.mount+"/update.md", []byte("hello")); err != nil {
				t.Fatalf("Write update.md: %v", err)
			}
			b, err := fs.Read("/" + tc.mount + "/update.md")
			if err != nil {
				t.Fatalf("Read update.md: %v", err)
			}
			if string(b) != "hello" {
				t.Fatalf("unexpected update.md: %q", string(b))
			}

			if err := fs.Append("/"+tc.mount+"/update.md", []byte(" world")); err != nil {
				t.Fatalf("Append update.md: %v", err)
			}
			b, err = fs.Read("/" + tc.mount + "/update.md")
			if err != nil {
				t.Fatalf("Read update.md after append: %v", err)
			}
			if string(b) != "hello world" {
				t.Fatalf("unexpected update.md after append: %q", string(b))
			}

			if err := fs.Write("/"+tc.mount+"/"+tc.mainFile, []byte("nope")); err == nil {
				t.Fatalf("expected %s write to be rejected", tc.mainFile)
			}
			if err := fs.Write("/"+tc.mount+"/commits.jsonl", []byte("nope")); err == nil {
				t.Fatalf("expected commits.jsonl write to be rejected")
			}
		})
	}
}

