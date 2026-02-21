package store

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tinoosan/agen8/pkg/bytesutil"
	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/validate"
)

// DiskHistoryStore is a session-scoped HistoryStore backed by the on-disk history layout:
//
//	data/sessions/<sessionId>/history/history.jsonl
//
// History is append-only: the store supports reading the full log and appending new
// JSONL lines. Higher-level components decide what to record and with what metadata.
type DiskHistoryStore struct {
	DiskStore
}

// NewDiskHistoryStore constructs a DiskHistoryStore for a sessionID under cfg.DataDir.
func NewDiskHistoryStore(cfg config.Config, sessionID string) (*DiskHistoryStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := validate.NonEmpty("sessionId", sessionID); err != nil {
		return nil, err
	}
	return NewDiskHistoryStoreFromPath(fsutil.GetSessionHistoryPath(cfg.DataDir, sessionID))
}

// NewDiskHistoryStoreFromPath constructs a DiskHistoryStore that reads/appends to path.
func NewDiskHistoryStoreFromPath(path string) (*DiskHistoryStore, error) {
	if err := validate.NonEmpty("path", path); err != nil {
		return nil, err
	}
	s := &DiskHistoryStore{DiskStore: DiskStore{Path: path}}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *DiskHistoryStore) ensure() error {
	if s == nil {
		return fmt.Errorf("disk history store is nil")
	}
	if err := validate.NonEmpty("disk history store path", s.Path); err != nil {
		return err
	}
	return s.EnsureFile(s.Path)
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
	b = bytesutil.TrimRightNewlines(b)
	b = append(b, '\n')

	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(b)
	return err
}

func (s *DiskHistoryStore) LinesSince(_ context.Context, cursor pkgstore.HistoryCursor, opts pkgstore.HistorySinceOptions) (pkgstore.HistoryBatch, error) {
	if err := s.ensure(); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: cursor}, err
	}

	maxBytes, limit := normalizeHistoryLimits(opts.MaxBytes, opts.Limit)

	offset, err := pkgstore.HistoryCursorToInt64(cursor)
	if err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, fmt.Errorf("invalid cursor: %w", ErrInvalid)
	}

	f, err := os.Open(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(offset)}, nil
		}
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(offset)}, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(offset)}, err
	}
	size := st.Size()
	if offset > size {
		offset = size
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(offset)}, err
	}

	r := bufio.NewReader(f)
	var out [][]byte
	var bytesRead int
	var linesTotal int
	truncated := false

	for {
		if bytesRead >= maxBytes {
			truncated = true
			break
		}
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			bytesRead += len(line)
			linesTotal++
			trim := bytesutil.TrimRightNewlines(line)
			if len(trim) > 0 {
				out = append(out, append([]byte(nil), trim...))
			}
			if len(out) >= limit {
				truncated = true
				break
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(offset)}, err
		}
	}

	after := offset + int64(bytesRead)
	if after > size {
		after = size
	}

	return pkgstore.HistoryBatch{
		Lines:          out,
		CursorAfter:    pkgstore.HistoryCursorFromInt64(after),
		BytesRead:      bytesRead,
		LinesTotal:     linesTotal,
		Returned:       len(out),
		ReturnedCapped: len(out) >= limit,
		Truncated:      truncated,
	}, nil
}

func (s *DiskHistoryStore) LinesLatest(_ context.Context, opts pkgstore.HistoryLatestOptions) (pkgstore.HistoryBatch, error) {
	if err := s.ensure(); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, err
	}

	maxBytes, limit := normalizeHistoryLimits(opts.MaxBytes, opts.Limit)

	f, err := os.Open(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, nil
		}
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, err
	}
	size := st.Size()
	if size == 0 {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, nil
	}

	start := size - int64(maxBytes)
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, err
	}

	// If we started mid-line, discard the partial first line for clean JSONL parsing.
	if start > 0 {
		_, _ = bufio.NewReader(f).ReadBytes('\n')
	}
	b, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)))
	if err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, err
	}

	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024)
	var lines [][]byte
	for sc.Scan() {
		line := bytesutil.TrimRightNewlines(sc.Bytes())
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines = append(lines, append([]byte(nil), line...))
	}
	if err := sc.Err(); err != nil {
		return pkgstore.HistoryBatch{CursorAfter: pkgstore.HistoryCursorFromInt64(0)}, err
	}

	// Keep last N lines.
	truncated := false
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
		truncated = true
	}

	return pkgstore.HistoryBatch{
		Lines:          lines,
		CursorAfter:    pkgstore.HistoryCursorFromInt64(size),
		BytesRead:      len(b),
		LinesTotal:     len(lines),
		Returned:       len(lines),
		ReturnedCapped: truncated,
		Truncated:      truncated || start > 0,
	}, nil
}
