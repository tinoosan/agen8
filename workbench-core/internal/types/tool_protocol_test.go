package types

import (
	"encoding/json"
	"testing"
)

func TestToolRequestValidate(t *testing.T) {
	valid := ToolRequest{
		Version:  "v1",
		CallID:   "123",
		ToolID:   ToolID("github.com.acme.tool"),
		ActionID: "acme.do",
		Input:    json.RawMessage(`{}`),
	}

	t.Run("Valid", func(t *testing.T) {
		if err := valid.Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("MissingCallID", func(t *testing.T) {
		r := valid
		r.CallID = ""
		if err := r.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("MissingToolID", func(t *testing.T) {
		r := valid
		r.ToolID = ""
		if err := r.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("MissingActionID", func(t *testing.T) {
		r := valid
		r.ActionID = ""
		if err := r.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("NilInput", func(t *testing.T) {
		r := valid
		r.Input = nil
		if err := r.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("InvalidJSONInput", func(t *testing.T) {
		r := valid
		r.Input = json.RawMessage(`{`)
		if err := r.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("NegativeTimeout", func(t *testing.T) {
		r := valid
		r.TimeoutMs = -1
		if err := r.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestToolResponseValidate(t *testing.T) {
	req := ToolRequest{
		Version:  "v1",
		CallID:   "123",
		ToolID:   ToolID("github.com.acme.tool"),
		ActionID: "acme.do",
		Input:    json.RawMessage(`{}`),
	}

	t.Run("OkTrueNilError", func(t *testing.T) {
		resp := NewToolResponseOK(req, nil, nil)
		if err := resp.Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("OkFalseNilErrorFails", func(t *testing.T) {
		resp := ToolResponse{
			Version:  "v1",
			CallID:   req.CallID,
			ToolID:   req.ToolID,
			ActionID: req.ActionID,
			Ok:       false,
			Error:    nil,
		}
		if err := resp.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("OkFalseInvalidErrorFails", func(t *testing.T) {
		resp := NewToolResponseError(req, "", "", false)
		if err := resp.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("MissingRequiredFieldsFails", func(t *testing.T) {
		resp := NewToolResponseOK(req, nil, nil)
		resp.CallID = ""
		if err := resp.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("InvalidJSONOutputFails", func(t *testing.T) {
		resp := NewToolResponseOK(req, json.RawMessage(`{`), nil)
		if err := resp.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestToolArtifactRefValidate(t *testing.T) {
	t.Run("LeadingSlashRejected", func(t *testing.T) {
		a := ToolArtifactRef{Path: "/abs", MediaType: "application/json"}
		if err := a.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("EmptyMediaTypeRejected", func(t *testing.T) {
		a := ToolArtifactRef{Path: "artifacts/x.json", MediaType: ""}
		if err := a.Validate(); err == nil {
			t.Fatalf("expected error")
		}
	})
}
