package agent

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/tinoosan/workbench-core/internal/jsonutil"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// HostOpExecutor is a tiny "host primitive" dispatcher for demos/tests.
//
// This is not the final host API; it is a concrete reference for the agent-facing
// request/response flow:
//   - fs.list/fs.read/fs.write/fs.append are always available
//   - tool.run executes via tools.Runner and returns a ToolResponse
type HostOpExecutor struct {
	FS     *vfs.FS
	Runner *tools.Runner

	DefaultMaxBytes int

	// MaxReadBytes caps fs.read payload size returned to the model.
	//
	// This protects the model context window and cost from accidental "read the whole file"
	// requests (e.g. reading large HTML pages, logs, or binary blobs).
	//
	// If zero, no explicit cap is applied beyond DefaultMaxBytes / req.MaxBytes behavior.
	MaxReadBytes int
}

func (x *HostOpExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if x == nil || x.FS == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing FS"}
	}
	if err := req.Validate(); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}

	switch req.Op {
	case types.HostOpFSList:
		entries, err := x.FS.List(req.Path)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Path)
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, Entries: out}

	case types.HostOpFSRead:
		b, err := x.FS.Read(req.Path)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		maxBytes := req.MaxBytes
		if maxBytes == 0 {
			maxBytes = x.DefaultMaxBytes
		}
		if maxBytes <= 0 {
			maxBytes = 4096
		}
		if x.MaxReadBytes > 0 && maxBytes > x.MaxReadBytes {
			maxBytes = x.MaxReadBytes
		}
		text, b64, truncated := encodeReadPayload(b, maxBytes)
		return types.HostOpResponse{
			Op:        req.Op,
			Ok:        true,
			BytesLen:  len(b),
			Text:      text,
			BytesB64:  b64,
			Truncated: truncated,
		}

	case types.HostOpFSWrite:
		if err := x.FS.Write(req.Path, []byte(req.Text)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpFSAppend:
		if err := x.FS.Append(req.Path, []byte(req.Text)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case types.HostOpToolRun:
		if x.Runner == nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing Runner"}
		}
		resp, err := x.Runner.Run(ctx, req.ToolID, req.ActionID, req.Input, req.TimeoutMs)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, ToolResponse: &resp}

	default:
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
}

// PrettyJSON is a small helper for demos/logging.
func PrettyJSON(v any) string {
	b, err := jsonutil.MarshalPretty(v)
	if err != nil {
		return "<json marshal error: " + err.Error() + ">"
	}
	return string(b)
}

func encodeReadPayload(b []byte, maxBytes int) (text string, bytesB64 string, truncated bool) {
	if maxBytes <= 0 {
		maxBytes = len(b)
	}
	n := len(b)
	if n > maxBytes {
		n = maxBytes
		truncated = true
	}
	head := b[:n]

	// Prefer returning text when valid UTF-8.
	// If we truncated, try trimming bytes until the prefix is valid UTF-8.
	for len(head) > 0 && !utf8.Valid(head) {
		head = head[:len(head)-1]
	}
	if len(head) > 0 && utf8.Valid(head) {
		return string(head), "", truncated
	}

	// Binary or non-UTF8: return base64 so the contract is lossless.
	return "", base64.StdEncoding.EncodeToString(b[:n]), truncated
}

func AgentSay(logf func(string, ...any), exec func(types.HostOpRequest) types.HostOpResponse, req types.HostOpRequest) types.HostOpResponse {
	logf("agent -> host:\n%s", PrettyJSON(req))
	resp := exec(req)
	// Avoid dumping huge raw bytes; HostOpResponse may contain truncated text or base64.
	logf("host -> agent:\n%s", strings.TrimSpace(PrettyJSON(resp)))
	return resp
}
