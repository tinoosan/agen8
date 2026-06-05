package app

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/contextlink"
	"github.com/tinoosan/agen8-mcp-server/internal/services/graph/domain"
	storagedb "github.com/tinoosan/agen8-mcp-server/internal/storage/db"
)

const contextLinkSchema = `
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
`

type graphTestHydrator struct {
	nodeType string
	nodes    map[string]domain.GraphNodeCore
}

func (h graphTestHydrator) NodeType() string { return h.nodeType }

func (h graphTestHydrator) Fetch(_ context.Context, _ string, nodeID string) (domain.GraphNodeCore, error) {
	if node, ok := h.nodes[nodeID]; ok {
		return node, nil
	}
	return domain.GraphNodeCore{}, sql.ErrNoRows
}

func (h graphTestHydrator) FetchMany(_ context.Context, _ string, nodeIDs []string) ([]domain.GraphNodeSummary, error) {
	out := make([]domain.GraphNodeSummary, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, ok := h.nodes[nodeID]
		if !ok {
			continue
		}
		out = append(out, domain.GraphNodeSummary{
			ID:        node.ID,
			Type:      node.Type,
			Title:     node.Title,
			Status:    node.Status,
			ScopeID:   node.ScopeID,
			CreatedAt: node.CreatedAt,
		})
	}
	return out, nil
}

