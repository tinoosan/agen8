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
//  4. Persist outputs under /results/<callId>/... (virtual mount backed by a ResultsStore)
//  5. Return a types.ToolResponse (what the agent/host "sees")
//
// Results layout written by this runner (Pattern A: callId-first)
//
// For each tool call (identified by callId), the runner stores:
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
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/internal/jsonutil"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

// Runner executes tool calls and persists their results under /results/<callId>/.
//
// Runner is intentionally small: it is the single code path for tool execution.
// Builtins and external tools (later) both implement ToolInvoker and run through
// the same lifecycle and persistence rules.
type Runner struct {
	// Results is the backing store for the virtual "/results" mount.
	//
	// The runner persists response.json and artifact bytes into this store, and
	// the agent later reads them via fs.read("/results/<callId>/...") through the
	// mounted VirtualResultsResource.
	Results store.ResultsStore

	// ToolRegistry resolves toolId -> ToolInvoker.
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

// InvokeError is an optional structured error that a ToolInvoker can return.
//
// The runner converts InvokeError into a types.ToolResponse with the provided
// error fields:
//   - Code becomes ToolError.Code
//   - Message becomes ToolError.Message
//   - Retryable becomes ToolError.Retryable
//
// If a tool returns a non-InvokeError, the runner falls back to code "tool_failed".
//
// This small wrapper lets tools express protocol-level error codes like:
//   - "invalid_input"
//   - "timeout"
//
// without introducing a separate execution flow for builtins vs custom tools.
type InvokeError struct {
	Code      string
	Message   string
	Retryable bool
	Err       error
}

// Error returns a stable, human-readable error message.
//
// The returned message is what ends up in ToolResponse.Error.Message when the
// runner persists an invocation failure.
func (e *InvokeError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "tool invoke error"
}

func (e *InvokeError) Unwrap() error { return e.Err }

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

// Get looks up a tool invoker by toolId.
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
	if r == nil || r.Results == nil {
		return types.ToolResponse{}, fmt.Errorf("runner ResultsStore is required")
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
		code := "tool_failed"
		message := err.Error()
		retryable := false

		var invErr *InvokeError
		if errors.As(err, &invErr) && invErr != nil {
			if invErr.Code != "" {
				code = invErr.Code
			}
			if invErr.Message != "" {
				message = invErr.Message
			}
			retryable = invErr.Retryable
		}

		resp := types.NewToolResponseError(req, code, message, retryable)
		if err := r.persist(callID, resp, nil); err != nil {
			return types.ToolResponse{}, err
		}
		return resp, nil
	}

	artifactRefs := make([]types.ToolArtifactRef, 0, len(result.Artifacts))
	cleanedArtifacts := make([]ToolArtifactWrite, 0, len(result.Artifacts))
	for _, a := range result.Artifacts {
		clean, err := validateAndCleanArtifactWrite(a)
		if err != nil {
			resp := types.NewToolResponseError(req, "invalid_artifact", err.Error(), false)
			if err := r.persist(callID, resp, nil); err != nil {
				return types.ToolResponse{}, err
			}
			return resp, nil
		}
		ref := types.ToolArtifactRef{Path: clean, MediaType: a.MediaType}
		if err := ref.Validate(); err != nil {
			resp := types.NewToolResponseError(req, "invalid_artifact", err.Error(), false)
			if err := r.persist(callID, resp, nil); err != nil {
				return types.ToolResponse{}, err
			}
			return resp, nil
		}
		artifactRefs = append(artifactRefs, ref)
		cleanedArtifacts = append(cleanedArtifacts, ToolArtifactWrite{
			Path:      clean,
			Bytes:     a.Bytes,
			MediaType: a.MediaType,
		})
	}

	resp := types.NewToolResponseOK(req, result.Output, artifactRefs)
	if err := r.persist(callID, resp, cleanedArtifacts); err != nil {
		return types.ToolResponse{}, err
	}
	return resp, nil
}

// persist writes the results of a tool call.
//
// Write order:
//   - artifacts first (so response.json references are safe)
//   - response.json last (so readers see a complete result)
func (r *Runner) persist(callID string, resp types.ToolResponse, artifacts []ToolArtifactWrite) error {
	if r == nil || r.Results == nil {
		return fmt.Errorf("runner ResultsStore is required")
	}
	// Write artifacts first so response.json references are safe.
	for _, a := range artifacts {
		if _, err := validateAndCleanArtifactWrite(a); err != nil {
			return err
		}
		if err := r.Results.PutArtifact(callID, a.Path, a.MediaType, a.Bytes); err != nil {
			return err
		}
	}

	// Write response.json last.
	b, err := jsonutil.MarshalPretty(resp)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	if err := r.Results.PutCall(callID, b); err != nil {
		return err
	}
	return nil
}

// validateAndCleanArtifactWrite validates a tool-provided artifact before the runner writes it.
//
// Tools write artifacts relative to the call directory, e.g. "quote.json".
// This helper prevents directory traversal and ensures MediaType is present.
//
// It returns the normalized relative path that will be used for persistence and for
// ToolResponse.Artifacts[].Path so the agent sees a stable, canonical path.
func validateAndCleanArtifactWrite(a ToolArtifactWrite) (string, error) {
	if a.Path == "" {
		return "", fmt.Errorf("artifact path is required")
	}
	if a.MediaType == "" {
		return "", fmt.Errorf("artifact mediaType is required")
	}
	return vfsutil.CleanResultsArtifactPath(a.Path)
}
