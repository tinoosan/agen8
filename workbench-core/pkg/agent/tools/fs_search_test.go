package agenttools

import "testing"

func TestFSSearchTool_DefinitionRequiredIncludesLimit(t *testing.T) {
	tool := (&FSSearchTool{}).Definition()
	params, ok := tool.Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("expected parameters to be a map, got %T", tool.Function.Parameters)
	}
	req, ok := params["required"].([]any)
	if !ok {
		t.Fatalf("expected required to be []any, got %T", params["required"])
	}
	seen := map[string]bool{}
	for _, v := range req {
		if s, ok := v.(string); ok {
			seen[s] = true
		}
	}
	for _, k := range []string{"path", "query", "limit"} {
		if !seen[k] {
			t.Fatalf("expected required to include %q, got %#v", k, req)
		}
	}
}

