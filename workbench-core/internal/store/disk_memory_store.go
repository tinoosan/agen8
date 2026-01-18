package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/types"
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
	BaseDir string
}

// NewDiskMemoryStore constructs a DiskMemoryStore for a runId under cfg.DataDir.
func NewDiskMemoryStore(cfg config.Config, runId string) (*DiskMemoryStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runId) == "" {
		return nil, fmt.Errorf("runId is required")
	}
	baseDir := fsutil.GetRunMemoryDir(cfg.DataDir, runId)
	return NewDiskMemoryStoreFromDir(baseDir)
}

// NewDiskMemoryStoreFromDir constructs a DiskMemoryStore rooted at baseDir.
func NewDiskMemoryStoreFromDir(baseDir string) (*DiskMemoryStore, error) {
	if strings.TrimSpace(baseDir) == "" {
		return nil, fmt.Errorf("baseDir is required")
	}
	s := &DiskMemoryStore{BaseDir: baseDir}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskMemoryStore) ensure() error {
	if s == nil || strings.TrimSpace(s.BaseDir) == "" {
		return fmt.Errorf("disk memory store baseDir is required")
	}
	if err := os.MkdirAll(s.BaseDir, 0755); err != nil {
		return err
	}
	// Ensure files exist so reads behave like empty rather than "file not found".
	for _, name := range []string{"memory.md", "update.md", "commits.jsonl"} {
		p := filepath.Join(s.BaseDir, name)
		if _, err := os.Stat(p); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.WriteFile(p, []byte{}, 0644); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *DiskMemoryStore) GetMemory(_ context.Context) (string, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.BaseDir, "memory.md"))
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
	p := filepath.Join(s.BaseDir, "memory.md")
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
	b, err := os.ReadFile(filepath.Join(s.BaseDir, "update.md"))
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
	return os.WriteFile(filepath.Join(s.BaseDir, "update.md"), []byte(text), 0644)
}

func (s *DiskMemoryStore) ClearUpdate(ctx context.Context) error {
	return s.SetUpdate(ctx, "")
}

func (s *DiskMemoryStore) GetCommitLog(_ context.Context) (string, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.BaseDir, "commits.jsonl"))
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
	p := filepath.Join(s.BaseDir, "commits.jsonl")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
