package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/config"
	"github.com/tinoosan/workbench-core/pkg/fsutil"
	"github.com/tinoosan/workbench-core/pkg/store"
	"github.com/tinoosan/workbench-core/pkg/types"
)

// CreateChildRun creates a sub-run for an orchestrator.
func CreateChildRun(cfg config.Config, sessionID, parentRunID, goal string, maxBytesForContext int) (types.Run, error) {
	return store.CreateSubRun(cfg, sessionID, parentRunID, goal, maxBytesForContext)
}

// EnqueueTask writes a Task envelope into the target run's inbox directory.
// Returns the absolute path to the task file.
func EnqueueTask(cfg config.Config, runID string, task types.Task) (string, error) {
	if err := cfg.Validate(); err != nil {
		return "", err
	}
	if strings.TrimSpace(runID) == "" {
		return "", fmt.Errorf("runID is required")
	}
	taskID := strings.TrimSpace(task.TaskID)
	if taskID == "" {
		taskID = "task-" + uuid.NewString()
		task.TaskID = taskID
	}
	if task.CreatedAt == nil {
		now := time.Now()
		task.CreatedAt = &now
	}
	inboxDir := filepath.Join(fsutil.GetRunDir(cfg.DataDir, runID), "inbox")
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return "", fmt.Errorf("create inbox dir: %w", err)
	}
	filename := taskFilename(task)
	fullPath := filepath.Join(inboxDir, filename)
	b, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal task: %w", err)
	}
	if err := os.WriteFile(fullPath, b, 0644); err != nil {
		return "", fmt.Errorf("write task: %w", err)
	}
	return fullPath, nil
}

// OutboxItem represents a parsed artifact from a run's outbox.
type OutboxItem struct {
	Path       string
	TaskResult *types.TaskResult
	Message    *types.Message
	Raw        map[string]any
}

// ReadOutbox reads and parses outbox JSON files for the given run.
func ReadOutbox(cfg config.Config, runID string) ([]OutboxItem, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("runID is required")
	}
	outboxDir := filepath.Join(fsutil.GetRunDir(cfg.DataDir, runID), "outbox")
	entries, err := os.ReadDir(outboxDir)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		paths = append(paths, filepath.Join(outboxDir, e.Name()))
	}
	sort.Strings(paths)
	items := make([]OutboxItem, 0, len(paths))
	for _, p := range paths {
		item, ok := parseOutboxItem(p)
		if ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func parseOutboxItem(path string) (OutboxItem, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return OutboxItem{}, false
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		return OutboxItem{}, false
	}
	item := OutboxItem{Path: path, Raw: raw}
	if _, ok := raw["messageId"]; ok {
		var msg types.Message
		if err := json.Unmarshal(b, &msg); err == nil {
			item.Message = &msg
			return item, true
		}
	}
	if _, ok := raw["taskId"]; ok {
		var res types.TaskResult
		if err := json.Unmarshal(b, &res); err == nil {
			item.TaskResult = &res
			return item, true
		}
	}
	return item, true
}

func taskFilename(task types.Task) string {
	priority := strings.ToUpper(strings.TrimSpace(task.Priority))
	if priority == "" {
		priority = "P2"
	}
	now := time.Now().UTC().Format("20060102T150405Z")
	return fmt.Sprintf("%s-%s-%s.json", priority, now, task.TaskID)
}
