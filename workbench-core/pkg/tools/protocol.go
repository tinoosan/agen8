package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrToolProtocolVersionRequired    = errors.New("version is required")
	ErrToolProtocolUnsupportedVersion = errors.New("unsupported version")
	ErrToolCallIDRequired             = errors.New("callId is required")
	ErrToolIDRequired                 = errors.New("toolId is required")
	ErrToolActionIDRequired           = errors.New("actionId is required")
	ErrToolRequestInputRequired       = errors.New("input is required")
	ErrToolRequestInputInvalidJSON    = errors.New("input must be valid JSON")
	ErrToolRequestTimeoutInvalid      = errors.New("timeoutMs must be >= 0")

	ErrToolResponseErrorMustBeNilWhenOK = errors.New("error must be nil when ok=true")
	ErrToolResponseErrorRequiredWhenNotOK = errors.New("error is required when ok=false")
	ErrToolResponseOutputInvalidJSON     = errors.New("output must be valid JSON")

	ErrToolErrorCodeRequired    = errors.New("error.code is required")
	ErrToolErrorMessageRequired = errors.New("error.message is required")

	ErrToolArtifactPathRequired       = errors.New("artifact.path is required")
	ErrToolArtifactPathMustBeRelative = errors.New("artifact.path must be relative")
	ErrToolArtifactMediaTypeRequired  = errors.New("artifact.mediaType is required")
)

type ToolRequest struct {
	Version   string            `json:"version"`
	CallID    string            `json:"callId"`
	ToolID    ToolID            `json:"toolId"`
	ActionID  string            `json:"actionId"`
	Input     json.RawMessage   `json:"input"`
	TimeoutMs int               `json:"timeoutMs,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

func (r ToolRequest) Validate() error {
	if r.Version != "v1" {
		if r.Version == "" {
			return ErrToolProtocolVersionRequired
		}
		return fmt.Errorf("%w %q", ErrToolProtocolUnsupportedVersion, r.Version)
	}
	if r.CallID == "" {
		return ErrToolCallIDRequired
	}
	if r.ToolID.String() == "" {
		return ErrToolIDRequired
	}
	if _, err := ParseToolID(r.ToolID.String()); err != nil {
		return err
	}
	if r.ActionID == "" {
		return ErrToolActionIDRequired
	}
	if _, err := ParseActionID(r.ActionID); err != nil {
		return err
	}
	if r.Input == nil {
		return ErrToolRequestInputRequired
	}
	if !json.Valid(r.Input) {
		return ErrToolRequestInputInvalidJSON
	}
	if r.TimeoutMs < 0 {
		return ErrToolRequestTimeoutInvalid
	}
	return nil
}

type ToolError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

func (e ToolError) Validate() error {
	if e.Code == "" {
		return ErrToolErrorCodeRequired
	}
	if e.Message == "" {
		return ErrToolErrorMessageRequired
	}
	return nil
}

type ToolArtifactRef struct {
	Path      string `json:"path"`
	MediaType string `json:"mediaType"`
}

func (a ToolArtifactRef) Validate() error {
	if a.Path == "" {
		return ErrToolArtifactPathRequired
	}
	if strings.HasPrefix(a.Path, "/") {
		return ErrToolArtifactPathMustBeRelative
	}
	if a.MediaType == "" {
		return ErrToolArtifactMediaTypeRequired
	}
	return nil
}

type ToolResponse struct {
	Version   string            `json:"version"`
	CallID    string            `json:"callId"`
	ToolID    ToolID            `json:"toolId"`
	ActionID  string            `json:"actionId"`
	Ok        bool              `json:"ok"`
	Error     *ToolError        `json:"error,omitempty"`
	Output    json.RawMessage   `json:"output,omitempty"`
	Artifacts []ToolArtifactRef `json:"artifacts,omitempty"`
}

func (r ToolResponse) Validate() error {
	if r.Version != "v1" {
		if r.Version == "" {
			return ErrToolProtocolVersionRequired
		}
		return fmt.Errorf("%w %q", ErrToolProtocolUnsupportedVersion, r.Version)
	}
	if r.CallID == "" {
		return ErrToolCallIDRequired
	}
	if r.ToolID.String() == "" {
		return ErrToolIDRequired
	}
	if _, err := ParseToolID(r.ToolID.String()); err != nil {
		return err
	}
	if r.ActionID == "" {
		return ErrToolActionIDRequired
	}
	if _, err := ParseActionID(r.ActionID); err != nil {
		return err
	}
	if r.Output != nil && !json.Valid(r.Output) {
		return ErrToolResponseOutputInvalidJSON
	}
	for _, a := range r.Artifacts {
		if err := a.Validate(); err != nil {
			return err
		}
	}

	if r.Ok {
		if r.Error != nil {
			return ErrToolResponseErrorMustBeNilWhenOK
		}
		return nil
	}
	if r.Error == nil {
		return ErrToolResponseErrorRequiredWhenNotOK
	}
	return r.Error.Validate()
}

func NewToolResponseOK(req ToolRequest, output json.RawMessage, artifacts []ToolArtifactRef) ToolResponse {
	return ToolResponse{
		Version:   req.Version,
		CallID:    req.CallID,
		ToolID:    req.ToolID,
		ActionID:  req.ActionID,
		Ok:        true,
		Error:     nil,
		Output:    output,
		Artifacts: artifacts,
	}
}

func NewToolResponseError(req ToolRequest, code, message string, retryable bool) ToolResponse {
	return ToolResponse{
		Version:  req.Version,
		CallID:   req.CallID,
		ToolID:   req.ToolID,
		ActionID: req.ActionID,
		Ok:       false,
		Error: &ToolError{
			Code:      code,
			Message:   message,
			Retryable: retryable,
		},
	}
}

type ToolResultIndexLine struct {
	CallID     string `json:"callId"`
	ToolID     ToolID `json:"toolId"`
	ActionID   string     `json:"actionId"`
	Ok         bool       `json:"ok"`
	FinishedAt string     `json:"finishedAt"`
	Summary    string     `json:"summary,omitempty"`
}

func NewIndexLineFromResponse(resp ToolResponse, summary string) ToolResultIndexLine {
	return ToolResultIndexLine{
		CallID:     resp.CallID,
		ToolID:     resp.ToolID,
		ActionID:   resp.ActionID,
		Ok:         resp.Ok,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Summary:    summary,
	}
}
