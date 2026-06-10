package file

import (
	"context"
	"errors"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
)

// ErrGitBaselineNotPermitted means a location has not been granted the git-diff
// capability. A sentinel (not a transport failure) so the file service degrades
// to "diff is off for this location" rather than surfacing an error.
var ErrGitBaselineNotPermitted = errors.New("git baseline is not enabled for this location")

type Reference struct {
	LocationID types.LocationID
	Path       string
}

type Info struct {
	IsDir      bool
	Size       int64
	ModifiedAt time.Time
}

type Entry struct {
	Name string
	Info Info
}

type Content struct {
	Bytes     []byte
	Truncated bool
	FileSize  int64
}

// GitBaseline is the committed (git HEAD) version of a file, produced by a
// transport that can run git where the repo lives. Tracked=false means the
// file is new/untracked or its directory is not a git repo — a normal answer,
// not an error. Bytes is the raw committed content; UTF-8 validity and
// truncation are decided by the caller.
type GitBaseline struct {
	Tracked bool
	Bytes   []byte
}

type Reader interface {
	Stat(ctx context.Context, ref Reference) (Info, error)
	ListDir(ctx context.Context, ref Reference) ([]Entry, error)
	Read(ctx context.Context, ref Reference, maxBytes int64) (Content, error)
}

type Writer interface {
	CreateDir(ctx context.Context, ref Reference) error
	CreateFile(ctx context.Context, ref Reference) error
	Move(ctx context.Context, source Reference, destination Reference) error
	Copy(ctx context.Context, source Reference, destination Reference) error
	Delete(ctx context.Context, ref Reference) error
	WriteFile(ctx context.Context, ref Reference, contents []byte) error
}

type Repository interface {
	Reader
	Writer
}
