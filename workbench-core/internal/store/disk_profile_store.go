package store

import (
	"context"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/validate"
)

// DiskProfileStore is a global ProfileStore backed by an on-disk directory:
//
//	data/profile/
//	  profile.md
//	  update.md
//	  commits.jsonl
//
// This makes the "user profile" inspectable and editable on disk while keeping the VFS
// contract stable and allowing a future swap to other backends (sqlite, cloud, etc).
type DiskProfileStore struct {
	DiskStagingStore
}

// NewDiskProfileStore constructs a DiskProfileStore rooted at cfg.DataDir/profile.
func NewDiskProfileStore(cfg config.Config) (*DiskProfileStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseDir := fsutil.GetProfileDir(cfg.DataDir)
	return NewDiskProfileStoreFromDir(baseDir)
}

// NewDiskProfileStoreFromDir constructs a DiskProfileStore rooted at baseDir.
func NewDiskProfileStoreFromDir(baseDir string) (*DiskProfileStore, error) {
	if err := validate.NonEmpty("baseDir", baseDir); err != nil {
		return nil, err
	}
	s := &DiskProfileStore{
		DiskStagingStore: DiskStagingStore{
			DiskStore:  DiskStore{Dir: baseDir},
			mainFile:   "profile.md",
			storeName:  "disk profile store",
		},
	}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskProfileStore) GetProfile(ctx context.Context) (string, error) {
	return s.getMain(ctx)
}

func (s *DiskProfileStore) AppendProfile(ctx context.Context, text string) error {
	return s.appendMain(ctx, text)
}
