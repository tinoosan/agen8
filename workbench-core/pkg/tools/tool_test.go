package tools

import "testing"

func TestParseToolID(t *testing.T) {
	t.Run("Normalizes", func(t *testing.T) {
		id, err := ParseToolID("  GitHub.com.Acme.Tool  ")
		if err != nil {
			t.Fatalf("ParseToolID: %v", err)
		}
		if id.String() != "github.com.acme.tool" {
			t.Fatalf("got %q", id.String())
		}
	})

	t.Run("RejectsInvalid", func(t *testing.T) {
		if _, err := ParseToolID("nope"); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestToolManifestValidate(t *testing.T) {
	m := ToolManifest{
		ID:          ToolID("github.com.acme.tool"),
		Version:     "0.1.0",
		Kind:        ToolKindBuiltin,
		DisplayName: "Acme Tool",
		Description: "Does things",
		Actions: []ToolAction{
			{
				ID:           ActionID("acme.do"),
				DisplayName:  "Do",
				Description:  "Do it",
				InputSchema:  []byte(`{"type":"object"}`),
				OutputSchema: []byte(`{"type":"object"}`),
			},
		},
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestToolManifestValidate_DuplicateActionID(t *testing.T) {
	m := ToolManifest{
		ID:          ToolID("github.com.acme.tool"),
		Version:     "0.1.0",
		Kind:        ToolKindBuiltin,
		DisplayName: "Acme Tool",
		Description: "Does things",
		Actions: []ToolAction{
			{
				ID:           ActionID("acme.do"),
				DisplayName:  "Do 1",
				Description:  "Do it",
				InputSchema:  []byte(`{"type":"object"}`),
				OutputSchema: []byte(`{"type":"object"}`),
			},
			{
				ID:           ActionID("acme.do"),
				DisplayName:  "Do 2",
				Description:  "Do it again",
				InputSchema:  []byte(`{"type":"object"}`),
				OutputSchema: []byte(`{"type":"object"}`),
			},
		},
	}
	if err := m.Validate(); err == nil {
		t.Fatalf("expected error")
	}
}
