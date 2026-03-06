package store

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/pkg/bytesutil"
	pkgstore "github.com/tinoosan/agen8/pkg/store"
	"github.com/tinoosan/agen8/pkg/validate"
)

const (
	defaultTraceSinceMaxBytes  = 64 * 1024
	defaultTraceSinceLimit     = 200
	defaultTraceLatestMaxBytes = 64 * 1024
	defaultTraceLatestLimit    = 200
)

// DiskTraceStore reads the current on-disk trace format:
//
//	data/agents/<agentId>/log/events.jsonl
//
// The file is produced by store.AppendEvent, which mirrors canonical run events into
// the trace directory.
type DiskTraceStore struct {
	// Dir is the trace directory (BaseDir of TraceResource).
	// events.jsonl is expected under this directory.
	DiskStore
}

// Kind returns a stable identifier for this store implementation.
func (s DiskTraceStore) Kind() string { return "disk" }

func (s DiskTraceStore) ensure() error {
	if err := validate.NonEmpty("disk trace store Dir", s.Dir); err != nil {
		return err
	}
	// Ensure directory exists and events.jsonl exists so reads behave like empty rather than not-found.
	var ds DiskStore
	return ds.EnsureDir(s.Dir, "events.jsonl")
}

func (s DiskTraceStore) EventsSince(_ context.Context, cursor pkgstore.TraceCursor, opts pkgstore.TraceSinceOptions) (pkgstore.TraceBatch, error) {
	if err := s.ensure(); err != nil {
		return pkgstore.TraceBatch{}, err
	}
	offset, err := pkgstore.TraceCursorToInt64(cursor)
	if err != nil {
		return pkgstore.TraceBatch{}, fmt.Errorf("disk trace store: invalid cursor: %w", ErrInvalid)
	}

	maxBytes, limit := normalizeTraceLimits(opts.MaxBytes, opts.Limit, defaultTraceSinceMaxBytes, defaultTraceSinceLimit)

	p := filepath.Join(s.Dir, "events.jsonl")
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pkgstore.TraceBatch{CursorAfter: pkgstore.TraceCursorFromInt64(offset)}, nil
		}
		return pkgstore.TraceBatch{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return pkgstore.TraceBatch{}, err
	}
	size := info.Size()
	if offset > size {
		offset = size
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return pkgstore.TraceBatch{}, err
	}

	lr := io.LimitReader(f, int64(maxBytes))
	r := bufio.NewReader(lr)

	var (
		records       []traceRecord
		bytesConsumed int64
	)

	for {
		lineStart := offset + bytesConsumed
		line, readErr := r.ReadBytes('\n')
		if len(line) == 0 && readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return pkgstore.TraceBatch{}, readErr
		}

		// If we hit the maxBytes cap mid-line, do NOT consume the partial line.
		// This keeps cursorAfter stable and ensures the next call can re-read the full line.
		if readErr == io.EOF && len(line) > 0 && !bytes.HasSuffix(line, []byte("\n")) && lineStart+int64(len(line)) < size {
			break
		}

		bytesConsumed += int64(len(line))

		text := strings.TrimSpace(string(bytesutil.TrimRightNewlines(line)))
		if text == "" {
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}
		records = append(records, traceRecord{raw: text, size: len(line)})

		if errors.Is(readErr, io.EOF) {
			break
		}
	}

	cursorAfter := offset + bytesConsumed
	if cursorAfter > size {
		cursorAfter = size
	}
	truncated := cursorAfter < size

	result := buildTraceSinceBatch(records, maxBytes, limit)
	result.batch.CursorAfter = pkgstore.TraceCursorFromInt64(cursorAfter)
	result.batch.Truncated = result.batch.Truncated || truncated
	return result.batch, nil
}

func (s DiskTraceStore) EventsLatest(_ context.Context, opts pkgstore.TraceLatestOptions) (pkgstore.TraceBatch, error) {
	if err := s.ensure(); err != nil {
		return pkgstore.TraceBatch{}, err
	}

	maxBytes, limit := normalizeTraceLimits(opts.MaxBytes, opts.Limit, defaultTraceLatestMaxBytes, defaultTraceLatestLimit)

	p := filepath.Join(s.Dir, "events.jsonl")
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return pkgstore.TraceBatch{CursorAfter: pkgstore.TraceCursorFromInt64(0)}, nil
		}
		return pkgstore.TraceBatch{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return pkgstore.TraceBatch{}, err
	}
	size := info.Size()
	if size == 0 {
		return pkgstore.TraceBatch{CursorAfter: pkgstore.TraceCursorFromInt64(0)}, nil
	}

	readSize := int64(maxBytes)
	if readSize > size {
		readSize = size
	}
	start := size - readSize
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return pkgstore.TraceBatch{}, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return pkgstore.TraceBatch{}, err
	}

	// Split into lines; first line may be partial if we started mid-file.
	rawLines := strings.Split(string(b), "\n")
	var records []traceRecord

	// Parse in order, skipping empty/partial prefixes that fail to unmarshal.
	for _, ln := range rawLines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		records = append(records, traceRecord{raw: ln, size: len(ln) + 1})
	}
	batch := buildTraceLatestBatch(records, maxBytes, limit, readSize < size)
	batch.CursorAfter = pkgstore.TraceCursorFromInt64(size)
	return batch, nil
}
