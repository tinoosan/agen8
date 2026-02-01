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
	return false
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
		return r.listSkillFiles()
	}

	if len(parts) != 1 {
		return nil, fmt.Errorf("skills: nested paths are not supported")
	}
	return nil, fmt.Errorf("skills: %q is a file", parts[0])
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
	if len(parts) != 1 {
		return nil, fmt.Errorf("skills read: path must be a single file under /skills")
	}
	name, err := parseSkillFilename(parts[0])
	if err != nil {
		return nil, fmt.Errorf("skills read: %w", err)
	}
	skill, ok := r.manager.Get(name)
	if !ok {
		return nil, fmt.Errorf("skills: not found %q", name)
	}
	return os.ReadFile(skill.Path)
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
	if len(parts) != 1 {
		return fmt.Errorf("skills write: path must be a single file under /skills")
	}
	name, err := parseSkillFilename(parts[0])
	if err != nil {
		return fmt.Errorf("skills write: %w", err)
	}
	full, err := r.manager.resolveWritablePath(name)
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
	if len(parts) != 1 {
		return fmt.Errorf("skills append: path must be a single file under /skills")
	}
	name, err := parseSkillFilename(parts[0])
	if err != nil {
		return fmt.Errorf("skills append: %w", err)
	}
	full, err := r.manager.resolveWritablePath(name)
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

func (r *SkillsResource) listSkillFiles() ([]vfs.Entry, error) {
	entries := r.manager.Entries()
	if len(entries) == 0 {
		return []vfs.Entry{}, nil
	}
	out := make([]vfs.Entry, 0, len(entries))
	for _, e := range entries {
		info, err := os.Stat(e.Skill.Path)
		if err != nil {
			return nil, err
		}
		out = append(out, vfs.NewFileEntry(e.Dir+".md", info.Size(), info.ModTime()))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func parseSkillFilename(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return "", fmt.Errorf("skill filename is required")
	}
	name := filename
	if strings.HasSuffix(strings.ToLower(name), ".md") {
		name = strings.TrimSuffix(name, name[len(name)-3:])
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}
	if _, err := sanitizeSkillName(name); err != nil {
		return "", err
	}
	return name, nil
}
