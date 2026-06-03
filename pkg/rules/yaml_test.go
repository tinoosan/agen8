package rules

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDecodeComposeOptionsYAML_DecodesLineMetadata(t *testing.T) {
	raw := `
add:
  - name: custom_rule
    order: 120
    appliesTo: [worker]
    lines:
      - one
      - two
disable:
  - delegation
append:
  review_handling:
    - check output
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(root.Content) == 0 {
		t.Fatalf("missing content")
	}
	rulesNode := root.Content[0]
	out, err := DecodeComposeOptionsYAML[rune](rulesNode, func(s string) (rune, error) {
		if strings.EqualFold(strings.TrimSpace(s), "worker") {
			return 'w', nil
		}
		return 0, nil
	})
	if err != nil {
		t.Fatalf("DecodeComposeOptionsYAML: %v", err)
	}
	if len(out.Add) != 1 || len(out.Disable) != 1 || len(out.Append) != 1 {
		t.Fatalf("decoded=%+v", out)
	}
	if out.Add[0].Line == 0 || out.Disable[0].Line == 0 || out.Append[0].Line == 0 {
		t.Fatalf("expected line metadata: %+v", out)
	}
}
