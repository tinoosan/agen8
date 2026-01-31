package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/vfs"
	"github.com/tinoosan/workbench-core/pkg/vfsutil"
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
		return r.listNamespaces()
	}

	dir := parts[0]
	subpath := ""
	if len(parts) > 1 {
		subpath = strings.Join(parts[1:], "/")
	}

	skill, ok := r.manager.Get(dir)
	if !ok {
		return nil, fmt.Errorf("skills: not found %q", dir)
	}
	if subpath == "" {
		return r.listSkillDir(skill.Path)
	}

	full, err := vfsutil.SafeJoinBaseDir(skill.Path, subpath)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(full)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("skills: %q is not a directory", subpath)
	}
	return r.listSkillDir(full)
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
	dir := parts[0]
	subpath := ""
	if len(parts) > 1 {
		subpath = strings.Join(parts[1:], "/")
	}
	skill, ok := r.manager.Get(dir)
	if !ok {
		return nil, fmt.Errorf("skills: not found %q", dir)
	}
	if subpath == "" {
		return nil, fmt.Errorf("skills read: path required")
	}
	full, err := vfsutil.SafeJoinBaseDir(skill.Path, subpath)
	if err != nil {
		return nil, err
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
	dir := parts[0]
	subpath := ""
	if len(parts) > 1 {
		subpath = strings.Join(parts[1:], "/")
	}
	if subpath == "" {
		return fmt.Errorf("skills write: subpath required")
	}
	full, err := r.manager.resolveWritablePath(dir, subpath)
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
	path = strings.Trim(path, "/")
	if path == "" {
		return fmt.Errorf("skills append: path required")
	}
	parts := strings.SplitN(path, "/", 2)
	dir := parts[0]
	subpath := ""
	if len(parts) == 2 {
		subpath = parts[1]
	}
	if subpath == "" {
		return fmt.Errorf("skills append: subpath required")
	}
	full, err := r.manager.resolveWritablePath(dir, subpath)
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

func (r *SkillsResource) listNamespaces() ([]vfs.Entry, error) {
	entries := r.manager.Entries()
	if len(entries) == 0 {
		return []vfs.Entry{}, nil
	}
	out := make([]vfs.Entry, 0, len(entries))
	for _, e := range entries {
		out = append(out, vfs.NewDirEntry(e.Dir))
	}
	return out, nil
}

func (r *SkillsResource) listSkillDir(dir string) ([]vfs.Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]vfs.Entry, 0, len(des))
	for _, de := range des {
		name := de.Name()
		if de.IsDir() {
			out = append(out, vfs.NewDirEntry(name))
			continue
		}
		info, err := de.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, vfs.NewFileEntry(name, info.Size(), info.ModTime()))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
