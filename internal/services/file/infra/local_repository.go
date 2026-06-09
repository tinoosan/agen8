package infra

import (
	"context"
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
	info, err := os.Stat(ref.Path)
	if err != nil {
		return filedomain.Info{}, err
	}
	return infoFromOS(info), nil
}

func (LocalRepository) ListDir(_ context.Context, ref filedomain.Reference) ([]filedomain.Entry, error) {
	items, err := os.ReadDir(ref.Path)
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
	info, err := os.Stat(ref.Path)
	if err != nil {
		return filedomain.Content{}, err
	}
	file, err := os.Open(ref.Path)
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
	return os.MkdirAll(ref.Path, 0o755)
}

func (LocalRepository) CreateFile(_ context.Context, ref filedomain.Reference) error {
	if err := os.MkdirAll(filepath.Dir(ref.Path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(ref.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func (LocalRepository) Move(_ context.Context, source filedomain.Reference, destination filedomain.Reference) error {
	if err := os.MkdirAll(filepath.Dir(destination.Path), 0o755); err != nil {
		return err
	}
	return os.Rename(source.Path, destination.Path)
}

func (LocalRepository) Copy(_ context.Context, source filedomain.Reference, destination filedomain.Reference) error {
	info, err := os.Stat(source.Path)
	if err != nil {
		return err
	}
	return copyPath(source.Path, destination.Path, info)
}

func (LocalRepository) Delete(_ context.Context, ref filedomain.Reference) error {
	return os.RemoveAll(ref.Path)
}

func (LocalRepository) WriteFile(_ context.Context, ref filedomain.Reference, contents []byte) error {
	if err := os.MkdirAll(filepath.Dir(ref.Path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(ref.Path, contents, 0o644)
}

func infoFromOS(info os.FileInfo) filedomain.Info {
	return filedomain.Info{
		IsDir:      info.IsDir(),
		Size:       info.Size(),
		ModifiedAt: info.ModTime().UTC(),
	}
}

func copyPath(src string, dst string, info os.FileInfo) error {
	if info.IsDir() {
		return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			target := filepath.Join(dst, rel)
			if entry.IsDir() {
				return os.MkdirAll(target, 0o755)
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
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode.Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
