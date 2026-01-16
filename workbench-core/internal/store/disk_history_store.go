package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
)

// DiskHistoryStore is a run-scoped HistoryStore backed by the on-disk history layout:
//
//	data/runs/<runId>/history/history.jsonl
//
// History is append-only: the store supports reading the full log and appending new
// JSONL lines. Higher-level components decide what to record and with what metadata.
type DiskHistoryStore struct {
	Path string
}

// NewDiskHistoryStore constructs a DiskHistoryStore for a runId under config.DataDir.
func NewDiskHistoryStore(runId string) (*DiskHistoryStore, error) {
	if strings.TrimSpace(runId) == "" {
		return nil, fmt.Errorf("runId is required")
	}
	return NewDiskHistoryStoreFromPath(fsutil.GetRunHistoryPath(config.DataDir, runId))
}

// NewDiskHistoryStoreFromPath constructs a DiskHistoryStore that reads/appends to path.
func NewDiskHistoryStoreFromPath(path string) (*DiskHistoryStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path is required")
	}
	s := &DiskHistoryStore{Path: path}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskHistoryStore) ensure() error {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return fmt.Errorf("disk history store path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return err
	}
	if _, err := os.Stat(s.Path); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.WriteFile(s.Path, []byte{}, 0644); err != nil {
			return err
		}
	}
	return nil
}

func (s *DiskHistoryStore) ReadAll(_ context.Context) ([]byte, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []byte{}, nil
		}
		return nil, err
	}
	return b, nil
}

func (s *DiskHistoryStore) AppendLine(_ context.Context, line []byte) error {
	if err := s.ensure(); err != nil {
		return err
	}
	if line == nil {
		return fmt.Errorf("line is required")
	}
	if len(line) == 0 {
		return nil
	}
	// Ensure exactly one trailing newline.
	b := append([]byte(nil), line...)
	b = bytesTrimRightNewlines(b)
	b = append(b, '\n')

	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

func bytesTrimRightNewlines(b []byte) []byte {
	for len(b) > 0 {
		last := b[len(b)-1]
		if last != '\n' && last != '\r' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}
