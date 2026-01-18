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
	BaseDir string
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
	s := &DiskProfileStore{BaseDir: baseDir}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskProfileStore) ensure() error {
	if s == nil {
		return fmt.Errorf("disk profile store is nil")
	}
	if err := validate.NonEmpty("disk profile store baseDir", s.BaseDir); err != nil {
		return err
	}
	if err := os.MkdirAll(s.BaseDir, 0755); err != nil {
		return err
	}
	for _, name := range []string{"profile.md", "update.md", "commits.jsonl"} {
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

func (s *DiskProfileStore) GetProfile(_ context.Context) (string, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.BaseDir, "profile.md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func (s *DiskProfileStore) AppendProfile(_ context.Context, text string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	p := filepath.Join(s.BaseDir, "profile.md")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func (s *DiskProfileStore) GetUpdate(_ context.Context) (string, error) {
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

func (s *DiskProfileStore) SetUpdate(_ context.Context, text string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.BaseDir, "update.md"), []byte(text), 0644)
}

func (s *DiskProfileStore) ClearUpdate(ctx context.Context) error {
	return s.SetUpdate(ctx, "")
}

func (s *DiskProfileStore) GetCommitLog(_ context.Context) (string, error) {
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

func (s *DiskProfileStore) AppendCommitLog(_ context.Context, line types.MemoryCommitLine) error {
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
