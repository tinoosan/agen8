package types

import "encoding/json"

// JSONIndent is the stable indentation style used for human-readable JSON artifacts.
const JSONIndent = "  "

// MarshalPretty marshals v as human-readable JSON with a stable indentation style.
func MarshalPretty(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", JSONIndent)
}
