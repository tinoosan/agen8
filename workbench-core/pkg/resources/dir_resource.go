// Package resources provides concrete implementations of the vfs.Resource interface.
package resources

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
)

// DirResource implements vfs.Resource by mapping virtual paths to a real OS directory.
type DirResource struct {
	BaseDir string
	Mount   string
}

func NewDirResource(baseDir, mount string) (*DirResource, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("baseDir cannot be empty")
	}
	mount = strings.TrimLeft(mount, "/")
	if mount == "" {
		return nil, fmt.Errorf("mount cannot be empty")
	}

	return &DirResource{
		BaseDir: baseDir,
		Mount:   mount,
	}, nil
}

func (d *DirResource) SupportsNestedList() bool {
	return true
}

func (d *DirResource) List(subpath string) ([]vfs.Entry, error) {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return nil, fmt.Errorf("safeJoin %s: %w", subpath, err)
	}

	des, err := os.ReadDir(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("does not exist: %w", err)
		}
		return nil, fmt.Errorf("list %s: %w", targetPath, err)
	}
	entries := make([]vfs.Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			return nil, fmt.Errorf("stat entry %s: %w", de.Name(), err)
		}

		childSubpath := strings.TrimLeft(filepath.ToSlash(filepath.Join(subpath, de.Name())), "/")
		e := vfs.Entry{
			Path:       childSubpath,
			IsDir:      de.IsDir(),
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			HasSize:    true,
			HasModTime: true,
		}
		entries = append(entries, e)
	}

	return entries, nil
}

func (d *DirResource) Read(subpath string) ([]byte, error) {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", targetPath, err)
	}

	return b, nil
}

func (d *DirResource) Write(subpath string, data []byte) error {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parentDir, err)
	}

	if err := fsutil.WriteFileAtomic(targetPath, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", targetPath, err)
	}
	return nil
}

func (d *DirResource) Append(subpath string, data []byte) error {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return err
	}

	parentDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parentDir, err)
	}

	f, err := os.OpenFile(targetPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open for append %s: %w", targetPath, err)
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("append %s: %w", targetPath, err)
	}

	_ = f.Sync()
	return nil
}

func (d *DirResource) Search(ctx context.Context, subpath string, query string, limit int) ([]types.SearchResult, error) {
	targetPath, err := d.safeJoin(subpath)
	if err != nil {
		return nil, fmt.Errorf("safeJoin %s: %w", subpath, err)
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 5
	}

	re, reErr := regexp.Compile(query)
	reOK := reErr == nil
	qLower := strings.ToLower(query)

	type hit struct {
		path    string
		snippet string
		score   float64
	}
	best := map[string]hit{}

	walkFn := func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
		}
		if de.IsDir() {
			return nil
		}
		// Skip large files by size, best-effort.
		if info, ierr := de.Info(); ierr == nil {
			const max = 2 * 1024 * 1024
			if info.Size() > max {
				return nil
			}
		}
		f, oerr := os.Open(p)
		if oerr != nil {
			return nil
		}
		defer f.Close()
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
				bestSnippet = fmt.Sprintf("%d: %s", lineN, strings.TrimSpace(ln))
			}
		}
		if bestScore <= 0 || bestSnippet == "" {
			return nil
		}
		rel, rerr := filepath.Rel(d.BaseDir, p)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		best[rel] = hit{path: rel, snippet: bestSnippet, score: bestScore}
		return nil
	}

	if err := filepath.WalkDir(targetPath, walkFn); err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(best))
	for p := range best {
		paths = append(paths, p)
	}
	sort.Slice(paths, func(i, j int) bool {
		hi := best[paths[i]]
		hj := best[paths[j]]
		if hi.score != hj.score {
			return hi.score > hj.score
		}
		return hi.path < hj.path
	})
	if len(paths) > limit {
		paths = paths[:limit]
	}

	out := make([]types.SearchResult, 0, len(paths))
	for _, p := range paths {
		h := best[p]
		out = append(out, types.SearchResult{
			Title:   p,
			Path:    "/" + strings.TrimLeft(d.Mount, "/") + "/" + p,
			Snippet: p + ":" + h.snippet,
			Score:   h.score,
		})
	}
	return out, nil
}

func (d *DirResource) safeJoin(subpath string) (string, error) {
	return vfsutil.SafeJoinBaseDir(d.BaseDir, subpath)
}
