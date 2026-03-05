package skills

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/resources"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
	"github.com/tinoosan/agen8/pkg/vfsutil"
)

// SkillsResource exposes discovered skills via a virtual /skills mount.
type SkillsResource struct {
	manager *Manager
}

func NewResource(manager *Manager) *SkillsResource {
	return &SkillsResource{manager: manager}
}

func (r *SkillsResource) SupportsNestedList() bool {
	return true
}

func (r *SkillsResource) List(path string) ([]vfs.Entry, error) {
	if r.manager == nil {
		return nil, fmt.Errorf("skills manager is required")
	}

	clean, parts, err := vfsutil.NormalizeResourceSubpath(path)
	if err != nil {
		return nil, err
	}
	if clean == "" || clean == "." {
		return r.listSkillDirs()
	}

	skillName := strings.TrimSpace(parts[0])
	if skillName == "" {
		return nil, fmt.Errorf("skills: invalid path %q", path)
	}
	skill, ok := r.manager.Get(skillName)
	if !ok {
		return nil, fmt.Errorf("skills: not found %q", skillName)
	}
	sub := ""
	if len(parts) > 1 {
		sub = filepath.ToSlash(filepath.Join(parts[1:]...))
	}
	return r.listSkillDir(skill, filepath.ToSlash(filepath.Join(skillName, sub)), sub)
}

func (r *SkillsResource) Read(path string) ([]byte, error) {
	if r.manager == nil {
		return nil, fmt.Errorf("skills manager is required")
	}
	clean, parts, err := vfsutil.NormalizeResourceSubpath(path)
	if err != nil {
		return nil, fmt.Errorf("skills read: %w", err)
	}
	if clean == "" || clean == "." {
		return nil, fmt.Errorf("skills read: path required")
	}
	if len(parts) < 2 {
		return nil, fmt.Errorf("skills read: path must be /skills/<skill>/<file>")
	}
	skillName := strings.TrimSpace(parts[0])
	skill, ok := r.manager.Get(skillName)
	if !ok {
		return nil, fmt.Errorf("skills: not found %q", skillName)
	}
	sub := filepath.ToSlash(filepath.Join(parts[1:]...))
	full, err := vfsutil.SafeJoinBaseDir(skill.Dir, sub)
	if err != nil {
		return nil, fmt.Errorf("skills read: %w", err)
	}
	return os.ReadFile(full)
}

func (r *SkillsResource) Write(path string, data []byte) error {
	if r.manager == nil {
		return fmt.Errorf("skills manager is required")
	}
	clean, parts, err := vfsutil.NormalizeResourceSubpath(path)
	if err != nil {
		return fmt.Errorf("skills write: %w", err)
	}
	if clean == "" || clean == "." {
		return fmt.Errorf("skills write: path required")
	}
	if len(parts) < 2 {
		return fmt.Errorf("skills write: path must be /skills/<skill>/<file>")
	}
	skillName := strings.TrimSpace(parts[0])
	if !r.manager.isAllowed(skillName) {
		return fmt.Errorf("skills: not found %q", skillName)
	}
	sub := filepath.ToSlash(filepath.Join(parts[1:]...))
	full, err := r.manager.resolveWritableFile(skillName, sub)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(full, data, 0644); err != nil {
		return err
	}
	return r.manager.Scan()
}

