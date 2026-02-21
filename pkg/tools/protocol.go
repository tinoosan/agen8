package tools

import (
	"encoding/json"
	"errors"
	"fmt"
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
