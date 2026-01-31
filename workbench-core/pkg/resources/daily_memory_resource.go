package resources

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
)

// DailyMemoryResource exposes the /memory mount backed by daily files on disk.
// It enforces that only today's file is writable; past days and MEMORY.MD are read-only.
type DailyMemoryResource struct {
	BaseDir     string
	VectorStore MemoryReindexer
	Emit        func(ctx context.Context, ev events.Event)
}

var dailyNameRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-memory\.md$`)

type MemoryReindexer interface {
	ReindexDailyFile(ctx context.Context, filename string, content []byte) error
}

// NewDailyMemoryResource creates a resource for daily memory files.
//
// The vectorStore parameter is optional and can be nil. When provided, memory files
// are reindexed on write. When nil, reindexing is skipped.
func NewDailyMemoryResource(baseDir string, vectorStore MemoryReindexer, emit func(ctx context.Context, ev events.Event)) (*DailyMemoryResource, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir is required")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir memory dir: %w", err)
	}
	return &DailyMemoryResource{
		BaseDir:     baseDir,
		VectorStore: vectorStore,
		Emit:        emit,
	}, nil
}

func (r *DailyMemoryResource) SupportsNestedList() bool {
	return false
}

func (r *DailyMemoryResource) List(subpath string) ([]vfs.Entry, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean != "" && clean != "." {
		return nil, fmt.Errorf("invalid subpath %q: listing only supported at root", subpath)
	}

	des, err := os.ReadDir(r.BaseDir)
	if err != nil {
		return nil, err
	}
	entries := make([]vfs.Entry, 0, len(des))
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if !(strings.EqualFold(name, "MEMORY.MD") || dailyNameRE.MatchString(name)) {
			continue
		}
		info, err := de.Info()
		if err != nil {
			return nil, err
		}
		entries = append(entries, vfs.Entry{
			Path:       name,
			IsDir:      false,
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			HasSize:    true,
			HasModTime: true,
		})
	}
	return entries, nil
}

func (r *DailyMemoryResource) Read(subpath string) ([]byte, error) {
	name, err := r.cleanFile(subpath)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(r.BaseDir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *DailyMemoryResource) Write(subpath string, data []byte) error {
	name, err := r.cleanFile(subpath)
	if err != nil {
		return err
	}
	if err := r.ensureWritable(name); err != nil {
		return err
	}
	path := filepath.Join(r.BaseDir, name)
	if err := fsutil.WriteFileAtomic(path, data, 0644); err != nil {
		return err
	}
	r.reindexBestEffort(name, data)
	return nil
}

func (r *DailyMemoryResource) Append(subpath string, data []byte) error {
	name, err := r.cleanFile(subpath)
	if err != nil {
		return err
	}
	if err := r.ensureWritable(name); err != nil {
		return err
	}
	path := filepath.Join(r.BaseDir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	updated, err := os.ReadFile(path)
	if err == nil {
		r.reindexBestEffort(name, updated)
	}
	return nil
}

func (r *DailyMemoryResource) reindexBestEffort(name string, data []byte) {
	if r == nil || r.VectorStore == nil {
		return
	}
	if !dailyNameRE.MatchString(name) {
		return
	}
	if err := r.VectorStore.ReindexDailyFile(context.Background(), name, data); err != nil {
		if r.Emit != nil {
			r.Emit(context.Background(), events.Event{
				Type:    "memory.reindex.failed",
				Message: "Vector reindex failed",
				Data:    map[string]string{"file": name, "error": err.Error()},
			})
		}
	}
}

func (r *DailyMemoryResource) cleanFile(subpath string) (string, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return "", err
	}
	if clean == "" || clean == "." {
		return "", fmt.Errorf("path required (e.g. %s-memory.md)", time.Now().Format("2006-01-02"))
	}
	// Disallow nested paths.
	if strings.Contains(clean, "/") {
		return "", fmt.Errorf("invalid memory path %q", subpath)
	}
	if strings.EqualFold(clean, "MEMORY.MD") {
		return "MEMORY.MD", nil
	}
	if !dailyNameRE.MatchString(clean) {
		return "", fmt.Errorf("memory files must match YYYY-MM-DD-memory.md")
	}
	return clean, nil
}

func (r *DailyMemoryResource) ensureWritable(name string) error {
	if strings.EqualFold(name, "MEMORY.MD") {
		return fmt.Errorf("MEMORY.MD is read-only")
	}
	today := time.Now().Format("2006-01-02") + "-memory.md"
	if name != today {
		return fmt.Errorf("can only write to today's memory file: %s", today)
	}
	return nil
}
