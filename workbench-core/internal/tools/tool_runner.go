// Package tools provides the Tool Runner: the minimal "host executes tool" lifecycle.
//
// This file deliberately contains:
//   - NO events
//   - NO /results/index.jsonl
//   - NO tool discovery logic (that lives in the /tools resource)
//   - NO external process execution (that can be added later behind ToolInvoker)
//
// The runner's job is to:
//  1. Accept (toolId, actionId, input)
//  2. Build a types.ToolRequest
//  3. Invoke a tool implementation (ToolInvoker) from a registry
//  4. Persist outputs under /results/<callId>/...
//  5. Return a types.ToolResponse (what the agent/host "sees")
//
// Results layout written by this runner (Pattern A: callId-first)
//
// For each tool call (identified by callId), the runner writes:
//   - /results/<callId>/response.json
//   - /results/<callId>/<artifact.Path>        (zero or more files)
//
// Artifact path rules (minimal)
//   - Path must be relative (no leading "/")
//   - Path must not escape the call directory ("..", "a/../x", etc.)
//   - Any relative file name is allowed (no forced "artifacts/" prefix)
//
// Error semantics
//   - Tool failures return types.ToolResponse{Ok:false} and a nil Go error.
//   - Runner errors are only for host/IO/marshal failures (e.g. cannot write results).
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type Runner struct {
	FS           *vfs.FS
	ToolRegistry ToolRegistry
}

// ToolRegistry is the lookup mechanism for tools.
//
// The runner doesn't care how tools are implemented (builtin or external);
// it only needs an invoker.
type ToolRegistry interface {
	Get(toolId types.ToolID) (ToolInvoker, bool)
}

// ToolInvoker executes a tool call.
type ToolInvoker interface {
	Invoke(ctx context.Context, req types.ToolRequest) (ToolCallResult, error)
}

// ToolCallResult is the successful output of a tool invocation.
//
// Output is returned inline in the ToolResponse.
// Artifacts are written to /results/<callId>/<artifact.Path>.
type ToolCallResult struct {
	Output    json.RawMessage
	Artifacts []ToolArtifactWrite
}

// ToolArtifactWrite represents a file to write under the call results directory.
type ToolArtifactWrite struct {
	Path      string
	Bytes     []byte
	MediaType string
}

// MapRegistry is the minimal in-memory registry.
type MapRegistry map[types.ToolID]ToolInvoker

func (m MapRegistry) Get(id types.ToolID) (ToolInvoker, bool) {
	inv, ok := m[id]
	return inv, ok
}

// Run executes a tool call and persists its results.
//
// timeoutMs:
//   - if < 0 => invalid input (runner returns error)
//   - if == 0 => no additional timeout wrapper (caller ctx rules apply)
//   - if > 0 => runner applies context.WithTimeout(ctx, timeoutMs)
//
// This method returns a types.ToolResponse for tool-level failures (unknown tool, tool error),
// and only returns a non-nil error for runner failures (IO/marshal/host bugs).
func (r *Runner) Run(ctx context.Context, toolId types.ToolID, actionId string, input json.RawMessage, timeoutMs int) (types.ToolResponse, error) {
	if r == nil || r.FS == nil {
		return types.ToolResponse{}, fmt.Errorf("runner FS is required")
	}
	if r.ToolRegistry == nil {
		return types.ToolResponse{}, fmt.Errorf("runner ToolRegistry is required")
	}
	if toolId.String() == "" {
		return types.ToolResponse{}, fmt.Errorf("toolId is required")
	}
	if actionId == "" {
		return types.ToolResponse{}, fmt.Errorf("actionId is required")
	}
	if input == nil {
		return types.ToolResponse{}, fmt.Errorf("input is required")
	}
	if timeoutMs < 0 {
		return types.ToolResponse{}, fmt.Errorf("timeoutMs must be >= 0")
	}

	if timeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}

	callID := uuid.NewString()
	req := types.ToolRequest{
		Version:   "v1",
		CallID:    callID,
		ToolID:    toolId,
		ActionID:  actionId,
		Input:     input,
		TimeoutMs: timeoutMs,
	}
	if err := req.Validate(); err != nil {
		return types.ToolResponse{}, err
	}

	inv, ok := r.ToolRegistry.Get(toolId)
	if !ok {
		resp := types.NewToolResponseError(req, "unknown_tool", fmt.Sprintf("unknown tool %q", toolId.String()), false)
		if err := r.persist(callID, resp, nil); err != nil {
			return types.ToolResponse{}, err
		}
		return resp, nil
	}

	result, err := inv.Invoke(ctx, req)
	if err != nil {
		resp := types.NewToolResponseError(req, "tool_failed", err.Error(), false)
		if err := r.persist(callID, resp, nil); err != nil {
			return types.ToolResponse{}, err
		}
		return resp, nil
	}

	artifactRefs := make([]types.ToolArtifactRef, 0, len(result.Artifacts))
	for _, a := range result.Artifacts {
		ref := types.ToolArtifactRef{Path: a.Path, MediaType: a.MediaType}
		if err := validateArtifactWrite(a); err != nil {
			resp := types.NewToolResponseError(req, "invalid_artifact", err.Error(), false)
			if err := r.persist(callID, resp, nil); err != nil {
				return types.ToolResponse{}, err
			}
			return resp, nil
		}
		if err := ref.Validate(); err != nil {
			resp := types.NewToolResponseError(req, "invalid_artifact", err.Error(), false)
			if err := r.persist(callID, resp, nil); err != nil {
				return types.ToolResponse{}, err
			}
			return resp, nil
		}
		artifactRefs = append(artifactRefs, ref)
	}

	resp := types.NewToolResponseOK(req, result.Output, artifactRefs)
	if err := r.persist(callID, resp, result.Artifacts); err != nil {
		return types.ToolResponse{}, err
	}
	return resp, nil
}

func (r *Runner) persist(callID string, resp types.ToolResponse, artifacts []ToolArtifactWrite) error {
	// Write artifacts first so response.json references are safe.
	for _, a := range artifacts {
		if err := validateArtifactWrite(a); err != nil {
			return err
		}
		target := "/results/" + callID + "/" + path.Clean(a.Path)
		if err := r.FS.Write(target, a.Bytes); err != nil {
			return err
		}
	}

	// Write response.json last.
	b, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	target := "/results/" + callID + "/response.json"
	if err := r.FS.Write(target, b); err != nil {
		return err
	}
	return nil
}

func validateArtifactWrite(a ToolArtifactWrite) error {
	if a.Path == "" {
		return fmt.Errorf("artifact path is required")
	}
	if strings.HasPrefix(a.Path, "/") {
		return fmt.Errorf("artifact path must be relative")
	}

	// Reject any explicit parent segments, even if they would clean away.
	for _, seg := range strings.Split(a.Path, "/") {
		if seg == ".." {
			return fmt.Errorf("artifact path escapes results directory")
		}
	}

	clean := path.Clean(a.Path)
	if clean == "." {
		return fmt.Errorf("artifact path is invalid")
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("artifact path escapes results directory")
	}
	if a.MediaType == "" {
		return fmt.Errorf("artifact mediaType is required")
	}
	return nil
}
