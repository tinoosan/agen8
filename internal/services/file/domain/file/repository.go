package file

import (
	"context"
	"time"

	"github.com/tinoosan/agen8/internal/core/types"
)

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
