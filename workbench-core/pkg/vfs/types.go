package vfs

import "time"

const (
	// MountScratch is the mount name for an agent's scratchpad workspace.
	// Paths under /scratch are run-scoped and intended for ephemeral notes/actions.
	MountScratch = "scratch"

	// MountProject is the mount name for the user's primary project directory.
	//
	// /project maps to the OS directory the user launched Workbench from (or a flag override).
	// It is intended for "real project files" so the agent can operate on them
	// while still keeping /scratch as a run-scoped scratch area.
	MountProject = "project"

	// MountLog is the mount name for the run's event log.
	// The agent can poll /log/events.since/<offset> for new events.
	MountLog = "log"

	// MountSkills exposes the user-defined skill directories under /skills.
	MountSkills = "skills"

	// MountTools is the mount name for tool discovery and manifests.
	// fs.List("/tools") returns tool IDs; fs.Read("/tools/<id>") returns JSON manifest bytes.
	MountTools = "tools"

	// MountResults is the mount name for tool call outputs.
	// Tool results are stored under /results/<callId>/response.json.
	MountResults = "results"

	// MountMemory is the mount name for run-scoped agent memory.
	// The host may inject /memory/memory.md into the system prompt,
	// and ingest /memory/update.md after each turn.
	//
	// Note: a future multi-agent system will likely introduce a shared, global
	// history mount for immutable provenance across runs/agents (distinct from /memory).
	MountMemory = "memory"

	// MountProfile is the mount name for user-scoped profile memory.
	//
	// Profile is global across runs and sessions and is intended for durable user facts
	// and preferences (e.g. timezone, writing style, birthday). It is distinct from
	// run-scoped /memory, which is working memory for the current run.
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
	// It is intentionally separate from /scratch so trace context can be kept apart.
	// Files written here are raw key/value blobs; the trace host op interacts with /trace/<key>.
	MountTrace = "trace"
)

// Resource is the minimal contract a “mounted thing” must implement to behave like a filesystem.
type Resource interface {
	List(path string) ([]Entry, error)
	Read(path string) ([]byte, error)
	Write(path string, data []byte) error
	Append(path string, data []byte) error
}

// Entry describes one item returned by Resource.List.
type Entry struct {
	Path     string
	IsDir    bool
	Size     int64
	ModTime  time.Time
	HasSize  bool
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
