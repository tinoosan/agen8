// Package jsonutil provides small helpers for stable JSON encoding.
//
// The workbench codebase writes a few JSON artifacts to disk or to virtual stores:
//   - run.json (run state)
//   - /results/<callId>/response.json (tool responses)
//   - /workspace/context_manifest.json (context audit)
//
// Keeping a single "pretty JSON" helper reduces drift in indentation and makes
// outputs easier to inspect while the system is still evolving.
package jsonutil

import "encoding/json"

const indent = "  "

// MarshalPretty marshals v as human-readable JSON with a stable indentation style.
func MarshalPretty(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", indent)
}
