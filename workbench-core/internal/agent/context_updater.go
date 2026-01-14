package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// ContextUpdater keeps the model's bounded context synchronized with persistent
// context sources (like /memory) and runtime sources (like /trace).
//
// This is the "v0" implementation:
//   - it refreshes context once per model step
//   - it tracks a byte offset into /trace so it only loads new events
//   - it injects small bounded excerpts into the system prompt
//   - it writes a context manifest for transparency/debugging
type ContextUpdater struct {
	FS *vfs.FS

	// TraceOffset is the current byte offset into /trace/events.jsonl used by
	// events.since/<offset>.
	TraceOffset int64

	// MaxMemoryBytes caps how many bytes from /memory/memory.md are injected.
	// If zero, a default is used.
	MaxMemoryBytes int

	// MaxTraceBytes caps how many bytes from /trace/events.since/<offset> are injected.
	// If zero, a default is used.
	MaxTraceBytes int

	// ManifestPath is the VFS path where the updater writes its last manifest.
	// If empty, no manifest is written.
	ManifestPath string

	// Emit is an optional hook for recording updater actions (events/telemetry).
	Emit func(eventType, message string, data map[string]string)
}

type ContextManifest struct {
	UpdatedAt string `json:"updatedAt"`

	Step int `json:"step"`

	Memory struct {
		Path          string `json:"path"`
		BytesTotal    int    `json:"bytesTotal"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
	} `json:"memory"`

	Trace struct {
		Path          string `json:"path"`
		OffsetBefore  int64  `json:"offsetBefore"`
		OffsetAfter   int64  `json:"offsetAfter"`
		BytesRead     int    `json:"bytesRead"`
		BytesIncluded int    `json:"bytesIncluded"`
		Truncated     bool   `json:"truncated"`
	} `json:"trace"`
}

// BuildSystemPrompt returns a base system prompt augmented with bounded context excerpts,
// and a manifest describing what was loaded.
func (u *ContextUpdater) BuildSystemPrompt(ctx context.Context, basePrompt string, step int) (string, ContextManifest, error) {
	if u == nil || u.FS == nil {
		return "", ContextManifest{}, fmt.Errorf("context updater FS is required")
	}

	maxMem := u.MaxMemoryBytes
	if maxMem == 0 {
		maxMem = 8 * 1024
	}
	maxTrace := u.MaxTraceBytes
	if maxTrace == 0 {
		maxTrace = 8 * 1024
	}

	manifest := ContextManifest{
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Step:      step,
	}

	// Memory excerpt (tail-biased).
	memPath := "/memory/memory.md"
	memBytes, memErr := u.FS.Read(memPath)
	if memErr != nil {
		memBytes = []byte{}
	}
	memIncl, memTrunc := tailUTF8(memBytes, maxMem)
	manifest.Memory.Path = memPath
	manifest.Memory.BytesTotal = len(memBytes)
	manifest.Memory.BytesIncluded = len(memIncl)
	manifest.Memory.Truncated = memTrunc

	// Trace excerpt (incremental since offset).
	traceSincePath := "/trace/events.since/" + strconv.FormatInt(u.TraceOffset, 10)
	traceBytes, traceErr := u.FS.Read(traceSincePath)
	if traceErr != nil {
		traceBytes = []byte{}
	}
	traceRead := len(traceBytes)
	// Advance offset by full bytes read (even if we later truncate what we inject).
	offsetBefore := u.TraceOffset
	u.TraceOffset = u.TraceOffset + int64(traceRead)
	offsetAfter := u.TraceOffset

	traceIncl, traceTrunc := headUTF8(traceBytes, maxTrace)
	manifest.Trace.Path = traceSincePath
	manifest.Trace.OffsetBefore = offsetBefore
	manifest.Trace.OffsetAfter = offsetAfter
	manifest.Trace.BytesRead = traceRead
	manifest.Trace.BytesIncluded = len(traceIncl)
	manifest.Trace.Truncated = traceTrunc

	system := strings.TrimSpace(basePrompt)
	if len(memIncl) > 0 {
		system = system + "\n\n" + "## Persistent Memory (/memory/memory.md)\n\n" + string(memIncl) + "\n"
	}
	if len(traceIncl) > 0 {
		system = system + "\n\n" + "## Recent Trace Updates (/trace/events.since)\n\n" + string(traceIncl) + "\n"
	}
	system = strings.TrimSpace(system)

	if u.ManifestPath != "" {
		b, err := json.MarshalIndent(manifest, "", "  ")
		if err == nil {
			_ = u.FS.Write(u.ManifestPath, b)
		}
	}

	if u.Emit != nil {
		u.Emit("context.update", "Context updated", map[string]string{
			"step":             strconv.Itoa(step),
			"memoryBytes":      strconv.Itoa(manifest.Memory.BytesIncluded),
			"traceBytes":       strconv.Itoa(manifest.Trace.BytesIncluded),
			"traceOffsetAfter": strconv.FormatInt(manifest.Trace.OffsetAfter, 10),
		})
	}

	_ = ctx
	return system, manifest, nil
}

func headUTF8(b []byte, max int) ([]byte, bool) {
	if max <= 0 || len(b) <= max {
		return b, false
	}
	out := b[:max]
	// First try trimming from the end (most common when the cut splits a rune).
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[:len(out)-1]
	}
	if len(out) > 0 && utf8.Valid(out) {
		return out, true
	}

	// If the prefix begins with invalid bytes, trimming the end won't help.
	// Fall back to trimming from the start until the remaining slice is valid UTF-8.
	out = b[:max]
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[1:]
	}
	return out, true
}

func tailUTF8(b []byte, max int) ([]byte, bool) {
	if max <= 0 || len(b) <= max {
		return b, false
	}
	start := len(b) - max
	out := b[start:]
	// First try trimming from the start (most common when the cut splits a rune).
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[1:]
	}
	if len(out) > 0 && utf8.Valid(out) {
		return out, true
	}

	// If the suffix ends with invalid bytes, trimming the start won't help.
	// Fall back to trimming from the end until valid.
	out = b[start:]
	for len(out) > 0 && !utf8.Valid(out) {
		out = out[:len(out)-1]
	}
	return out, true
}

// Ensure ContextUpdater is only used with the agent loop types.
var _ = types.HostOpRequest{}
