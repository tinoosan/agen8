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

	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

// DiskStagingStore implements the common "main content + staging update + commit log"
// pattern used by disk-backed staging stores like /memory.
//
// Layout under s.Dir:
//   - <mainFile>       (committed main content, host-managed; agent can read)
//   - update.md        (staging file, agent can write; host evaluates + commits)
//   - commits.jsonl    (audit log, host-managed; readable for debugging)
type DiskStagingStore struct {
	DiskStore

	mainFile  string // e.g. "memory.md"
	storeName string // e.g. "disk memory store" (for stable error messages)
}

func (s *DiskStagingStore) effectiveStoreName() string {
	if s == nil {
		return "disk staging store"
	}
	name := strings.TrimSpace(s.storeName)
	if name == "" {
		return "disk staging store"
	}
	return name
}

func (s *DiskStagingStore) ensure() error {
	if s == nil {
		return fmt.Errorf("%s is nil", "disk staging store")
	}
	name := s.effectiveStoreName()
	if err := validate.NonEmpty(name+" baseDir", s.Dir); err != nil {
		return err
	}
	if err := validate.NonEmpty(name+" mainFile", s.mainFile); err != nil {
		return err
	}
	return s.EnsureDir(s.Dir, s.mainFile, "update.md", "commits.jsonl")
}

func (s *DiskStagingStore) getMain(_ context.Context) (string, error) {
	if err := s.ensure(); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, s.mainFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return string(b), nil
}

func (s *DiskStagingStore) appendMain(_ context.Context, text string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if text == "" {
		return nil
	}
	p := filepath.Join(s.Dir, s.mainFile)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

func (s *DiskStagingStore) GetUpdate(_ context.Context) (string, error) {
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

func (s *DiskStagingStore) SetUpdate(_ context.Context, text string) error {
	if err := s.ensure(); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(filepath.Join(s.Dir, "update.md"), []byte(text), 0644)
}

func (s *DiskStagingStore) ClearUpdate(ctx context.Context) error {
	return s.SetUpdate(ctx, "")
}

func (s *DiskStagingStore) GetCommitLog(_ context.Context) (string, error) {
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

func (s *DiskStagingStore) AppendCommitLog(_ context.Context, line types.MemoryCommitLine) error {
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
