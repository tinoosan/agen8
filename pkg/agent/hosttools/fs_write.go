package hosttools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/pkg/checksumutil"
	llmtypes "github.com/tinoosan/agen8/pkg/llm/types"
	"github.com/tinoosan/agen8/pkg/types"
)

// FSWriteTool writes a file in the VFS.
type FSWriteTool struct{}

func (t *FSWriteTool) Definition() llmtypes.Tool {
	return fsTool(
		"fs_write",
		"[DIRECT] Write/create a file at a VFS path. Typical target: /project/... Optional safety controls: verify/checksum/atomic/sync and expected checksum verification.",
		map[string]any{
			"path":             map[string]any{"type": "string", "description": "VFS path to write"},
			"text":             map[string]any{"type": "string", "description": "File contents to write"},
			"mode":             map[string]any{"type": "string", "enum": []string{"w", "a"}, "description": "Write mode: w=overwrite/create (default), a=append/create."},
			"verify":           map[string]any{"type": "boolean", "description": "Read back and compare after writing."},
			"checksum":         map[string]any{"type": "string", "enum": checksumutil.SupportedAlgorithms(), "description": "Optional checksum algorithm for the written content."},
			"checksumExpected": map[string]any{"type": "string", "description": "Optional expected checksum hex digest for the selected algorithm. Fails write when mismatched."},
			// Compatibility aliases for callers that pass explicit digest fields.
			"checksumMd5":    map[string]any{"type": "string", "description": "Compatibility alias for checksum='md5' with expected digest value."},
			"checksumSha1":   map[string]any{"type": "string", "description": "Compatibility alias for checksum='sha1' with expected digest value."},
			"checksumSha256": map[string]any{"type": "string", "description": "Compatibility alias for checksum='sha256' with expected digest value."},
			"atomic":         map[string]any{"type": "boolean", "description": "Request atomic write semantics (best-effort by mount)."},
			"sync":           map[string]any{"type": "boolean", "description": "Request fsync durability semantics (best-effort by mount)."},
		},
		[]any{"path", "text"},
	)
}

func (t *FSWriteTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
	var payload struct {
		Path             string `json:"path"`
		Text             string `json:"text"`
		Mode             string `json:"mode"`
		Verify           bool   `json:"verify"`
		Checksum         string `json:"checksum"`
		ChecksumExpected string `json:"checksumExpected"`
		ChecksumMD5      string `json:"checksumMd5"`
		ChecksumSHA1     string `json:"checksumSha1"`
		ChecksumSHA256   string `json:"checksumSha256"`
		Atomic           bool   `json:"atomic"`
		Sync             bool   `json:"sync"`
	}
	dec := json.NewDecoder(bytes.NewReader(args))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&payload); err != nil {
		return types.HostOpRequest{}, err
	}
	checksum, expected, err := normalizeChecksumArgs(payload.Checksum, payload.ChecksumExpected, payload.ChecksumMD5, payload.ChecksumSHA1, payload.ChecksumSHA256)
	if err != nil {
		return types.HostOpRequest{}, err
	}
	return types.HostOpRequest{
		Op:               types.HostOpFSWrite,
		Path:             resolveVFSPath(payload.Path),
		Text:             payload.Text,
		Mode:             strings.TrimSpace(payload.Mode),
		Verify:           payload.Verify,
		Checksum:         checksum,
		ChecksumExpected: expected,
		Atomic:           payload.Atomic,
		Sync:             payload.Sync,
	}, nil
}

func normalizeChecksumArgs(checksum, expected, checksumMD5, checksumSHA1, checksumSHA256 string) (string, string, error) {
	checksum = checksumutil.NormalizeAlgorithm(checksum)
	expected = strings.TrimSpace(expected)
	legacy := map[string]string{}
	if v := strings.TrimSpace(checksumMD5); v != "" {
		legacy["md5"] = v
	}
	if v := strings.TrimSpace(checksumSHA1); v != "" {
		legacy["sha1"] = v
	}
	if v := strings.TrimSpace(checksumSHA256); v != "" {
		legacy["sha256"] = v
	}
	if len(legacy) > 1 {
		return "", "", fmt.Errorf("only one of checksumMd5/checksumSha1/checksumSha256 may be set")
	}
	for algo, value := range legacy {
		if checksum != "" && checksum != algo {
			return "", "", fmt.Errorf("checksum %q conflicts with %s alias", checksum, aliasNameForAlgo(algo))
		}
		checksum = algo
		if expected != "" && !strings.EqualFold(expected, value) {
			return "", "", fmt.Errorf("checksumExpected conflicts with %s alias", aliasNameForAlgo(algo))
		}
		expected = value
	}
	return checksum, expected, nil
}

func aliasNameForAlgo(algo string) string {
	switch strings.ToLower(strings.TrimSpace(algo)) {
	case "md5":
		return "checksumMd5"
	case "sha1":
		return "checksumSha1"
	case "sha256":
		return "checksumSha256"
	default:
		return "checksumAlias"
	}
}
