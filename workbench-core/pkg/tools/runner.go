package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tinoosan/workbench-core/pkg/ports"
)

type Orchestrator struct {
	Results      ResultWriter
	ToolRegistry ToolInvokerRegistry
}

type ResultWriter = ports.ResultWriter

type ToolInvokerRegistry interface {
	Get(toolId ToolID) (ToolInvoker, bool)
}

type ToolInvoker interface {
	Invoke(ctx context.Context, req ToolRequest) (ToolCallResult, error)
}

type InvokeError struct {
	Code      string
	Message   string
	Retryable bool
	Err       error
}

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

type ToolCallResult struct {
	Output    json.RawMessage
	Artifacts []ToolArtifactWrite
}

type ToolArtifactWrite struct {
	Path      string
	Bytes     []byte
	MediaType string
}

type MapRegistry map[ToolID]ToolInvoker

func (m MapRegistry) Get(id ToolID) (ToolInvoker, bool) {
	inv, ok := m[id]
	return inv, ok
}

func (r *Orchestrator) Run(ctx context.Context, toolId ToolID, actionId string, input json.RawMessage, timeoutMs int) (ToolResponse, error) {
	if r == nil || r.Results == nil {
		return ToolResponse{}, fmt.Errorf("orchestrator ResultWriter is required")
	}
	if r.ToolRegistry == nil {
		return ToolResponse{}, fmt.Errorf("orchestrator ToolInvokerRegistry is required")
	}
	if err := nonEmpty("toolId", toolId.String()); err != nil {
		return ToolResponse{}, err
	}
	if err := nonEmpty("actionId", actionId); err != nil {
		return ToolResponse{}, err
	}
	if input == nil {
		return ToolResponse{}, fmt.Errorf("input is required")
	}
	if timeoutMs < 0 {
		return ToolResponse{}, fmt.Errorf("timeoutMs must be >= 0")
	}

	if timeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
		defer cancel()
	}

	callID := uuid.NewString()
	req := ToolRequest{
		Version:   "v1",
		CallID:    callID,
		ToolID:    toolId,
		ActionID:  actionId,
		Input:     input,
		TimeoutMs: timeoutMs,
	}
	if err := req.Validate(); err != nil {
		return ToolResponse{}, err
	}

	inv, ok := r.ToolRegistry.Get(toolId)
	if !ok {
		resp := NewToolResponseError(req, "unknown_tool", fmt.Sprintf("unknown tool %q", toolId.String()), false)
		if err := r.persist(callID, resp, nil); err != nil {
			return ToolResponse{}, err
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

		resp := NewToolResponseError(req, code, message, retryable)
		if err := r.persist(callID, resp, nil); err != nil {
			return ToolResponse{}, err
		}
		return resp, nil
	}

	artifactRefs := make([]ToolArtifactRef, 0, len(result.Artifacts))
	cleanedArtifacts := make([]ToolArtifactWrite, 0, len(result.Artifacts))
	for _, a := range result.Artifacts {
		clean, err := validateAndCleanArtifactWrite(a)
		if err != nil {
			resp := NewToolResponseError(req, "invalid_artifact", err.Error(), false)
			if err := r.persist(callID, resp, nil); err != nil {
				return ToolResponse{}, err
			}
			return resp, nil
		}
		ref := ToolArtifactRef{Path: clean, MediaType: a.MediaType}
		if err := ref.Validate(); err != nil {
			resp := NewToolResponseError(req, "invalid_artifact", err.Error(), false)
			if err := r.persist(callID, resp, nil); err != nil {
				return ToolResponse{}, err
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

	resp := NewToolResponseOK(req, result.Output, artifactRefs)
	if err := r.persist(callID, resp, cleanedArtifacts); err != nil {
		return ToolResponse{}, err
	}
	return resp, nil
}

func (r *Orchestrator) persist(callID string, resp ToolResponse, artifacts []ToolArtifactWrite) error {
	if r == nil || r.Results == nil {
		return fmt.Errorf("orchestrator ResultWriter is required")
	}
	for _, a := range artifacts {
		if _, err := validateAndCleanArtifactWrite(a); err != nil {
			return err
		}
		if err := r.Results.PutArtifact(callID, a.Path, a.MediaType, a.Bytes); err != nil {
			return err
		}
	}

	b, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	if err := r.Results.PutCall(callID, b); err != nil {
		return err
	}
	return nil
}

func validateAndCleanArtifactWrite(a ToolArtifactWrite) (string, error) {
	if err := nonEmpty("artifact path", a.Path); err != nil {
		return "", err
	}
	if err := nonEmpty("artifact mediaType", a.MediaType); err != nil {
		return "", err
	}
	return cleanResultsArtifactPath(a.Path)
}

func nonEmpty(name, s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func cleanResultsArtifactPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("artifact path is required")
	}
	clean, err := cleanRelPath(p)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "absolute paths not allowed"):
			return "", fmt.Errorf("artifact path must be relative")
		case strings.Contains(msg, "escapes mount root"):
			return "", fmt.Errorf("artifact path escapes results directory")
		default:
			return "", err
		}
	}
	if clean == "." {
		return "", fmt.Errorf("artifact path is invalid")
	}
	return clean, nil
}

func cleanRelPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("invalid path: empty")
	}
	if path.IsAbs(p) {
		return "", fmt.Errorf("invalid path: absolute paths not allowed")
	}
	rel := path.Clean(strings.ReplaceAll(p, "\\", "/"))
	for _, seg := range strings.Split(rel, "/") {
		if seg == ".." {
			return "", fmt.Errorf("invalid path: escapes mount root")
		}
	}
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("invalid path: escapes mount root")
	}
	return rel, nil
}
