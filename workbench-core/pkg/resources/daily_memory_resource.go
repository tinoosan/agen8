package resources

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
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

func (r *DailyMemoryResource) Search(ctx context.Context, subpath string, query string, limit int) ([]types.SearchResult, error) {
	clean, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return nil, err
	}
	if clean != "" && clean != "." {
		return nil, fmt.Errorf("invalid subpath %q: search only supported at root", subpath)
	}

	re, reOK, qLower := compileSearchQuery(query)
	if files, rgTried, rgErr := rgFilesWithMatches(ctx, rgFilesWithMatchesOpts{
		Dir:         r.BaseDir,
		Query:       query,
		RegexOK:     reOK,
		MaxFilesize: "2M",
		Globs:       []string{"MEMORY.MD", "*-memory.md"},
		MaxFiles:    max(1000, limit*200),
	}); rgTried && rgErr == nil {
		if len(files) == 0 {
			return nil, nil
		}
		allow := func(name string) bool {
			return strings.EqualFold(name, "MEMORY.MD") || dailyNameRE.MatchString(name)
		}
		// Enforce allowlist (rg globs are broader than our naming rules).
		filtered := make([]string, 0, len(files))
		for _, name := range files {
			name = strings.TrimPrefix(name, "./")
			name = strings.TrimPrefix(name, ".\\")
			if strings.Contains(name, "/") || strings.Contains(name, "\\") {
				continue
			}
			if !allow(name) {
				continue
			}
			filtered = append(filtered, name)
		}
		sort.Strings(filtered)

		if limit <= 0 {
			limit = 5
		}
		out := make([]types.SearchResult, 0, min(limit, len(filtered)))
		for _, name := range filtered {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return out, ctx.Err()
				default:
				}
			}
			m, err := bestMatchInFile(ctx, filepath.Join(r.BaseDir, name), re, reOK, qLower)
			if err != nil {
				if ctx != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
					return out, err
				}
				continue
			}
			if m.score <= 0 {
				continue
			}
			out = append(out, types.SearchResult{
				Title:   name,
				Path:    "/memory/" + name,
				Snippet: fmt.Sprintf("%s:%d: %s", name, m.line, m.text),
				Score:   m.score,
			})
			if len(out) >= limit {
				break
			}
		}
		return out, nil
	}

	return searchTextFiles(ctx, r.BaseDir, query, limit, func(name string) bool {
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

func searchTextFiles(ctx context.Context, baseDir string, query string, limit int, allow func(name string) bool, vfsPrefix string) ([]types.SearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	baseDir = strings.TrimSpace(baseDir)
	query = strings.TrimSpace(query)
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir is required")
	}
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	re, reErr := regexp.Compile(query)
	reOK := reErr == nil
	qLower := strings.ToLower(query)

	des, err := os.ReadDir(baseDir)
	if err != nil {
		return nil, err
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
		names = append(names, name)
	}
	sort.Strings(names)

	results := make([]types.SearchResult, 0, limit)
	for _, name := range names {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			default:
			}
		}

		p := filepath.Join(baseDir, name)
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 1024*1024)

		lineN := 0
		bestScore := 0.0
		bestSnippet := ""
		for sc.Scan() {
			lineN++
			ln := sc.Text()
			match := false
			matchN := 0
			if reOK && re != nil {
				idxs := re.FindAllStringIndex(ln, -1)
				if len(idxs) != 0 {
					match = true
					matchN = len(idxs)
				}
			} else {
				if strings.Contains(strings.ToLower(ln), qLower) {
					match = true
					matchN = 1
				}
			}
			if !match {
				continue
			}
			score := float64(matchN)
			if score > bestScore {
				bestScore = score
				bestSnippet = fmt.Sprintf("%s:%d: %s", name, lineN, strings.TrimSpace(ln))
			}
		}
		_ = f.Close()
		if bestScore <= 0 || bestSnippet == "" {
			continue
		}

		results = append(results, types.SearchResult{
			Title:   name,
			Path:    strings.TrimRight(vfsPrefix, "/") + "/" + name,
			Snippet: bestSnippet,
			Score:   bestScore,
		})
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}