func (h graphTestHydrator) Search(_ context.Context, _ string, _ string, limit int) ([]domain.GraphNodeSummary, error) {
	out := make([]domain.GraphNodeSummary, 0, len(h.nodes))
	for _, node := range h.nodes {
		out = append(out, domain.GraphNodeSummary{
			ID:        node.ID,
			Type:      node.Type,
			Title:     node.Title,
			Status:    node.Status,
			ScopeID:   node.ScopeID,
			CreatedAt: node.CreatedAt,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func newGraphQueryTestService(t *testing.T) (*Service, *contextlink.SQLiteRepository) {
	t.Helper()
	handle, err := storagedb.Open(context.Background(), storagedb.Config{
		Driver:       storagedb.DriverSQLite,
		DataDir:      t.TempDir(),
		MigrationKey: "graphquery-context-links-test",
		Migrate: func(ctx context.Context, db *sql.DB, driver storagedb.Driver) error {
			_, err := db.ExecContext(ctx, contextLinkSchema)
			return err
		},
	})
	if err != nil {
		t.Fatalf("open db handle: %v", err)
	}
	repo, err := contextlink.NewSQLiteRepository(handle)
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	decisionHydrator := graphTestHydrator{
		nodeType: domain.NodeTypeDecision,
		nodes: map[string]domain.GraphNodeCore{
			"dec-root":  {ID: "dec-root", Type: domain.NodeTypeDecision, Title: "Root decision", Status: "log", CreatedAt: now},
			"dec-child": {ID: "dec-child", Type: domain.NodeTypeDecision, Title: "Child decision", Status: "log", CreatedAt: now},
			"dec-leaf":  {ID: "dec-leaf", Type: domain.NodeTypeDecision, Title: "Leaf decision", Status: "log", CreatedAt: now},
			"dec-empty": {ID: "dec-empty", Type: domain.NodeTypeDecision, Title: "Unlinked decision", Status: "log", CreatedAt: now},
		},
	}
	svc, err := NewService(repo, []domain.NodeHydrator{decisionHydrator}, time.Second)
	if err != nil {
		t.Fatalf("new graph query service: %v", err)
	}
	return svc, repo
}

func TestServiceNodeInfersTypeAndExpandsDepth(t *testing.T) {
	ctx := context.Background()
	svc, links := newGraphQueryTestService(t)
	if err := links.Save(ctx, contextlink.Link{
		ID:         "cl-1",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-child"},
		EdgeType:   contextlink.EdgeTypeInformedBy,
		Confidence: 0.9,
		Metadata:   map[string]string{"rationale": "Root cites child."},
		CreatedAt:  time.Now().UTC(),
		CreatedBy:  "system",
	}); err != nil {
		t.Fatalf("save root link: %v", err)
	}
	if err := links.Save(ctx, contextlink.Link{
		ID:         "cl-2",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-child"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-leaf"},
		EdgeType:   contextlink.EdgeTypeInformedBy,
		Confidence: 1,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save child link: %v", err)
	}

	detail, _, err := svc.Node(ctx, "proj", "", "dec-root", 2)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if detail.Type != domain.NodeTypeDecision {
		t.Fatalf("type=%q", detail.Type)
	}
	if len(detail.Edges) != 1 || detail.Edges[0].Rationale != "Root cites child." {
		t.Fatalf("edges did not include rationale: %+v", detail.Edges)
	}
	if detail.Edges[0].ID != "cl-1" || detail.Edges[0].CreatedAt == "" || detail.Edges[0].CreatedBy != "system" || detail.Edges[0].Origin != "system" || detail.Edges[0].Source != "system" || detail.Edges[0].Manual {
		t.Fatalf("edge provenance not exposed: %+v", detail.Edges[0])
	}
	if len(detail.Subgraph) != 2 {
		t.Fatalf("subgraph=%d want 2: %+v", len(detail.Subgraph), detail.Subgraph)
	}
}

func TestServiceLinkStoresRationaleAndUnlinkRemovesEdge(t *testing.T) {
	ctx := context.Background()
	svc, _ := newGraphQueryTestService(t)
	edge, _, err := svc.Link(ctx, domain.GraphLinkRequest{
		ProjectID:  "proj",
		SourceID:   "dec-root",
		TargetID:   "dec-child",
		EdgeType:   string(contextlink.EdgeTypeInformedBy),
		Confidence: ptrFloat(0.8),
		Rationale:  "The child workstream supports this decision.",
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if edge.Rationale != "The child workstream supports this decision." {
		t.Fatalf("rationale=%q", edge.Rationale)
	}
	if edge.Operation != "created" {
		t.Fatalf("operation=%q want created", edge.Operation)
	}
	if edge.SourceType != domain.NodeTypeDecision || edge.TargetType != domain.NodeTypeDecision {
		t.Fatalf("edge types were not inferred: %+v", edge)
	}
	if edge.ID == "" || edge.CreatedAt == "" || edge.CreatedBy != "graph_query" || edge.Origin != "manual" || edge.Source != "manual" || !edge.Manual {
		t.Fatalf("edge missing provenance: %+v", edge)
	}
	unchanged, _, err := svc.Link(ctx, domain.GraphLinkRequest{
		ProjectID:  "proj",
		SourceType: domain.NodeTypeDecision,
		SourceID:   "dec-root",
		TargetType: domain.NodeTypeDecision,
		TargetID:   "dec-child",
		EdgeType:   string(contextlink.EdgeTypeInformedBy),
		Confidence: ptrFloat(0.8),
		Rationale:  "The child workstream supports this decision.",
	})
	if err != nil {
		t.Fatalf("link noop: %v", err)
	}
	if unchanged.Operation != "noop" {
		t.Fatalf("operation=%q want noop", unchanged.Operation)
	}
	if unchanged.ID != edge.ID || unchanged.CreatedAt != edge.CreatedAt || !unchanged.Manual {
		t.Fatalf("noop did not preserve edge provenance: created=%+v noop=%+v", edge, unchanged)
	}
	updated, _, err := svc.Link(ctx, domain.GraphLinkRequest{
		ProjectID:  "proj",
		SourceType: domain.NodeTypeDecision,
		SourceID:   "dec-root",
		TargetType: domain.NodeTypeDecision,
		TargetID:   "dec-child",
		EdgeType:   string(contextlink.EdgeTypeInformedBy),
		Confidence: ptrFloat(0.9),
		Rationale:  "Updated rationale.",
	})
	if err != nil {
		t.Fatalf("link update: %v", err)
	}
	if updated.Operation != "updated" || updated.Rationale != "Updated rationale." {
		t.Fatalf("updated edge=%+v", updated)
	}
	if updated.ID != edge.ID || updated.CreatedBy != "graph_query" || updated.Origin != "manual" || updated.Source != "manual" {
		t.Fatalf("updated edge did not preserve provenance: %+v", updated)
	}

	unlinked, _, err := svc.Unlink(ctx, domain.GraphLinkRequest{
		ProjectID: "proj",
		EdgeID:    edge.ID,
	})
	if err != nil {
		t.Fatalf("unlink: %v", err)
	}
	if unlinked.Rationale != "Updated rationale." {
		t.Fatalf("unlinked rationale=%q", unlinked.Rationale)
	}
	if unlinked.Operation != "deleted" {
		t.Fatalf("unlink operation=%q want deleted", unlinked.Operation)
	}
	if unlinked.ID != edge.ID || unlinked.CreatedAt == "" || unlinked.Origin != "manual" || unlinked.Source != "manual" || !unlinked.Manual {
		t.Fatalf("unlinked edge missing provenance: %+v", unlinked)
	}
	detail, _, err := svc.Node(ctx, "proj", domain.NodeTypeDecision, "dec-root", 1)
	if err != nil {
		t.Fatalf("node after unlink: %v", err)
	}
	if len(detail.Edges) != 0 {
		t.Fatalf("edges after unlink=%d want 0", len(detail.Edges))
	}
}

func TestServiceLinkStoresReferenceOrigin(t *testing.T) {
	ctx := context.Background()
	svc, _ := newGraphQueryTestService(t)
	edge, _, err := svc.Link(ctx, domain.GraphLinkRequest{
		ProjectID:  "proj",
		SourceID:   "dec-root",
		TargetID:   "dec-child",
		EdgeType:   string(contextlink.EdgeTypeInformedBy),
		Confidence: ptrFloat(1),
		Rationale:  "Decision references this work item.",
		Origin:     "reference",
		CreatedBy:  "decision_service",
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if edge.Origin != "reference" || edge.Source != "reference" || edge.Manual {
		t.Fatalf("edge origin=%+v", edge)
	}
	if edge.CreatedBy != "decision_service" {
		t.Fatalf("createdBy=%q want decision_service", edge.CreatedBy)
	}
}

func TestServiceLinkRepairsExistingReferenceProvenance(t *testing.T) {
	ctx := context.Background()
	svc, links := newGraphQueryTestService(t)
	if err := links.Save(ctx, contextlink.Link{
		ID:         "cl-legacy-reference",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-child"},
		EdgeType:   contextlink.EdgeTypeInformedBy,
		Confidence: 1,
		Metadata: map[string]string{
			"origin":    "explicit",
			"rationale": "Decision references this work item.",
		},
		CreatedAt: time.Now().UTC(),
		CreatedBy: "graph_query",
	}); err != nil {
		t.Fatalf("save legacy link: %v", err)
	}

	detail, _, err := svc.Node(ctx, "proj", domain.NodeTypeDecision, "dec-root", 1)
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	if len(detail.Edges) != 1 || detail.Edges[0].Origin != "reference" || detail.Edges[0].Manual || detail.Edges[0].CreatedBy != "decision_service" {
		t.Fatalf("legacy edge not interpreted as reference: %+v", detail.Edges)
	}

	edge, _, err := svc.Link(ctx, domain.GraphLinkRequest{
		ProjectID:  "proj",
		SourceID:   "dec-root",
		TargetID:   "dec-child",
		EdgeType:   string(contextlink.EdgeTypeInformedBy),
		Confidence: ptrFloat(1),
		Rationale:  "Decision references this work item.",
		Origin:     "reference",
		CreatedBy:  "decision_service",
	})
	if err != nil {
		t.Fatalf("repair link: %v", err)
	}
	if edge.Operation != "updated" || edge.Origin != "reference" || edge.Manual || edge.CreatedBy != "decision_service" {
		t.Fatalf("repair edge=%+v", edge)
	}
	stored, err := links.FindByID(ctx, contextlink.ID("cl-legacy-reference"))
	if err != nil {
		t.Fatalf("find repaired link: %v", err)
	}
	if stored.Metadata["origin"] != "reference" || stored.CreatedBy != "decision_service" {
		t.Fatalf("stored repaired provenance=%+v createdBy=%q", stored.Metadata, stored.CreatedBy)
	}
}

func TestServiceLinkRequiresTypeWhenPrefixUnknown(t *testing.T) {
	ctx := context.Background()
	svc, _ := newGraphQueryTestService(t)
	_, _, err := svc.Link(ctx, domain.GraphLinkRequest{
		ProjectID: "proj",
		SourceID:  "unknown-1",
		TargetID:  "dec-child",
		EdgeType:  string(contextlink.EdgeTypeInformedBy),
	})
	if err == nil {
		t.Fatal("expected source type inference error")
	}
}

func TestServiceSearchFiltersMissingEdge(t *testing.T) {
	ctx := context.Background()
	svc, links := newGraphQueryTestService(t)
	if err := links.Save(ctx, contextlink.Link{
		ID:         "cl-1",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-child"},
		EdgeType:   contextlink.EdgeTypeInformedBy,
		Confidence: 1,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save link: %v", err)
	}

	results, _, err := svc.Search(ctx, domain.GraphSearchRequest{
		ProjectID:   "proj",
		NodeType:    domain.NodeTypeDecision,
		Query:       "",
		Limit:       10,
		MissingEdge: string(contextlink.EdgeTypeInformedBy),
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	for _, result := range results {
		if result.ID == "dec-root" || result.ID == "dec-child" {
			t.Fatalf("result with informed_by edge was not filtered: %+v", results)
		}
	}
}

func TestServiceSearchFiltersDirectionalEdges(t *testing.T) {
	ctx := context.Background()
	svc, links := newGraphQueryTestService(t)
	if err := links.Save(ctx, contextlink.Link{
		ID:         "cl-outgoing",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-child"},
		EdgeType:   contextlink.EdgeTypeInformedBy,
		Confidence: 1,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save outgoing link: %v", err)
	}
	if err := links.Save(ctx, contextlink.Link{
		ID:         "cl-incoming",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-leaf"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"},
		EdgeType:   contextlink.EdgeTypeServes,
		Confidence: 1,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save incoming link: %v", err)
	}

	outgoing, _, err := svc.Search(ctx, domain.GraphSearchRequest{
		ProjectID:    "proj",
		NodeType:     domain.NodeTypeDecision,
		Limit:        10,
		OutgoingEdge: string(contextlink.EdgeTypeInformedBy),
	})
	if err != nil {
		t.Fatalf("search outgoing: %v", err)
	}
	if !containsNodeID(outgoing, "dec-root") || containsNodeID(outgoing, "dec-child") {
		t.Fatalf("outgoing results=%+v", outgoing)
	}
	for _, result := range outgoing {
		if result.ID != "dec-root" {
			continue
		}
		if len(result.MatchedEdges) != 1 || result.MatchedEdges[0].ID != "cl-outgoing" {
			t.Fatalf("matched outgoing edges=%+v", result.MatchedEdges)
		}
	}

	incoming, _, err := svc.Search(ctx, domain.GraphSearchRequest{
		ProjectID:    "proj",
		NodeType:     domain.NodeTypeDecision,
		Limit:        10,
		IncomingEdge: string(contextlink.EdgeTypeInformedBy),
	})
	if err != nil {
		t.Fatalf("search incoming: %v", err)
	}
	if !containsNodeID(incoming, "dec-child") || containsNodeID(incoming, "dec-root") {
		t.Fatalf("incoming results=%+v", incoming)
	}
	for _, result := range incoming {
		if result.ID != "dec-child" {
			continue
		}
		if len(result.MatchedEdges) != 1 || result.MatchedEdges[0].ID != "cl-outgoing" {
			t.Fatalf("matched incoming edges=%+v", result.MatchedEdges)
		}
	}
}

func TestServiceSearchBrowsesConcreteNodeTypeWithoutQuery(t *testing.T) {
	ctx := context.Background()
	svc, _ := newGraphQueryTestService(t)
	results, _, err := svc.Search(ctx, domain.GraphSearchRequest{
		ProjectID: "proj",
		NodeType:  domain.NodeTypeDecision,
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("search browse: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("results empty")
	}
}

func TestServiceDeleteLinksForNodeRemovesIncidentEdges(t *testing.T) {
	ctx := context.Background()
	svc, links := newGraphQueryTestService(t)
	if err := links.Save(ctx, contextlink.Link{
		ID:         "cl-source",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-child"},
		EdgeType:   contextlink.EdgeTypeInformedBy,
		Confidence: 1,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save source link: %v", err)
	}
	if err := links.Save(ctx, contextlink.Link{
		ID:         "cl-target",
		Source:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-leaf"},
		Target:     contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"},
		EdgeType:   contextlink.EdgeTypeServes,
		Confidence: 1,
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save target link: %v", err)
	}

	if err := svc.DeleteLinksForNode(ctx, domain.NodeTypeDecision, "dec-root"); err != nil {
		t.Fatalf("DeleteLinksForNode: %v", err)
	}

	outgoing, err := links.FindBySource(ctx, contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"})
	if err != nil {
		t.Fatalf("FindBySource: %v", err)
	}
	incoming, err := links.FindByTarget(ctx, contextlink.NodeRef{Type: contextlink.NodeType(domain.NodeTypeDecision), ID: "dec-root"})
	if err != nil {
		t.Fatalf("FindByTarget: %v", err)
	}
	if len(outgoing) != 0 || len(incoming) != 0 {
		t.Fatalf("links remain after delete: outgoing=%+v incoming=%+v", outgoing, incoming)
	}
}

func TestServiceDeleteLinksForNodeRequiresNodeRef(t *testing.T) {
	ctx := context.Background()
	svc, _ := newGraphQueryTestService(t)
	if err := svc.DeleteLinksForNode(ctx, "", "dec-root"); err == nil {
		t.Fatal("expected missing node type error")
	}
	if err := svc.DeleteLinksForNode(ctx, domain.NodeTypeDecision, ""); err == nil {
		t.Fatal("expected missing node id error")
	}
}

func TestServiceSearchAllPreservesRelevanceOrder(t *testing.T) {
	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	relevantOlder := now.Add(-time.Hour).Format(time.RFC3339Nano)
	irrelevantNewer := now.Format(time.RFC3339Nano)
	svc, err := NewService(noopGraphLinks{}, []domain.NodeHydrator{
		graphTestHydrator{
			nodeType: domain.NodeTypeDecision,
			nodes: map[string]domain.GraphNodeCore{
				"dec-metrics": {ID: "dec-metrics", Type: domain.NodeTypeDecision, Title: "Restore metrics-server and kubectl top", CreatedAt: relevantOlder},
			},
		},
		graphTestHydrator{
			nodeType: domain.NodeTypeTask,
			nodes: map[string]domain.GraphNodeCore{
				"task-oom": {ID: "task-oom", Type: domain.NodeTypeTask, Title: "Investigate and eliminate recent SystemOOM risk", CreatedAt: irrelevantNewer},
			},
		},
	}, time.Second)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	results, _, err := svc.Search(context.Background(), domain.GraphSearchRequest{
		ProjectID: "proj",
		NodeType:  domain.NodeTypeAll,
		Query:     "metrics",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("results=%+v", results)
	}
	if results[0].ID != "dec-metrics" {
		t.Fatalf("first result=%+v want dec-metrics", results[0])
	}
}

func TestMatchesQueryAllowsMeaningfulMultiTermSubset(t *testing.T) {
	if !matchesQuery("graph search traversal readability density ranking", "Prioritize graph readability controls over new graph semantics") {
		t.Fatal("expected multi-term query to match meaningful token subset")
	}
	if matchesQuery("graph search traversal readability density ranking", "Release baseline setup and verification") {
		t.Fatal("expected unrelated title not to match sparse multi-term query")
	}
	if !matchesQuery("release graph", "Release baseline setup and verification") {
		t.Fatal("expected two-term query to match one meaningful token")
	}
}

func TestSortSummariesBySearchScoreFavorsStrongerTokenOverlap(t *testing.T) {
	now := time.Date(2026, 4, 24, 10, 0, 0, 0, time.UTC)
	summaries := []domain.GraphNodeSummary{
		{ID: "dec-newer", Type: domain.NodeTypeDecision, Title: "Graph notes", CreatedAt: now.Format(time.RFC3339Nano)},
		{ID: "dec-older", Type: domain.NodeTypeDecision, Title: "Graph readability controls and graph semantics", CreatedAt: now.Add(-time.Hour).Format(time.RFC3339Nano)},
	}
	sortSummariesBySearchScore(summaries, "graph readability semantics")
	if summaries[0].ID != "dec-older" {
		t.Fatalf("first result=%+v want stronger token overlap", summaries[0])
	}
}

func ptrFloat(v float64) *float64 { return &v }

func containsNodeID(nodes []domain.GraphNodeSummary, id string) bool {
	for _, node := range nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

type noopGraphLinks struct{}

func (noopGraphLinks) FindByID(context.Context, contextlink.ID) (contextlink.Link, error) {
	return contextlink.Link{}, sql.ErrNoRows
}

func (noopGraphLinks) FindBySource(context.Context, contextlink.NodeRef) ([]contextlink.Link, error) {
	return []contextlink.Link{}, nil
}

func (noopGraphLinks) FindByTarget(context.Context, contextlink.NodeRef) ([]contextlink.Link, error) {
	return []contextlink.Link{}, nil
}

func (noopGraphLinks) FindBetween(context.Context, contextlink.NodeRef, contextlink.NodeRef) ([]contextlink.Link, error) {
	return []contextlink.Link{}, nil
}

func (noopGraphLinks) FindByEdgeType(context.Context, contextlink.EdgeType, int) ([]contextlink.Link, error) {
	return []contextlink.Link{}, nil
}

func (noopGraphLinks) Save(context.Context, contextlink.Link) error { return nil }

func (noopGraphLinks) Replace(context.Context, contextlink.Link) error { return nil }

func (noopGraphLinks) Delete(context.Context, contextlink.ID) error { return nil }

func (noopGraphLinks) DeleteLinksForEntity(context.Context, contextlink.NodeRef) error { return nil }
