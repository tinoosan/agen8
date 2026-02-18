package hosttools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pkgsoul "github.com/tinoosan/workbench-core/pkg/services/soul"
)

type mockSoulUpdater struct {
	last pkgsoul.UpdateRequest
	doc  pkgsoul.Doc
	err  error
}

func (m *mockSoulUpdater) Update(_ context.Context, req pkgsoul.UpdateRequest) (pkgsoul.Doc, error) {
	m.last = req
	if m.doc.Version == 0 {
		m.doc.Version = 2
	}
	return m.doc, m.err
}

func TestSoulUpdateTool(t *testing.T) {
	up := &mockSoulUpdater{}
	tool := &SoulUpdateTool{Updater: up}
	args, _ := json.Marshal(map[string]any{"content": "# SOUL\n\n## Constitutional Core\nA\n\n## Long-Horizon Intent\nB\n\n## Operating Constraints\nC\n\n## Change Policy\nD\n", "reason": "adapt"})
	req, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if req.Tag != "soul_update" || !strings.Contains(req.Text, "version") {
		t.Fatalf("unexpected req: %+v", req)
	}
	if up.last.Actor != pkgsoul.ActorAgent {
		t.Fatalf("expected default actor=agent, got %q", up.last.Actor)
	}
}
