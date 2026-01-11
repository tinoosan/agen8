package vfs

import "time"

const (
	MountWorkspace = "workspace"
	MountTrace     = "trace"
)

// Resource is the minimal contract a “mounted thing” must implement to behave like a filesystem.
//
// A Resource does not have to be a real OS directory.
// It can be:
//   - a real directory on disk (DirResource)
//   - an MCP server (exposing tools as paths)
//   - a database (exposing tables/queries as paths)
//   - a vector store (exposing upsert/search as paths)
//
// The point is that the rest of the system (agent loop, UI, etc.) can treat all of those the same way.
//
// Path rules
//   - The path passed into a Resource method is ALWAYS a subpath relative to the mount.
//   - It should NOT start with "/".
//   - Example: if the VFS resolves "/workspace/notes.md":
//     mount = "workspace"
//     resource receives path = "notes.md"
//
// Semantics
//   - List: discover what exists at a path (like listing a directory).
//   - Read: fetch the bytes at a path.
//   - Write: replace/set the bytes at a path (creates parent dirs if applicable).
//   - Append: add bytes to the end of an existing entry, or create it if absent.
//
// For “tool-like” mounts (MCP/DB/vector store), Write/Append commonly act as “invoke” operations,
// and the output is retrieved via a subsequent Read (for example Read("last.json")) or emitted
// as events through your run event log.
type Resource interface {
	List(path string) ([]Entry, error)
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	Append(path string, data []byte) error
}

// Entry describes one item returned by Resource.List.
//
// Path conventions
//   - Entry.Path should be a FULL VFS path (starting with "/") so callers can use it directly.
//   - Example: "/workspace/file.txt" or "/mcp-finance/tools/getPrice".
//
// Optional fields
//   - Some Resources cannot reliably provide size/modification time (e.g. remote MCP servers).
//   - HasSize and HasModTime indicate whether Size and ModTime are valid.
//
// IsDir
//   - If true, the entry represents a “directory-like” container.
//   - For tool mounts, a “directory” can mean a namespace grouping, not a real folder.
type Entry struct {
	// Resource-relative path (no leading "/").
	// Examples: "notes.md", "reports/q1.md". The root itself is "".
	Path string

	// Whether the entry is directory-like.
	IsDir bool

	// Size in bytes (only valid if HasSize is true).
	Size int64

	// Last modification time (only valid if HasModTime is true).
	ModTime time.Time

	// Whether Size is valid.
	HasSize bool

	// Whether ModTime is valid.
	HasModTime bool
}

// NewDirEntry constructs a directory-like entry with unknown size and mod time.
func NewDirEntry(path string) Entry {
	return Entry{
		Path:       path,
		IsDir:      true,
		HasSize:    false,
		HasModTime: false,
	}
}

// NewFileEntry constructs a file-like entry with known size and mod time.
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
