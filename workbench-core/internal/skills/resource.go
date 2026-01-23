package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// SkillsResource exposes discovered skills via a virtual /skills mount.
type SkillsResource struct {
	manager *Manager
}

// NewResource constructs a SkillsResource backed by the provided manager.
func NewResource(manager *Manager) *SkillsResource {
	return &SkillsResource{manager: manager}
}

// List returns available skill directories or their contents.
func (r *SkillsResource) List(path string) ([]vfs.Entry, error) {
	if r.manager == nil {
		return nil, fmt.Errorf("skills manager is required")
	}

	path = strings.Trim(path, "/")
	if path == "" {
		return r.listNamespaces()
	}

	parts := strings.SplitN(path, "/", 2)
	dir := parts[0]
	subpath := ""
	if len(parts) == 2 {
		subpath = parts[1]
	}

	return r.listSkillDir(dir, subpath)
}

// Read returns a file from within a skill directory.
func (r *SkillsResource) Read(path string) ([]byte, error) {
	if r.manager == nil {
		return nil, fmt.Errorf("skills manager is required")
	}

	path = strings.Trim(path, "/")
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return nil, fmt.Errorf("path must target a file under /skills/<name>")
	}

	skill, ok := r.manager.Get(parts[0])
	if !ok {
		return nil, fmt.Errorf("skill %q not found", parts[0])
	}

	target, err := vfsutil.SafeJoinBaseDir(skill.Path, parts[1])
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", target, err)
	}
	return b, nil
}

func (r *SkillsResource) Write(path string, data []byte) error {
	if r.manager == nil {
		return fmt.Errorf("skills manager is required")
	}
	skill, relPath, err := parseSkillResourcePath(path)
	if err != nil {
		return err
	}
	target, err := r.manager.resolveWritablePath(skill, relPath)
	if err != nil {
		return err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parent, err)
	}
	if err := fsutil.WriteFileAtomic(target, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return r.rescan()
}

func (r *SkillsResource) Append(path string, data []byte) error {
	if r.manager == nil {
		return fmt.Errorf("skills manager is required")
	}
	skill, relPath, err := parseSkillResourcePath(path)
	if err != nil {
		return err
	}
	target, err := r.manager.resolveWritablePath(skill, relPath)
	if err != nil {
		return err
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", parent, err)
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("append %s: %w", target, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("append %s: %w", target, err)
	}
	return r.rescan()
}

func (r *SkillsResource) listNamespaces() ([]vfs.Entry, error) {
	dirs := r.manager.SkillDirs()
	if len(dirs) == 0 {
		return []vfs.Entry{}, nil
	}
	out := make([]vfs.Entry, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, vfs.Entry{Path: dir, IsDir: true})
	}
	return out, nil
}

func (r *SkillsResource) listSkillDir(dir, subpath string) ([]vfs.Entry, error) {
	skill, ok := r.manager.Get(dir)
	if !ok {
		return nil, fmt.Errorf("skill %q not found", dir)
	}

	target, err := vfsutil.SafeJoinBaseDir(skill.Path, subpath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", target, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path %s is not a directory", subpath)
	}

	dirEntries, err := os.ReadDir(target)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", target, err)
	}

	sort.Slice(dirEntries, func(i, j int) bool {
		return dirEntries[i].Name() < dirEntries[j].Name()
	})

	out := make([]vfs.Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		info, err := de.Info()
		if err != nil {
			return nil, fmt.Errorf("stat entry %s: %w", de.Name(), err)
		}

		childRel := filepath.ToSlash(filepath.Join(dir, subpath, de.Name()))
		childRel = strings.TrimLeft(childRel, "/")

		out = append(out, vfs.Entry{
			Path:       childRel,
			IsDir:      de.IsDir(),
			Size:       info.Size(),
			ModTime:    info.ModTime(),
			HasSize:    true,
			HasModTime: true,
		})
	}

	return out, nil
}

func (r *SkillsResource) rescan() error {
	if err := r.manager.Scan(); err != nil {
		return fmt.Errorf("refresh skills: %w", err)
	}
	return nil
}

func parseSkillResourcePath(path string) (string, string, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return "", "", fmt.Errorf("path is required")
	}
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("path must target a file under /skills/<name>/<file>")
	}
	skill, err := sanitizeSkillName(parts[0])
	if err != nil {
		return "", "", err
	}
	rel := strings.TrimLeft(parts[1], "/")
	if rel == "" || rel == "." {
		return "", "", fmt.Errorf("file path required under /skills/%s", skill)
	}
	return skill, rel, nil
}

func (m *Manager) SkillDirs() []string {
	if len(m.entries) == 0 {
		return nil
	}
	dirs := make([]string, 0, len(m.entries))
	for dir := range m.entries {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}
