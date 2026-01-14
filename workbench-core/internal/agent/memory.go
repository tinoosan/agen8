package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/fsutil"
)

// Persistent memory (across runs)
//
// The agent loop itself is stateless between runs unless we explicitly persist and
// re-inject context. This file implements the simplest possible persistence:
//
//   - host reads a single markdown file from disk at startup
//   - host prepends that text into the model's system prompt as "Persistent Memory"
//   - after each run/turn, host ingests "/workspace/memory_update.md" (written by the agent)
//     and appends it to the persistent memory file
//
// This keeps "memory" explicit and inspectable, and avoids introducing new host primitives.

// DefaultPersistentMemoryPath returns the on-disk path used for cross-run memory.
//
// This path is an implementation detail; the agent interacts via VFS (/workspace/memory_update.md).
func DefaultPersistentMemoryPath() string {
	return fsutil.GetAgentMemoryPath(config.DataDir)
}

// LoadPersistentMemory reads persistent memory from disk.
//
// If the file does not exist, it returns an empty string and nil error.
func LoadPersistentMemory(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// AppendPersistentMemory appends a short update to the persistent memory file.
//
// Updates are separated by a timestamped divider for easy debugging.
// If update is empty/whitespace, this is a no-op.
func AppendPersistentMemory(path string, update string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}
	update = strings.TrimSpace(update)
	if update == "" {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	divider := "\n\n---\n" + time.Now().UTC().Format(time.RFC3339Nano) + "\n\n"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(divider + update + "\n"); err != nil {
		return err
	}
	return nil
}

// BuildSystemPromptWithMemory appends a "Persistent Memory" block to a system prompt.
//
// If memory is empty, basePrompt is returned unchanged.
func BuildSystemPromptWithMemory(basePrompt, memory string) string {
	basePrompt = strings.TrimSpace(basePrompt)
	memory = strings.TrimSpace(memory)
	if memory == "" {
		return basePrompt
	}
	return strings.TrimSpace(basePrompt + "\n\n" +
		"## Persistent Memory (from previous sessions)\n\n" +
		memory + "\n")
}
