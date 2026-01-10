package vfs

import "time"

type Resource interface {
	List(path string) ([]Entry, error)
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	Append(path string, data []byte) error
}

type Entry struct {
	// Full path of the entry e.g /workspace/file.txt
	Path string
	// Whether the entry is a directory
	IsDir bool
	// Size of the entry in bytes
	Size int64
	// Modification time of the entry
	ModTime time.Time
	// Whether the entry has a size
	HasSize bool
	// Whether the entry has a modification time
	HasModTime bool
}

func NewDirEntry(path string) Entry {
	return Entry{
		Path:       path,
		IsDir:      true,
		HasSize:    false,
		HasModTime: false,
	}
}

func NewFileEntry(path string, size int64, modTime time.Time) Entry {
	return Entry{
		Path:       path,
		IsDir:      false,
		Size:       size,
		ModTime:    modTime,
		HasSize:    true,
		HasModTime: true,
	}
}
