package tools

import (
	"context"
	"testing"
)

type staticProvider struct {
	ids  []ToolID
	data map[ToolID][]byte
}

func (s staticProvider) ListToolIDs(_ context.Context) ([]ToolID, error) {
	return append([]ToolID(nil), s.ids...), nil
}

func (s staticProvider) GetManifest(_ context.Context, toolID ToolID) ([]byte, bool, error) {
	b, ok := s.data[toolID]
	if !ok {
		return nil, false, nil
	}
	return b, true, nil
}

func TestCompositeToolManifestRegistry_DedupesAndOrders(t *testing.T) {
	p1 := staticProvider{
		ids: []ToolID{ToolID("tool.a"), ToolID("tool.b")},
		data: map[ToolID][]byte{
			ToolID("tool.a"): []byte(`{"id":"tool.a"}`),
			ToolID("tool.b"): []byte(`{"id":"tool.b"}`),
		},
	}
	p2 := staticProvider{
		ids: []ToolID{ToolID("tool.b"), ToolID("tool.c")},
		data: map[ToolID][]byte{
			ToolID("tool.b"): []byte(`{"id":"tool.b"}`),
			ToolID("tool.c"): []byte(`{"id":"tool.c"}`),
		},
	}

	reg := NewCompositeToolManifestRegistry(p1, p2)
	ids, err := reg.ListToolIDs(context.Background())
	if err != nil {
		t.Fatalf("ListToolIDs: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %d", len(ids))
	}
	if ids[0].String() != "tool.a" || ids[1].String() != "tool.b" || ids[2].String() != "tool.c" {
		t.Fatalf("unexpected order: %v", ids)
	}
}
