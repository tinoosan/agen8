// Package resources provides concrete implementations of the vfs.Resource interface.
package resources

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
	"github.com/tinoosan/agen8/pkg/vfsutil"
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

func (d *DirResource) Search(ctx context.Context, subpath string, req types.SearchRequest) (types.SearchResponse, error) {
	cleanSubpath, _, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return types.SearchResponse{}, err
	}
	if cleanSubpath == "." {
		cleanSubpath = ""
	}

	targetPath, err := d.safeJoin(cleanSubpath)
	if err != nil {
		return types.SearchResponse{}, fmt.Errorf("safeJoin %s: %w", subpath, err)
	}
	spec, err := compileSearchRequest(req)
	if err != nil {
		return types.SearchResponse{}, err
	}
	results := make([]types.SearchResult, 0, spec.limit)

	if files, rgTried, rgErr := rgFilesWithMatches(ctx, rgFilesWithMatchesOpts{
		Dir:          targetPath,
		Query:        spec.effectiveQuery(),
		FixedString:  !spec.usesRegex(),
		MaxFilesize:  spec.rgMaxFileSize(),
		IncludeGlobs: includeGlobs(spec.includeGlob),
		ExcludeGlobs: spec.excludeGlobs,
		MaxFiles:     max(2000, spec.limit*500),
	}); rgTried && rgErr == nil {
		for _, relToTarget := range files {
			if ctx != nil {
				select {
				case <-ctx.Done():
					return types.SearchResponse{}, ctx.Err()
				default:
				}
			}
			relToTarget = normalizeSearchPath(relToTarget)
			if !spec.allowRelativePath(relToTarget) {
				continue
			}
			abs := filepath.Join(targetPath, filepath.FromSlash(relToTarget))
			info, statErr := os.Stat(abs)
			if statErr != nil || info.IsDir() || spec.shouldSkipSize(info.Size()) {
				continue
			}
			match, matchErr := bestMatchInFile(ctx, abs, spec)
			if matchErr != nil {
				if ctx != nil && errors.Is(matchErr, context.Canceled) {
					return types.SearchResponse{}, matchErr
				}
				continue
			}
			if match.score <= 0 {
				continue
			}
			relToBase := normalizeSearchPath(filepath.Join(cleanSubpath, relToTarget))
			result, buildErr := buildSearchResult(relToBase, "/"+strings.TrimLeft(d.Mount, "/")+"/"+relToBase, match, abs, spec)
			if buildErr != nil {
				continue
			}
			results = append(results, result)
		}
		return finalizeSearchResults(results, spec.limit), nil
	}

	walkFn := func(p string, de fs.DirEntry, walkErr error) error {
		if walkErr != nil {
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
		relToTarget, err := filepath.Rel(targetPath, p)
		if err != nil {
			return nil
		}
		relToTarget = normalizeSearchPath(relToTarget)
		if !spec.allowRelativePath(relToTarget) {
			return nil
		}
		if info, ierr := de.Info(); ierr == nil && spec.shouldSkipSize(info.Size()) {
			return nil
		}
		match, matchErr := bestMatchInFile(ctx, p, spec)
		if matchErr != nil {
			return nil
		}
		if match.score <= 0 {
			return nil
		}
		relToBase := normalizeSearchPath(filepath.Join(cleanSubpath, relToTarget))
		result, buildErr := buildSearchResult(relToBase, "/"+strings.TrimLeft(d.Mount, "/")+"/"+relToBase, match, p, spec)
		if buildErr != nil {
			return nil
		}
		results = append(results, result)
		return nil
	}

	if err := filepath.WalkDir(targetPath, walkFn); err != nil {
		return types.SearchResponse{}, err
	}

	return finalizeSearchResults(results, spec.limit), nil
}

func (d *DirResource) safeJoin(subpath string) (string, error) {
	return vfsutil.SafeJoinBaseDir(d.BaseDir, subpath)
}
