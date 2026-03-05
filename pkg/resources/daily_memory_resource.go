package resources

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
	"github.com/tinoosan/agen8/pkg/vfsutil"
)

// DailyMemoryResource exposes the /memory mount backed by daily files on disk.
// It enforces that only today's file is writable; past days and MEMORY.MD are read-only.
type DailyMemoryResource struct {
	BaseDir string
}

var dailyNameRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}-memory\.md$`)

// NewDailyMemoryResource creates a resource for daily memory files.
func NewDailyMemoryResource(baseDir string) (*DailyMemoryResource, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir is required")
	}
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir memory dir: %w", err)
	}
	return &DailyMemoryResource{
		BaseDir: baseDir,
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
	return nil
}

func (r *DailyMemoryResource) Search(ctx context.Context, subpath string, req types.SearchRequest) (types.SearchResponse, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return types.SearchResponse{}, err
	}
	if clean != "" && clean != "." {
		return types.SearchResponse{}, fmt.Errorf("invalid subpath %q: search only supported at root", subpath)
	}

	spec, err := compileSearchRequest(req)
	if err != nil {
		return types.SearchResponse{}, err
	}
	if files, rgTried, rgErr := rgFilesWithMatches(ctx, rgFilesWithMatchesOpts{
		Dir:          r.BaseDir,
		Query:        spec.effectiveQuery(),
		FixedString:  !spec.usesRegex(),
		MaxFilesize:  spec.rgMaxFileSize(),
		IncludeGlobs: []string{"MEMORY.MD", "*-memory.md"},
		ExcludeGlobs: spec.excludeGlobs,
		MaxFiles:     max(1000, spec.limit*200),
	}); rgTried && rgErr == nil {
		allow := func(name string) bool {
			return strings.EqualFold(name, "MEMORY.MD") || dailyNameRE.MatchString(name)
		}
		// Enforce allowlist (rg globs are broader than our naming rules).
		filtered := make([]string, 0, len(files))
		for _, name := range files {
			name = normalizeSearchPath(name)
			if strings.Contains(name, "/") || strings.Contains(name, "\\") {
				continue
			}
			if !allow(name) {
				continue
			}
			if !spec.allowRelativePath(name) {
				continue
			}
			filtered = append(filtered, name)
		}
		sort.Strings(filtered)

		out := make([]types.SearchResult, 0, min(spec.limit, len(filtered)))
		for _, name := range filtered {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return types.SearchResponse{Results: out, Returned: len(out)}, ctx.Err()
				default:
				}
			}
			abs := filepath.Join(r.BaseDir, name)
			info, statErr := os.Stat(abs)
			if statErr != nil || info.IsDir() || spec.shouldSkipSize(info.Size()) {
				continue
			}
			m, matchErr := bestMatchInFile(ctx, abs, spec)
			if matchErr != nil {
				if ctx != nil && (errors.Is(matchErr, context.Canceled) || errors.Is(matchErr, context.DeadlineExceeded)) {
					return types.SearchResponse{Results: out, Returned: len(out)}, matchErr
				}
				continue
			}
			if m.score <= 0 {
				continue
			}
			result, buildErr := buildSearchResult(name, "/memory/"+name, m, abs, spec)
			if buildErr != nil {
				continue
			}
			out = append(out, result)
		}
		return finalizeSearchResults(out, spec.limit), nil
	}

	return searchTextFiles(ctx, r.BaseDir, spec, func(name string) bool {
		return strings.EqualFold(name, "MEMORY.MD") || dailyNameRE.MatchString(name)
	}, "/memory/")
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

func searchTextFiles(ctx context.Context, baseDir string, spec compiledSearchRequest, allow func(name string) bool, vfsPrefix string) (types.SearchResponse, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		return types.SearchResponse{}, fmt.Errorf("baseDir is required")
	}

	des, err := os.ReadDir(baseDir)
	if err != nil {
		return types.SearchResponse{}, err
	}
	names := make([]string, 0, len(des))
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		name := de.Name()
		if allow != nil && !allow(name) {
			continue
		}
		if !spec.allowRelativePath(name) {
			continue
		}
		if info, ierr := de.Info(); ierr == nil && spec.shouldSkipSize(info.Size()) {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	results := make([]types.SearchResult, 0, spec.limit)
	for _, name := range names {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return types.SearchResponse{Results: results, Returned: len(results)}, ctx.Err()
			default:
			}
		}

		p := filepath.Join(baseDir, name)
		match, matchErr := bestMatchInFile(ctx, p, spec)
		if matchErr != nil || match.score <= 0 {
			continue
		}
		result, buildErr := buildSearchResult(name, strings.TrimRight(vfsPrefix, "/")+"/"+name, match, p, spec)
		if buildErr != nil {
			continue
		}
		results = append(results, result)
	}
	return finalizeSearchResults(results, spec.limit), nil
}
