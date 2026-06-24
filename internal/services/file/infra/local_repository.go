package infra

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	filedomain "github.com/tinoosan/agen8/internal/services/file/domain/file"
)

type LocalRepository struct{}

func NewLocalRepository() LocalRepository {
	return LocalRepository{}
}

var _ filedomain.Repository = LocalRepository{}

func (LocalRepository) Stat(_ context.Context, ref filedomain.Reference) (filedomain.Info, error) {
	if err := ensureNoSymlinkPath(ref.Path); err != nil {
		return filedomain.Info{}, err
	}
	root, rootPath, err := localRootForPath(ref.Path)
	if err != nil {
		return filedomain.Info{}, err
	}
	defer func() {
		_ = root.Close()
	}()
	info, err := root.Stat(filepath.ToSlash(rootPath))
	if err != nil {
		return filedomain.Info{}, err
	}
	return infoFromOS(info), nil
}

func (LocalRepository) ListDir(_ context.Context, ref filedomain.Reference) ([]filedomain.Entry, error) {
	if err := ensureNoSymlinkPath(ref.Path); err != nil {
		return nil, err
	}
	root, rootPath, err := localRootForPath(ref.Path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = root.Close()
	}()
	d, err := root.Open(filepath.ToSlash(rootPath))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = d.Close()
	}()
	items, err := d.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	entries := make([]filedomain.Entry, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			return nil, fmt.Errorf("stat directory entry %s: %w", item.Name(), err)
		}
		entries = append(entries, filedomain.Entry{
			Name: item.Name(),
			Info: infoFromOS(info),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Info.IsDir != entries[j].Info.IsDir {
			return entries[i].Info.IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func (LocalRepository) Read(_ context.Context, ref filedomain.Reference, maxBytes int64) (filedomain.Content, error) {
	if err := ensureNoSymlinkPath(ref.Path); err != nil {
		return filedomain.Content{}, err
	}
	root, rootPath, err := localRootForPath(ref.Path)
	if err != nil {
		return filedomain.Content{}, err
	}
	defer func() {
		_ = root.Close()
	}()
	info, err := root.Stat(filepath.ToSlash(rootPath))
	if err != nil {
		return filedomain.Content{}, err
	}
	file, err := root.Open(filepath.ToSlash(rootPath))
	if err != nil {
		return filedomain.Content{}, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return filedomain.Content{}, err
	}
	truncated := int64(len(raw)) > maxBytes
	if truncated {
		raw = raw[:maxBytes]
	}
	return filedomain.Content{
		Bytes:     raw,
		Truncated: truncated,
		FileSize:  info.Size(),
	}, nil
}

func (LocalRepository) CreateDir(_ context.Context, ref filedomain.Reference) error {
	if err := ensureNoSymlinkPath(ref.Path); err != nil {
		return err
	}
	root, rootPath, err := localRootForPath(ref.Path)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()
	return root.MkdirAll(filepath.ToSlash(rootPath), 0o700)
}

func (LocalRepository) CreateFile(_ context.Context, ref filedomain.Reference) error {
	if err := ensureNoSymlinkPath(ref.Path); err != nil {
		return err
	}
	root, rootPath, err := localRootForPath(ref.Path)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()
	if err := root.MkdirAll(filepath.ToSlash(filepath.Dir(rootPath)), 0o700); err != nil {
		return err
	}
	file, err := root.OpenFile(filepath.ToSlash(rootPath), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

func (LocalRepository) Move(_ context.Context, source filedomain.Reference, destination filedomain.Reference) error {
	if err := ensureNoSymlinkPath(source.Path); err != nil {
		return err
	}
	if err := ensureNoSymlinkPath(destination.Path); err != nil {
		return err
	}
	root, rootPath, err := localRootForPath(destination.Path)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()
	if err := root.MkdirAll(filepath.ToSlash(filepath.Dir(rootPath)), 0o700); err != nil {
		return err
	}
	return os.Rename(source.Path, destination.Path)
}

func (LocalRepository) Copy(_ context.Context, source filedomain.Reference, destination filedomain.Reference) error {
	if err := ensureNoSymlinkPath(source.Path); err != nil {
		return err
	}
	if err := ensureNoSymlinkPath(destination.Path); err != nil {
		return err
	}
	root, rootPath, err := localRootForPath(source.Path)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()
	info, err := root.Stat(filepath.ToSlash(rootPath))
	if err != nil {
		return err
	}
	return copyPath(source.Path, destination.Path, info)
}

func (LocalRepository) Delete(_ context.Context, ref filedomain.Reference) error {
	if err := ensureNoSymlinkPath(ref.Path); err != nil {
		return err
	}
	return os.RemoveAll(ref.Path)
}

func (LocalRepository) WriteFile(_ context.Context, ref filedomain.Reference, contents []byte) error {
	if err := ensureNoSymlinkPath(ref.Path); err != nil {
		return err
	}
	root, rootPath, err := localRootForPath(ref.Path)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()
	if err := root.MkdirAll(filepath.ToSlash(filepath.Dir(rootPath)), 0o700); err != nil {
		return err
	}
	out, err := root.OpenFile(filepath.ToSlash(rootPath), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	_, err = out.Write(contents)
	return err
}

func (LocalRepository) WriteFileReader(_ context.Context, ref filedomain.Reference, contents io.Reader) error {
	if err := ensureNoSymlinkPath(ref.Path); err != nil {
		return err
	}
	root, rootPath, err := localRootForPath(ref.Path)
	if err != nil {
		return err
	}
	defer func() {
		_ = root.Close()
	}()
	if err := root.MkdirAll(filepath.ToSlash(filepath.Dir(rootPath)), 0o700); err != nil {
		return err
	}
	rootPath = filepath.ToSlash(rootPath)
	tmp, tmpPath, err := createRootTemp(root, filepath.Dir(rootPath), "."+filepath.Base(rootPath)+".tmp-")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = root.Remove(tmpPath)
		}
	}()
	if _, err := io.Copy(tmp, contents); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := root.Rename(tmpPath, rootPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func createRootTemp(root *os.Root, dir string, prefix string) (*os.File, string, error) {
	for range 100 {
		suffix, err := randomHex(8)
		if err != nil {
			return nil, "", err
		}
		name := filepath.ToSlash(filepath.Join(dir, prefix+suffix))
		file, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("create temp file in %s: too many collisions", dir)
}

func randomHex(bytesLen int) (string, error) {
	var raw = make([]byte, bytesLen)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func infoFromOS(info os.FileInfo) filedomain.Info {
	return filedomain.Info{
		IsDir:      info.IsDir(),
		Size:       info.Size(),
		ModifiedAt: info.ModTime().UTC(),
	}
}

func copyPath(src string, dst string, info os.FileInfo) error {
	if err := ensureNoSymlinkPath(src); err != nil {
		return err
	}
	if err := ensureNoSymlinkPath(dst); err != nil {
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ensureNoSymlinkPath(path); err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, rel)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o700)
			}
			entryInfo, err := entry.Info()
			if err != nil {
				return err
			}
			return copyFile(path, target, entryInfo.Mode())
		})
	}
	return copyFile(src, dst, info.Mode())
}

func copyFile(src string, dst string, mode os.FileMode) error {
	if err := ensureNoSymlinkPath(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	if err := ensureNoSymlinkPath(src); err != nil {
		return err
	}
	srcRoot, rootSrcPath, err := localRootForPath(src)
	if err != nil {
		return err
	}
	defer func() {
		_ = srcRoot.Close()
	}()
	dstRoot, rootDstPath, err := localRootForPath(dst)
	if err != nil {
		_ = srcRoot.Close()
		return err
	}
	defer func() {
		_ = dstRoot.Close()
	}()
	if err := dstRoot.MkdirAll(filepath.ToSlash(filepath.Dir(rootDstPath)), 0o700); err != nil {
		return err
	}
	in, err := srcRoot.Open(filepath.ToSlash(rootSrcPath))
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := dstRoot.OpenFile(filepath.ToSlash(rootDstPath), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm()&0o600)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func localRootForPath(path string) (*os.Root, string, error) {
	cleanPath := filepath.Clean(path)
	rootPath := filepath.VolumeName(cleanPath)
	if rootPath == "" {
		rootPath = string(filepath.Separator)
	} else {
		rootPath = filepath.Clean(rootPath + string(filepath.Separator))
	}
	relativePath, err := filepath.Rel(rootPath, cleanPath)
	if err != nil {
		return nil, "", fmt.Errorf("resolve local root path %s: %w", path, err)
	}
	relativePath = filepath.ToSlash(relativePath)
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, "", fmt.Errorf("open local root %s: %w", rootPath, err)
	}
	return root, relativePath, nil
}

func ensureNoSymlinkPath(path string) error {
	cleanPath := filepath.Clean(path)
	isAbs := filepath.IsAbs(cleanPath)
	parts := strings.Split(cleanPath, string(filepath.Separator))
	var current string

	for idx, part := range parts {
		if part == "" {
			if idx == 0 {
				current = string(filepath.Separator)
			}
			continue
		}

		if current == "" {
			current = part
		} else if current == string(filepath.Separator) {
			current = filepath.Join(current, part)
		} else {
			current = filepath.Join(current, part)
		}

		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if isAbs && idx == 1 {
			// Keep first absolute path components flexible for host-level mounts.
			// Systems like macOS can expose symlinked top-level directories (for
			// example `/var`), and we only need to enforce path hardening for
			// repository-local segments where untrusted input can redirect traversal.
			continue
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path is disallowed: %s", current)
		}
		if !info.IsDir() && idx < len(parts)-1 {
			return fmt.Errorf("path component is not a directory: %s", current)
		}
	}

	return nil
}
