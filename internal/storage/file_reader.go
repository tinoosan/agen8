package storage

import (
	"context"
)

// FileReader reads file content with a byte limit.
// Used by RPC artifact.get to avoid direct os.Open in handlers.
type FileReader interface {
	// Read reads up to maxBytes from the file at path.
	// Returns the content and whether it was truncated.
	Read(ctx context.Context, path string, maxBytes int) (content []byte, truncated bool, err error)
}
