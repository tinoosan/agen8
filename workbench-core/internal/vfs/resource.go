package vfs

import "time"

type Resource interface {
	List(path string) ([]Entry, error)
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	Append(path string, data []byte) error
}

type Entry struct {
	Path                string
	IsDir               bool
	Size                int64
	ModTime             time.Time
	HasSize, HasModTime bool
}

func NewEntry(path string, isDir bool, size int64, modTime time.Time) Entry {
	return Entry{
		Path:    path,
		IsDir:   isDir,
		Size:    size,
		ModTime: modTime,
	}
}
