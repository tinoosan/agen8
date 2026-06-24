package infra

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	filedomain "github.com/tinoosan/agen8/internal/services/file/domain/file"
)

func TestLocalRepositoryRejectsSymlinkPathForFileOperations(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	root := t.TempDir()
	outside := t.TempDir()
	repo := NewLocalRepository()

	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "source.txt"), []byte("safe"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "escape")))
	linkedPath := filepath.Join(root, "escape")

	_, err := repo.Stat(ctx, filedomain.Reference{Path: linkedPath})
	require.Error(t, err)
	_, err = repo.ListDir(ctx, filedomain.Reference{Path: linkedPath})
	require.Error(t, err)
	_, err = repo.Read(ctx, filedomain.Reference{Path: filepath.Join(linkedPath, "secret.txt")}, 1)
	require.Error(t, err)
	err = repo.CreateDir(ctx, filedomain.Reference{Path: filepath.Join(linkedPath, "new")})
	require.Error(t, err)
	err = repo.CreateFile(ctx, filedomain.Reference{Path: filepath.Join(linkedPath, "touch.txt")})
	require.Error(t, err)
	err = repo.Move(ctx, filedomain.Reference{Path: filepath.Join(root, "source.txt")}, filedomain.Reference{Path: filepath.Join(linkedPath, "moved.txt")})
	require.Error(t, err)
	err = repo.Copy(ctx, filedomain.Reference{Path: filepath.Join(root, "source.txt")}, filedomain.Reference{Path: filepath.Join(linkedPath, "copied.txt")})
	require.Error(t, err)
	err = repo.Delete(ctx, filedomain.Reference{Path: filepath.Join(linkedPath, "secret.txt")})
	require.Error(t, err)
	err = repo.WriteFile(ctx, filedomain.Reference{Path: filepath.Join(linkedPath, "upload.txt")}, []byte("payload"))
	require.Error(t, err)
	err = repo.WriteFileReader(ctx, filedomain.Reference{Path: filepath.Join(linkedPath, "upload-stream.txt")}, bytes.NewBufferString("payload"))
	require.Error(t, err)
}

func TestLocalRepositoryCopyRejectsNestedSymlinkInSourceTree(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	root := t.TempDir()
	repo := NewLocalRepository()

	src := filepath.Join(root, "source")
	reqTarget := filepath.Join(root, "linked-target")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.MkdirAll(reqTarget, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "safe.txt"), []byte("safe"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(reqTarget, "outside.txt"), []byte("outside"), 0o644))
	require.NoError(t, os.Symlink(reqTarget, filepath.Join(src, "escape")))

	err := repo.Copy(ctx, filedomain.Reference{Path: src}, filedomain.Reference{Path: filepath.Join(root, "dest")})
	require.Error(t, err)
}

func TestLocalRepositoryWriteFileReaderWritesAbsolutePathInIntendedTree(t *testing.T) {
	ctx := context.Background()

	root := t.TempDir()
	cwd := t.TempDir()
	t.Chdir(cwd)

	target := filepath.Join(root, "nested", "stream.txt")
	repo := NewLocalRepository()

	err := repo.WriteFileReader(ctx, filedomain.Reference{Path: target}, bytes.NewBufferString("payload"))
	require.NoError(t, err)

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "payload", string(got))
	require.NoFileExists(t, filepath.Join(cwd, "nested", "stream.txt"))
}

func TestEnsureNoSymlinkPathAllowsRegularPathsBeforeMissingSegment(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "safe"), 0o755))
	require.NoError(t, ensureNoSymlinkPath(filepath.Join(root, "safe", "new.txt")))
	require.NoError(t, ensureNoSymlinkPath(filepath.Join(root, "safe", "missing", "nested", "path")))
}

func TestEnsureNoSymlinkPathPreservesAbsoluteRootCompatibility(t *testing.T) {
	t.Parallel()

	// Some host systems keep common top-level mount points as symlinks.
	require.NoError(t, ensureNoSymlinkPath("/var/tmp"))
}
