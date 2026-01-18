package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/types"
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
	DiskStore
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
	s := &DiskMemoryStore{DiskStore: DiskStore{Dir: baseDir}}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskMemoryStore) ensure() error {
	if s == nil {
		return fmt.Errorf("disk memory store is nil")
	}
	if err := validate.NonEmpty("disk memory store baseDir", s.Dir); err != nil {
		return err
	}
	return s.EnsureDir(s.Dir, "memory.md", "update.md", "commits.jsonl")
}

func (s *DiskMemoryStore) GetMemory(_ context.Context) (string, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, "memory.md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func (s *DiskMemoryStore) AppendMemory(_ context.Context, text string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	p := filepath.Join(s.Dir, "memory.md")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func (s *DiskMemoryStore) GetUpdate(_ context.Context) (string, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, "update.md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func (s *DiskMemoryStore) SetUpdate(_ context.Context, text string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Dir, "update.md"), []byte(text), 0644)
}

func (s *DiskMemoryStore) ClearUpdate(ctx context.Context) error {
	return s.SetUpdate(ctx, "")
}

func (s *DiskMemoryStore) GetCommitLog(_ context.Context) (string, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, "commits.jsonl"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func (s *DiskMemoryStore) AppendCommitLog(_ context.Context, line types.MemoryCommitLine) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if line.Timestamp == "" {
		line.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	b, err := json.Marshal(line)
	if err != nil {
		return err
	}
	p := filepath.Join(s.Dir, "commits.jsonl")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