func (r *SkillsResource) Append(path string, data []byte) error {
	if r.manager == nil {
		return fmt.Errorf("skills manager is required")
	}
	clean, parts, err := vfsutil.NormalizeResourceSubpath(path)
	if err != nil {
		return fmt.Errorf("skills append: %w", err)
	}
	if clean == "" || clean == "." {
		return fmt.Errorf("skills append: path required")
	}
	if len(parts) < 2 {
		return fmt.Errorf("skills append: path must be /skills/<skill>/<file>")
	}
	skillName := strings.TrimSpace(parts[0])
	if !r.manager.isAllowed(skillName) {
		return fmt.Errorf("skills: not found %q", skillName)
	}
	sub := filepath.ToSlash(filepath.Join(parts[1:]...))
	full, err := r.manager.resolveWritableFile(skillName, sub)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(full, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return r.manager.Scan()
}

func (r *SkillsResource) Search(ctx context.Context, subpath string, req types.SearchRequest) (types.SearchResponse, error) {
	if r.manager == nil {
		return types.SearchResponse{}, fmt.Errorf("skills manager is required")
	}
	clean, parts, err := vfsutil.NormalizeResourceSubpath(subpath)
	if err != nil {
		return types.SearchResponse{}, err
	}
	if clean == "." {
		clean = ""
	}
	if clean == "" {
		return r.searchAllSkills(ctx, req)
	}

	skillName := strings.TrimSpace(parts[0])
	if skillName == "" {
		return types.SearchResponse{}, fmt.Errorf("skills: invalid path %q", subpath)
	}
	skill, ok := r.manager.Get(skillName)
	if !ok {
		return types.SearchResponse{}, fmt.Errorf("skills: not found %q", skillName)
	}
	innerSubpath := ""
	if len(parts) > 1 {
		innerSubpath = filepath.ToSlash(filepath.Join(parts[1:]...))
	}
	return searchSingleSkill(ctx, skillName, skill, innerSubpath, req)
}

func (r *SkillsResource) listSkillDirs() ([]vfs.Entry, error) {
	entries := r.manager.Entries()
	if len(entries) == 0 {
		return []vfs.Entry{}, nil
	}
	out := make([]vfs.Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, vfs.NewDirEntry(e.Dir))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (r *SkillsResource) searchAllSkills(ctx context.Context, req types.SearchRequest) (types.SearchResponse, error) {
	entries := r.manager.Entries()
	if len(entries) == 0 {
		return types.SearchResponse{}, nil
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}
	perSkillLimit := max(limit*len(entries), 32)
	childReq := req
	childReq.Limit = perSkillLimit

	allResults := make([]types.SearchResult, 0, perSkillLimit)
	total := 0
	for _, entry := range entries {
		resp, err := searchSingleSkill(ctx, entry.Dir, entry.Skill, "", childReq)
		if err != nil {
			return types.SearchResponse{}, err
		}
		total += resp.Total
		allResults = append(allResults, resp.Results...)
	}
	sort.Slice(allResults, func(i, j int) bool {
		if allResults[i].Score != allResults[j].Score {
			return allResults[i].Score > allResults[j].Score
		}
		return allResults[i].Path < allResults[j].Path
	})
	truncated := false
	if len(allResults) > limit {
		allResults = allResults[:limit]
		truncated = true
	}
	if total > len(allResults) {
		truncated = true
	}
	return types.SearchResponse{
		Results:   allResults,
		Total:     total,
		Returned:  len(allResults),
		Truncated: truncated,
	}, nil
}

func searchSingleSkill(ctx context.Context, skillName string, skill *Skill, subpath string, req types.SearchRequest) (types.SearchResponse, error) {
	if skill == nil || strings.TrimSpace(skill.Dir) == "" {
		return types.SearchResponse{}, fmt.Errorf("skills: invalid skill directory")
	}
	dirRes, err := resources.NewDirResource(skill.Dir, vfs.MountSkills)
	if err != nil {
		return types.SearchResponse{}, err
	}
	resp, err := dirRes.Search(ctx, subpath, req)
	if err != nil {
		return types.SearchResponse{}, err
	}
	for i := range resp.Results {
		resp.Results[i] = prefixSkillSearchResult(skillName, resp.Results[i])
	}
	return resp, nil
}

func prefixSkillSearchResult(skillName string, result types.SearchResult) types.SearchResult {
	prefix := strings.Trim(strings.TrimSpace(skillName), "/")
	if prefix == "" {
		return result
	}
	if result.Title != "" {
		result.Title = prefix + "/" + strings.TrimLeft(result.Title, "/")
	}
	if result.Path != "" {
		path := strings.TrimPrefix(result.Path, "/"+vfs.MountSkills+"/")
		path = strings.TrimLeft(path, "/")
		result.Path = "/" + vfs.MountSkills + "/" + prefix + "/" + path
	}
	if result.Snippet != "" {
		result.Snippet = prefix + "/" + result.Snippet
	}
	return result
}

func (r *SkillsResource) listSkillDir(skill *Skill, listPrefix, subpath string) ([]vfs.Entry, error) {
	if skill == nil || strings.TrimSpace(skill.Dir) == "" {
		return nil, fmt.Errorf("skills: invalid skill directory")
	}
	target, err := vfsutil.SafeJoinBaseDir(skill.Dir, subpath)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(target)
	if err != nil {
		return nil, err
	}
	out := make([]vfs.Entry, 0, len(des))
	for _, de := range des {
		info, err := de.Info()
		if err != nil {
			return nil, err
		}
		child := filepath.ToSlash(filepath.Join(listPrefix, de.Name()))
		if de.IsDir() {
			out = append(out, vfs.NewDirEntry(child))
		} else {
			out = append(out, vfs.NewFileEntry(child, info.Size(), info.ModTime()))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
