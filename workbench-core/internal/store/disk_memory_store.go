package store

import (
	"context"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/validate"
)

// DiskMemoryStore is a run-scoped MemoryStore backed by the existing on-disk layout:
//
//	data/runs/<runId>/memory/
//	  memory.md
//	  update.md
//	  commits.jsonl
//
// This preserves backward compatibility: runs remain inspectable on disk, but the rest
// of the system interacts only via the virtual "/memory" mount.
type DiskMemoryStore struct {
	DiskStagingStore
}

// NewDiskMemoryStore constructs a DiskMemoryStore for a runId under cfg.DataDir.
func NewDiskMemoryStore(cfg config.Config, runId string) (*DiskMemoryStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("runId", runId); err != nil {
		return nil, err
	}
	baseDir := fsutil.GetRunMemoryDir(cfg.DataDir, runId)
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
	return s, nil
}

func (s *DiskMemoryStore) GetMemory(ctx context.Context) (string, error) {
	return s.getMain(ctx)
}

func (s *DiskMemoryStore) AppendMemory(ctx context.Context, text string) error {
	return s.appendMain(ctx, text)
}
