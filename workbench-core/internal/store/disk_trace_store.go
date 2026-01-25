package store

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/bytesutil"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/validate"
)

const (
	defaultTraceSinceMaxBytes  = 64 * 1024
	defaultTraceSinceLimit     = 200
	defaultTraceLatestMaxBytes = 64 * 1024
	defaultTraceLatestLimit    = 200
)

// DiskTraceStore reads the current on-disk trace format:
//
//	data/runs/<runId>/log/events.jsonl
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

func (s DiskTraceStore) EventsSince(_ context.Context, cursor TraceCursor, opts TraceSinceOptions) (TraceBatch, error) {
	if err := s.ensure(); err != nil {
		return TraceBatch{}, err
	}
	offset, err := TraceCursorToInt64(cursor)
	if err != nil {
		return TraceBatch{}, fmt.Errorf("disk trace store: invalid cursor: %w", ErrInvalid)
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultTraceSinceMaxBytes
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultTraceSinceLimit
	}

	p := filepath.Join(s.Dir, "events.jsonl")
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TraceBatch{CursorAfter: TraceCursorFromInt64(offset)}, nil
		}
		return TraceBatch{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return TraceBatch{}, err
	}
	size := info.Size()
	if offset > size {
		offset = size
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return TraceBatch{}, err
	}

	lr := io.LimitReader(f, int64(maxBytes))
	r := bufio.NewReader(lr)

	var (
		events        []TraceEvent
		bytesConsumed int64
		linesTotal    int
		parsed        int
		parseErrors   int
	)

	for len(events) < limit {
		lineStart := offset + bytesConsumed
		line, readErr := r.ReadBytes('\n')
		if len(line) == 0 && readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return TraceBatch{}, readErr
		}

		// If we hit the maxBytes cap mid-line, do NOT consume the partial line.
		// This keeps cursorAfter stable and ensures the next call can re-read the full line.
		if readErr == io.EOF && len(line) > 0 && !bytes.HasSuffix(line, []byte("\n")) && lineStart+int64(len(line)) < size {
			break
		}

		bytesConsumed += int64(len(line))
		linesTotal++

		text := strings.TrimSpace(string(bytesutil.TrimRightNewlines(line)))
		if text == "" {
			if errors.Is(readErr, io.EOF) {
				break
			}
			continue
		}

		var ev types.Event
		if err := json.Unmarshal([]byte(text), &ev); err != nil {
			parseErrors++
		} else {
			parsed++
			events = append(events, TraceEvent{
				Timestamp: ev.Timestamp.UTC().Format(time.RFC3339Nano),
				Type:      ev.Type,
				Message:   ev.Message,
				Data:      ev.Data,
			})
		}

		if errors.Is(readErr, io.EOF) {
			break
		}
	}

	cursorAfter := offset + bytesConsumed
	if cursorAfter > size {
		cursorAfter = size
	}
	truncated := cursorAfter < size

	return TraceBatch{
		Events:         events,
		CursorAfter:    TraceCursorFromInt64(cursorAfter),
		BytesRead:      int(bytesConsumed),
		LinesTotal:     linesTotal,
		Parsed:         parsed,
		ParseErrors:    parseErrors,
		Returned:       len(events),
		ReturnedCapped: len(events) >= limit,
		Truncated:      truncated,
	}, nil
}

func (s DiskTraceStore) EventsLatest(_ context.Context, opts TraceLatestOptions) (TraceBatch, error) {
	if err := s.ensure(); err != nil {
		return TraceBatch{}, err
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultTraceLatestMaxBytes
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultTraceLatestLimit
	}

	p := filepath.Join(s.Dir, "events.jsonl")
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TraceBatch{CursorAfter: TraceCursorFromInt64(0)}, nil
		}
		return TraceBatch{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return TraceBatch{}, err
	}
	size := info.Size()
	if size == 0 {
		return TraceBatch{CursorAfter: TraceCursorFromInt64(0)}, nil
	}

	readSize := int64(maxBytes)
	if readSize > size {
		readSize = size
	}
	start := size - readSize
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return TraceBatch{}, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return TraceBatch{}, err
	}

	// Split into lines; first line may be partial if we started mid-file.
	rawLines := strings.Split(string(b), "\n")
	linesTotal := 0
	parsed := 0
	parseErrors := 0
	var parsedEvents []TraceEvent

	// Parse in order, skipping empty/partial prefixes that fail to unmarshal.
	for _, ln := range rawLines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		linesTotal++
		var ev types.Event
		if err := json.Unmarshal([]byte(ln), &ev); err != nil {
			parseErrors++
			continue
		}
		parsed++
		parsedEvents = append(parsedEvents, TraceEvent{
			Timestamp: ev.Timestamp.UTC().Format(time.RFC3339Nano),
			Type:      ev.Type,
			Message:   ev.Message,
			Data:      ev.Data,
		})
	}

	// Keep only last N, preserving chronological order.
	if len(parsedEvents) > limit {
		parsedEvents = parsedEvents[len(parsedEvents)-limit:]
	}

	return TraceBatch{
		Events:         parsedEvents,
		CursorAfter:    TraceCursorFromInt64(size),
		BytesRead:      len(b),
		LinesTotal:     linesTotal,
		Parsed:         parsed,
		ParseErrors:    parseErrors,
		Returned:       len(parsedEvents),
		ReturnedCapped: len(parsedEvents) >= limit,
		Truncated:      readSize < size || len(parsedEvents) >= limit,
	}, nil
}
