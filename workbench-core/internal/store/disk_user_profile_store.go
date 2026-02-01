package store

import (
	"context"
	"os"
	"path/filepath"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

// DiskUserProfileStore is a global UserProfileStore backed by an on-disk directory:
//
//	data/user_profile/
//	  user_profile.md
//	  update.md
//	  commits.jsonl
//
// This makes the "user profile" inspectable and editable on disk while keeping the VFS
// contract stable and allowing a future swap to other backends (sqlite, cloud, etc).
type DiskUserProfileStore struct {
	DiskStagingStore
}

// NewDiskUserProfileStore constructs a DiskUserProfileStore rooted at cfg.DataDir/user_profile.
func NewDiskUserProfileStore(cfg config.Config) (*DiskUserProfileStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseDir := fsutil.GetUserProfileDir(cfg.DataDir)
	mainFile := "user_profile.md"

	// Legacy compatibility: older installs used <dataDir>/profile/profile.md.
	legacyDir := filepath.Join(cfg.DataDir, "profile")
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		if st, err := os.Stat(legacyDir); err == nil && st.IsDir() {
			baseDir = legacyDir
			mainFile = "profile.md"
		}
	}
	return newDiskUserProfileStoreFromDir(baseDir, mainFile)
}

// NewDiskUserProfileStoreFromDir constructs a DiskUserProfileStore rooted at baseDir.
func NewDiskUserProfileStoreFromDir(baseDir string) (*DiskUserProfileStore, error) {
	return newDiskUserProfileStoreFromDir(baseDir, "user_profile.md")
}

func newDiskUserProfileStoreFromDir(baseDir string, mainFile string) (*DiskUserProfileStore, error) {
	if err := validate.NonEmpty("baseDir", baseDir); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("mainFile", mainFile); err != nil {
		return nil, err
	}
	s := &DiskUserProfileStore{
		DiskStagingStore: DiskStagingStore{
			DiskStore: DiskStore{Dir: baseDir},
			mainFile:  mainFile,
			storeName: "disk user profile store",
		},
	}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskUserProfileStore) GetUserProfile(ctx context.Context) (string, error) {
	return s.getMain(ctx)
}

func (s *DiskUserProfileStore) AppendUserProfile(ctx context.Context, text string) error {
	return s.appendMain(ctx, text)
}
