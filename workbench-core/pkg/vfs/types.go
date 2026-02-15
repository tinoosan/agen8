package vfs

import (
	"context"
	"time"

	"github.com/tinoosan/workbench-core/pkg/types"
)

const (
	// MountWorkspace is the mount name for an agent's workspace.
	// Paths under /workspace are run-scoped and intended for ephemeral notes/actions.
	MountWorkspace = "workspace"

	// MountProject is the mount name for the user's primary project directory.
	//
	// /project maps to the OS directory the user launched Workbench from (or a flag override).
	// It is intended for "real project files" so the agent can operate on them
	// while still keeping /workspace as a run-scoped workspace area.
	MountProject = "project"

	// MountLog is the mount name for the run's event log.
	// The agent can poll /log/events.since/<offset> for new events.
	MountLog = "log"

	// MountSkills exposes the user-defined skill files under /skills.
	MountSkills = "skills"

	// MountInbox is a compatibility mount for legacy workflows.
	// Task routing is DB-backed; /inbox is not used for task transport.
	MountInbox = "inbox"

	// MountOutbox is a compatibility mount for legacy workflows.
	// Task completion is persisted in SQLite; /outbox is not used for task transport.
	MountOutbox = "outbox"

	// MountPlan is the mount name for run-scoped planning workspace.
	MountPlan = "plan"

	// MountMemory is the mount name for shared agent memory.
	// The host may inject today's /memory/<YYYY-MM-DD>-memory.md into the system prompt.
	//
	// Note: a future multi-agent system will likely introduce a shared, global
	// history mount for immutable provenance across runs/agents (distinct from /memory).
	MountMemory = "memory"

	// MountSubagents exposes the parent run's subagent run directories under /subagents.
	// Only present for top-level runs; child runs live under parent's subagents dir on disk.
	MountSubagents = "subagents"
)

// Resource is the minimal contract a “mounted thing” must implement to behave like a filesystem.
type Resource interface {
	// SupportsNestedList returns true if List() supports non-root subpaths.
	SupportsNestedList() bool
	List(path string) ([]Entry, error)
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	Append(path string, data []byte) error
}

// Searchable is an optional interface for resources that can perform semantic or indexed search.
type Searchable interface {
	Search(ctx context.Context, path string, query string, limit int) ([]types.SearchResult, error)
}

// Entry describes one item returned by Resource.List.
//
// Path conventions:
//   - Empty string ("") represents the resource root and will be rewritten by the VFS
//     layer to the mount point path.
//   - Paths must be relative (no leading "/").
//   - Paths should use "/" separators (not platform-specific separators).
//
// Metadata conventions:
//   - HasSize/HasModTime should be true for real filesystem-backed entries.
//   - HasSize/HasModTime should be false for virtual/generated entries.
type Entry struct {
	Path       string
	IsDir      bool
	Size       int64
	ModTime    time.Time
	HasSize    bool
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
