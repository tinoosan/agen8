package resources

import (
	"testing"

	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func TestVirtualProfileResource_ReadWriteUpdate(t *testing.T) {
	tmp := t.TempDir()
	ps, err := store.NewDiskProfileStoreFromDir(tmp)
	if err != nil {
		t.Fatalf("NewDiskProfileStoreFromDir: %v", err)
	}
	r, err := NewProfileResource(ps)
	if err != nil {
		t.Fatalf("NewProfileResource: %v", err)
	}

	fs := vfs.NewFS()
	fs.Mount(vfs.MountProfile, r)

	if err := fs.Write("/profile/update.md", []byte("birthday: 1994-11-27\n")); err != nil {
		t.Fatalf("Write update: %v", err)
	}
	b, err := fs.Read("/profile/update.md")
	if err != nil {
		t.Fatalf("Read update: %v", err)
	}
	if string(b) == "" {
		t.Fatalf("expected update contents")
	}

	if err := fs.Write("/profile/profile.md", []byte("nope")); err == nil {
		t.Fatalf("expected profile.md write to fail")
	}
}
