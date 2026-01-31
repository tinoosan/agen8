package store

import "context"

// MemoryStore is the host-side storage interface backing the virtual VFS mount "/memory".
// In the daily-memory design, the store provides access to a directory of memory files:
//   - /memory/MEMORY.MD                 (master instructions, read-only)
//   - /memory/YYYY-MM-DD-memory.md      (daily files; only today is writable)
type MemoryStore interface {
	// BaseDir returns the absolute directory on disk where memory files live.
	BaseDir() string

	// GetMemory returns the contents of the memory file for the provided date (UTC).
	GetMemory(ctx context.Context, date string) (string, error)

	// WriteMemory replaces the memory file for the provided date.
	WriteMemory(ctx context.Context, date string, text string) error

	// AppendMemory appends to the memory file for the provided date.
	AppendMemory(ctx context.Context, date string, text string) error

	// ListMemoryFiles returns the filenames present under the memory dir.
	ListMemoryFiles(ctx context.Context) ([]string, error)
}

// PlanFileStore exposes the minimal API needed to persist a separate plan file
// alongside the standard memory staging files.
type PlanFileStore interface {
	GetPlan(ctx context.Context) (string, error)
	SetPlan(ctx context.Context, text string) error
}

const PlanFileName = "plan.md"
