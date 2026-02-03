package types

import (
	"encoding/json"
	"testing"
)

func TestHostOpRequest_TraceValidation_AllowsTraceActions(t *testing.T) {
	req := HostOpRequest{
		Op:     HostOpTrace,
		Action: "events.latest",
		Input:  json.RawMessage(`{}`),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestHostOpRequest_TraceValidation_RejectsUnknownActions(t *testing.T) {
	req := HostOpRequest{
		Op:     HostOpTrace,
		Action: "write",
		Input:  json.RawMessage(`{}`),
	}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for unsupported action")
	}
}

func TestHostOpRequest_BrowserValidation_RequiresInput(t *testing.T) {
	req := HostOpRequest{Op: HostOpBrowser}
	if err := req.Validate(); err == nil {
		t.Fatalf("expected error for missing input")
	}
}

func TestHostOpRequest_BrowserValidation_AllowsInput(t *testing.T) {
	req := HostOpRequest{
		Op:    HostOpBrowser,
		Input: json.RawMessage(`{"action":"start"}`),
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}
