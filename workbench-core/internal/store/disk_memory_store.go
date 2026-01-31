package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	pkgstore "github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

// DiskMemoryStore is a shared MemoryStore backed by the on-disk layout:
//
//	data/memory/
//	  memory.md
//	  update.md
//	  commits.jsonl
//
// This keeps memory stable across runs while the rest of the system interacts only
// via the virtual "/memory" mount.
type DiskMemoryStore struct {
	DiskStagingStore
}

// NewDiskMemoryStore constructs a DiskMemoryStore under cfg.DataDir.
func NewDiskMemoryStore(cfg config.Config) (*DiskMemoryStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseDir := fsutil.GetMemoryDir(cfg.DataDir)
	return NewDiskMemoryStoreFromDir(baseDir)
}

// NewDiskMemoryStoreFromDir constructs a DiskMemoryStore rooted at baseDir.
func NewDiskMemoryStoreFromDir(baseDir string) (*DiskMemoryStore, error) {
	if err := validate.NonEmpty("baseDir", baseDir); err != nil {
		return nil, err
	}
	s := &DiskMemoryStore{
		DiskStagingStore: DiskStagingStore{
			DiskStore: DiskStore{Dir: baseDir},
			mainFile:  "memory.md",
			storeName: "disk memory store",
		},
	}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	if err := s.ensurePlan(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskMemoryStore) GetMemory(ctx context.Context) (string, error) {
	return s.getMain(ctx)
}

func (s *DiskMemoryStore) AppendMemory(ctx context.Context, text string) error {
	return s.appendMain(ctx, text)
}

func (s *DiskMemoryStore) ensurePlan() error {
	return s.EnsureFile(filepath.Join(s.Dir, pkgstore.PlanFileName))
}

func (s *DiskMemoryStore) GetPlan(ctx context.Context) (string, error) {
	if err := s.ensurePlan(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, pkgstore.PlanFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func (s *DiskMemoryStore) SetPlan(ctx context.Context, text string) error {
	if err := s.ensurePlan(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, pkgstore.PlanFileName), []byte(text), 0644)
}
