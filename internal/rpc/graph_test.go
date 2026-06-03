package rpc

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	graphapp "github.com/tinoosan/agen8-mcp-server/internal/services/graph/app"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/contextlink"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	graphrpc "github.com/tinoosan/agen8-mcp-server/internal/services/graph/rpc"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

type rpcGraphHydrator struct{}

func (rpcGraphHydrator) NodeType() string { return domain.NodeTypeDecision }

func (rpcGraphHydrator) Fetch(_ context.Context, _ string, nodeID string) (domain.GraphNodeCore, error) {
	return domain.GraphNodeCore{
		ID:        nodeID,
		Type:      domain.NodeTypeDecision,
		Title:     "Decision",
		CreatedAt: time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
	}, nil
}

func (rpcGraphHydrator) FetchMany(ctx context.Context, projectID string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	out := make([]domain.GraphNodeSummary, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, err := (rpcGraphHydrator{}).Fetch(ctx, projectID, nodeID)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        node.ID,
			Type:      node.Type,
			Title:     node.Title,
			CreatedAt: node.CreatedAt,
		})
	}
	return out, nil
}

func (rpcGraphHydrator) Search(_ context.Context, _ string, _ string, _ int) ([]domain.GraphNodeSummary, error) {
	return []domain.GraphNodeSummary{}, nil
}

func newRPCGraphService(t *testing.T) (*graphapp.Service, contextlink.Repository) {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:       storagedb.DriverSQLite,
		DataDir:      t.TempDir(),
		MigrationKey: "rpc-graph-context-links-test",
		Migrate: func(ctx context.Context, db *sql.DB, _ storagedb.Driver) error {
			_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS context_links (
	id TEXT PRIMARY KEY,
	source_type TEXT NOT NULL,
	source_id TEXT NOT NULL,
	target_type TEXT NOT NULL,
	target_id TEXT NOT NULL,
	edge_type TEXT NOT NULL,
	confidence REAL NOT NULL DEFAULT 1.0,
	metadata_json TEXT DEFAULT '{}',
	created_at TEXT NOT NULL,
	created_by TEXT DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_cl_source ON context_links(source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_cl_target ON context_links(target_type, target_id);
CREATE INDEX IF NOT EXISTS idx_cl_edge_type ON context_links(edge_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_cl_unique_edge ON context_links(source_type, source_id, target_type, target_id, edge_type);
`)
			return err
		},
	})
	if err != nil {
		t.Fatalf("open db handle: %v", err)
	}
	links, err := contextlink.NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	svc, err := graphapp.NewService(links, []domain.NodeHydrator{rpcGraphHydrator{}}, time.Second)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, links
}

func TestRegisterGraphDispatchLinksBySource(t *testing.T) {
	graphSvc, links := newRPCGraphService(t)
	if err := links.Save(context.Background(), contextlink.Link{
		ID:         "cl-1",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-1"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-2"},
		EdgeType:   contextlink.EdgeTypeInformedBy,
		Confidence: 0.8,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	reg := NewRegistry()
	if err := RegisterGraph(reg, graphSvc, links); err != nil {
		t.Fatalf("RegisterGraph returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "graph.linksBySource",
		"params": { "sourceType": "decision", "sourceId": "dec-1" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error != nil {
		t.Fatalf("response error=%+v", resp.Error)
	}
	var result graphrpc.LinksBySourceResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(result.ContextLinks) != 1 || result.ContextLinks[0].ID != "cl-1" {
		t.Fatalf("context links=%+v", result.ContextLinks)
	}
}

func TestRegisterGraphMapsInvalidParams(t *testing.T) {
	graphSvc, links := newRPCGraphService(t)
	reg := NewRegistry()
	if err := RegisterGraph(reg, graphSvc, links); err != nil {
		t.Fatalf("RegisterGraph returned error: %v", err)
	}
	server, err := NewServer(reg)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	raw, err := server.Handle(context.Background(), []byte(`{
		"jsonrpc": "2.0",
		"id": "1",
		"method": "graph.linksBySource",
		"params": { "sourceType": "decision" }
	}`))
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	resp := decodeRPCResponse(t, raw)
	if resp.Error == nil || resp.Error.Code != CodeInternalError {
		t.Fatalf("response error=%+v", resp.Error)
	}
}
