package app

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const envDisableDefaultsSeed = "WORKBENCH_NO_DEFAULTS_SEED"

func maybeSeedRepoDefaults(dataDir string) error {
	if envBoolDefault(envDisableDefaultsSeed, false) {
		return nil
	}

	defaultsRoot, ok := findRepoDefaultsRoot()
	if !ok {
		return nil
	}

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("prepare data dir: %w", err)
	}

	if err := seedDir(filepath.Join(defaultsRoot, "roles"), filepath.Join(dataDir, "roles")); err != nil {
		return fmt.Errorf("seed roles: %w", err)
	}
	if err := seedDir(filepath.Join(defaultsRoot, "skills"), filepath.Join(dataDir, "skills")); err != nil {
		return fmt.Errorf("seed skills: %w", err)
	}
	return nil
}

func findRepoDefaultsRoot() (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := cwd
	for {
		candidates := []string{
			filepath.Join(dir, "defaults"),
			filepath.Join(dir, "workbench-core", "defaults"),
		}
		for _, candidate := range candidates {
			if isDir(filepath.Join(candidate, "roles")) || isDir(filepath.Join(candidate, "skills")) {
				return candidate, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func seedDir(srcDir, dstDir string) error {
	if !isDir(srcDir) {
		return nil
	}
	if err := os.RemoveAll(dstDir); err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Refuse traversal even though Rel+WalkDir should keep us inside.
		if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return fmt.Errorf("invalid defaults path %q", rel)
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported in defaults: %s", path)
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src, dst string, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func envBoolDefault(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
