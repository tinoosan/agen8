package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

	ReadPreviewLimit int
}

func (x *HostOpExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if x == nil || x.FS == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing FS"}
	}

	switch req.Op {
	case "fs.list":
		entries, err := x.FS.List(req.Path)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		out := make([]string, 0, len(entries))
		for _, e := range entries {
			out = append(out, e.Path)
		}
		return types.HostOpResponse{Op: req.Op, Ok: true, Entries: out}

	case "fs.read":
		b, err := x.FS.Read(req.Path)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		limit := x.ReadPreviewLimit
		if limit <= 0 {
			limit = 600
		}
		return types.HostOpResponse{
			Op:       req.Op,
			Ok:       true,
			BytesLen: len(b),
			Text:     PreviewText(b, limit),
		}

	case "fs.write":
		if err := x.FS.Write(req.Path, []byte(req.Text)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case "fs.append":
		if err := x.FS.Append(req.Path, []byte(req.Text)); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: req.Op, Ok: true}

	case "tool.run":
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
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "<json marshal error: " + err.Error() + ">"
	}
	return string(b)
}

// PreviewText returns a short string preview for logs.
func PreviewText(b []byte, limit int) string {
	if limit <= 0 || len(b) <= limit {
		return string(b)
	}
	return string(b[:limit]) + "\n... (truncated)"
}

func AgentSay(logf func(string, ...any), exec func(types.HostOpRequest) types.HostOpResponse, req types.HostOpRequest) types.HostOpResponse {
	logf("agent -> host:\n%s", PrettyJSON(req))
	resp := exec(req)
	// Avoid dumping huge raw bytes; HostOpResponse already contains a preview.
	logf("host -> agent:\n%s", strings.TrimSpace(PrettyJSON(resp)))
	return resp
}
