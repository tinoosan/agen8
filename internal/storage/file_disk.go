package storage

import (
	"context"
	"io"
	"os"
)

// DiskFileReader implements FileReader using os.Open.
type DiskFileReader struct{}

// NewDiskFileReader returns a FileReader that reads from the filesystem.
func NewDiskFileReader() *DiskFileReader {
	return &DiskFileReader{}
}

// Read reads up to maxBytes from the file at path.
func (d *DiskFileReader) Read(ctx context.Context, path string, maxBytes int) ([]byte, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, int64(maxBytes)+1))
	if err != nil {
		return nil, false, err
	}
	truncated := len(buf) > maxBytes
	if truncated {
		buf = buf[:maxBytes]
	}
	return buf, truncated, nil
}
