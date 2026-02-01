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

	// MountTools is the mount name for tool discovery and manifests.
	// fs.List("/tools") returns tool IDs; fs.Read("/tools/<id>") returns JSON manifest bytes.
	MountTools = "tools"

	// MountResults is the mount name for tool call outputs.
	// Tool results are stored under /results/<callId>/response.json.
	MountResults = "results"

	// MountInbox is the mount name for run-scoped task inboxes.
	// External inputs can drop tasks/messages here; the agent polls and consumes them.
	MountInbox = "inbox"

	// MountOutbox is the mount name for run-scoped task results.
	// Agents write TaskResult envelopes and audit messages here for the host to collect.
	MountOutbox = "outbox"

	// MountPlan is the mount name for run-scoped planning workspace.
	MountPlan = "plan"

	// MountMemory is the mount name for shared agent memory.
	// The host may inject /memory/memory.md into the system prompt,
	// and ingest /memory/update.md when the agent chooses to write.
	//
	// Note: a future multi-agent system will likely introduce a shared, global
	// history mount for immutable provenance across runs/agents (distinct from /memory).
	MountMemory = "memory"

	// MountProfile is the mount name for user-scoped profile memory.
	//
	// Profile is global across runs and sessions and is intended for durable user facts
	// and preferences (e.g. timezone, writing style, birthday). It is distinct from
	// shared /memory, which is long-term agent memory across runs.
	MountProfile = "profile"

	// MountHistory is the mount name for session-scoped history.
	//
	// History is an immutable, append-only log of raw interactions between users,
	// agents, and the environment. It is intended as a verifiable source of truth
	// for provenance, debugging, and compliance.
	//
	// In the current implementation, history is session-scoped and stored in SQLite.
	//
	// In a future multi-agent system, you may add a shared global history mount
	// alongside per-session histories.
	MountHistory = "history"

	// MountTrace exposes an agent-specific VFS directory for trace host ops.
	// It is intentionally separate from /workspace so trace context can be kept apart.
	// Files written here are raw key/value blobs; the trace host op interacts with /trace/<key>.
	MountTrace = "trace"
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
