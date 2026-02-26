package harness

import (
	"fmt"
	"os"
	"strings"
)

// HarnessIDFromMetadata returns task-level harness selection (if present).
func HarnessIDFromMetadata(metadata map[string]any) string {
	if len(metadata) == 0 {
		return ""
	}
	raw, ok := metadata["harnessId"]
	if !ok || raw == nil {
		return ""
	}
	return normalizeAdapterID(fmt.Sprint(raw))
}

// SelectHarnessID resolves harness selection with this precedence:
// task metadata -> run runtime -> env AGEN8_DEFAULT_HARNESS -> agen8-native.
func SelectHarnessID(taskMetadata map[string]any, runHarnessID string, envLookup func(string) string) string {
	if id := HarnessIDFromMetadata(taskMetadata); id != "" {
		return id
	}
	if id := normalizeAdapterID(runHarnessID); id != "" {
		return id
	}
	lookup := envLookup
	if lookup == nil {
		lookup = os.Getenv
	}
	if id := normalizeAdapterID(lookup(DefaultHarnessEnvVar)); id != "" {
		return id
	}
	return NativeAdapterID
}

func SetHarnessIDMetadata(metadata map[string]any, harnessID string) map[string]any {
	harnessID = normalizeAdapterID(harnessID)
	if metadata == nil {
		metadata = map[string]any{}
	}
	if harnessID == "" {
		delete(metadata, "harnessId")
		return metadata
	}
	metadata["harnessId"] = strings.TrimSpace(harnessID)
	return metadata
}
