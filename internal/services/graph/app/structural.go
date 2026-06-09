package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/tinoosan/agen8/internal/core/types"
	decisiondomain "github.com/tinoosan/agen8/internal/services/decision/domain"
	"github.com/tinoosan/agen8/internal/services/graph/domain"
	krdomain "github.com/tinoosan/agen8/internal/services/mission/domain/kr"
	missiondomain "github.com/tinoosan/agen8/internal/services/mission/domain/mission"
	taskdomain "github.com/tinoosan/agen8/internal/services/task/domain"
)

// StructuralEdgeResolver derives the structural skeleton of the graph
// (mission -> key result -> task -> decision lineage) directly from the entity
// refs each node already carries, at read time. This is the single source of
// truth for structure: no edge rows are stored, so the skeleton can never drift
// from the refs it is computed from, and every consumer (the map, a detail
// panel, an agent reading graph_query) sees the same tree without re-deriving it
// itself.
//
// A structural link IS a direct ref: a decision's taskRef attaches it to that
// task; a task's keyResultRef / missionRef attaches it to its parent; a key
// result's missionId attaches it to its mission. Context links remain a separate
// concern — indirect, cross-tree relationships an agent asserts (e.g. a decision
// on task A steering task B) — and are NOT produced here.
type StructuralEdgeResolver interface {
	// Edges returns every structural edge incident to the focal node, in both
	// directions: the focal node's edge up to its parent, and the edges down
	// from each of its direct children.
	Edges(ctx context.Context, projectID string, focal domain.GraphNodeCore) ([]domain.GraphEdge, error)
}

// structuralTaskReader / structuralDecisionReader / structuralKeyResultReader
// are the narrow read slices the resolver needs. The concrete task, decision and
// mission services satisfy them; tests use stubs.
type structuralTaskReader interface {
	List(ctx context.Context, filter taskdomain.TaskFilter) ([]taskdomain.Task, error)
}

type structuralDecisionReader interface {
	List(ctx context.Context, filter decisiondomain.DecisionFilter) ([]decisiondomain.Decision, error)
}

type structuralKeyResultReader interface {
	ListKeyResults(ctx context.Context, missionID missiondomain.MissionID) ([]krdomain.KeyResult, error)
}

const structuralChildScanLimit = 1000

type structuralResolver struct {
	tasks     structuralTaskReader
	decisions structuralDecisionReader
	keys      structuralKeyResultReader
}

// NewStructuralResolver builds the resolver from the same services that back the
// hydrators. Any reader may be nil; the resolver simply skips the edges it can't
// compute (so a partially-wired graph still returns the structure it can).
func NewStructuralResolver(tasks structuralTaskReader, decisions structuralDecisionReader, keys structuralKeyResultReader) StructuralEdgeResolver {
	return structuralResolver{tasks: tasks, decisions: decisions, keys: keys}
}

func (r structuralResolver) Edges(ctx context.Context, projectID string, focal domain.GraphNodeCore) ([]domain.GraphEdge, error) {
	focalType := normalizeNodeType(focal.Type)
	focalID := strings.TrimSpace(focal.ID)
	if focalID == "" {
		return nil, nil
	}
	edges := make([]domain.GraphEdge, 0, 4)
	if up := r.upwardEdge(focalType, focalID, focal.Fields); up != nil {
		edges = append(edges, *up)
	}
	down, err := r.downwardEdges(ctx, projectID, focalType, focalID)
	if err != nil {
		return nil, err
	}
	edges = append(edges, down...)
	return edges, nil
}

// upwardEdge returns the focal node's single edge to its most-specific structural
// parent, computed from the refs the hydrator already put in Fields. "Most
// specific" mirrors the planning hierarchy: a decision attaches to its task if it
// has one, else its key result, else its mission; a task attaches to its key
// result if it has one, else its mission directly.
func (r structuralResolver) upwardEdge(focalType, focalID string, fields map[string]any) *domain.GraphEdge {
	switch focalType {
	case domain.NodeTypeTask:
		if kr := fieldString(fields, "keyResultRef"); kr != "" {
			return ptr(structuralEdge(domain.NodeTypeTask, focalID, domain.NodeTypeKeyResult, kr, domain.EdgeTypeServes, "task serves key result"))
		}
		if mission := fieldString(fields, "missionRef"); mission != "" {
			return ptr(structuralEdge(domain.NodeTypeTask, focalID, domain.NodeTypeMission, mission, domain.EdgeTypeServes, "task serves mission"))
		}
	case domain.NodeTypeDecision:
		if task := fieldString(fields, "taskRef"); task != "" {
			return ptr(structuralEdge(domain.NodeTypeDecision, focalID, domain.NodeTypeTask, task, domain.EdgeTypeMadeDuring, "decision made during task"))
		}
		if kr := fieldString(fields, "keyResultRef"); kr != "" {
			return ptr(structuralEdge(domain.NodeTypeDecision, focalID, domain.NodeTypeKeyResult, kr, domain.EdgeTypeServes, "decision serves key result"))
		}
		if mission := fieldString(fields, "missionRef"); mission != "" {
			return ptr(structuralEdge(domain.NodeTypeDecision, focalID, domain.NodeTypeMission, mission, domain.EdgeTypeServes, "decision serves mission"))
		}
	case domain.NodeTypeKeyResult:
		if mission := fieldString(fields, "missionId"); mission != "" {
			return ptr(structuralEdge(domain.NodeTypeKeyResult, focalID, domain.NodeTypeMission, mission, domain.EdgeTypeServes, "key result serves mission"))
		}
	}
	return nil
}

// downwardEdges returns the edges from the focal node's direct children up to it.
// Each child is attached only where the focal node is its most-specific parent,
// so the skeleton stays a tree (a decision under a task does not also hang off
// that task's mission).
func (r structuralResolver) downwardEdges(ctx context.Context, projectID, focalType, focalID string) ([]domain.GraphEdge, error) {
	switch focalType {
	case domain.NodeTypeTask:
		return r.decisionChildren(ctx, projectID, func(d decisiondomain.Decision) bool {
			return strings.EqualFold(strings.TrimSpace(d.TaskRef), focalID)
		}, domain.NodeTypeTask, focalID, domain.EdgeTypeMadeDuring, "decision made during task")
	case domain.NodeTypeKeyResult:
		edges, err := r.taskChildren(ctx, projectID, func(t taskdomain.Task) bool {
			return strings.EqualFold(strings.TrimSpace(t.KeyResultRef), focalID)
		}, domain.NodeTypeKeyResult, focalID, "task serves key result")
		if err != nil {
			return nil, err
		}
		decEdges, err := r.decisionChildren(ctx, projectID, func(d decisiondomain.Decision) bool {
			return strings.TrimSpace(d.TaskRef) == "" && strings.EqualFold(strings.TrimSpace(d.KeyResultRef), focalID)
		}, domain.NodeTypeKeyResult, focalID, domain.EdgeTypeServes, "decision serves key result")
		if err != nil {
			return nil, err
		}
		return append(edges, decEdges...), nil
	case domain.NodeTypeMission:
		return r.missionChildren(ctx, projectID, focalID)
	}
	return nil, nil
}

func (r structuralResolver) missionChildren(ctx context.Context, projectID, missionID string) ([]domain.GraphEdge, error) {
	edges := make([]domain.GraphEdge, 0, 8)
	if r.keys != nil {
		krs, err := r.keys.ListKeyResults(ctx, missiondomain.MissionID(missionID))
		if err != nil {
			return nil, fmt.Errorf("structural: list key results for mission %s: %w", missionID, err)
		}
		for _, kr := range krs {
			edges = append(edges, structuralEdge(domain.NodeTypeKeyResult, strings.TrimSpace(string(kr.ID)), domain.NodeTypeMission, missionID, domain.EdgeTypeServes, "key result serves mission"))
		}
	}
	taskEdges, err := r.taskChildren(ctx, projectID, func(t taskdomain.Task) bool {
		return strings.TrimSpace(t.KeyResultRef) == "" && strings.EqualFold(taskMissionRef(t), missionID)
	}, domain.NodeTypeMission, missionID, "task serves mission")
	if err != nil {
		return nil, err
	}
	edges = append(edges, taskEdges...)
	decEdges, err := r.decisionChildren(ctx, projectID, func(d decisiondomain.Decision) bool {
		return strings.TrimSpace(d.TaskRef) == "" && strings.TrimSpace(d.KeyResultRef) == "" && strings.EqualFold(strings.TrimSpace(d.MissionRef), missionID)
	}, domain.NodeTypeMission, missionID, domain.EdgeTypeServes, "decision serves mission")
	if err != nil {
		return nil, err
	}
	return append(edges, decEdges...), nil
}

func (r structuralResolver) taskChildren(ctx context.Context, projectID string, match func(taskdomain.Task) bool, parentType, parentID, rationale string) ([]domain.GraphEdge, error) {
	if r.tasks == nil {
		return nil, nil
	}
	tasks, err := r.tasks.List(ctx, taskdomain.TaskFilter{
		ProjectID: types.ProjectID(strings.TrimSpace(projectID)),
		Limit:     structuralChildScanLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("structural: list tasks for %s %s: %w", parentType, parentID, err)
	}
	edges := make([]domain.GraphEdge, 0, len(tasks))
	for _, task := range tasks {
		if !match(task) {
			continue
		}
		edges = append(edges, structuralEdge(domain.NodeTypeTask, strings.TrimSpace(string(task.ID)), parentType, parentID, domain.EdgeTypeServes, rationale))
	}
	return edges, nil
}

func (r structuralResolver) decisionChildren(ctx context.Context, projectID string, match func(decisiondomain.Decision) bool, parentType, parentID, edgeType, rationale string) ([]domain.GraphEdge, error) {
	if r.decisions == nil {
		return nil, nil
	}
	decisions, err := r.decisions.List(ctx, decisiondomain.DecisionFilter{
		ProjectID: strings.TrimSpace(projectID),
		Limit:     structuralChildScanLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("structural: list decisions for %s %s: %w", parentType, parentID, err)
	}
	edges := make([]domain.GraphEdge, 0, len(decisions))
	for _, decision := range decisions {
		if !match(decision) {
			continue
		}
		edges = append(edges, structuralEdge(domain.NodeTypeDecision, strings.TrimSpace(string(decision.ID)), parentType, parentID, edgeType, rationale))
	}
	return edges, nil
}

func structuralEdge(sourceType, sourceID, targetType, targetID, edgeType, rationale string) domain.GraphEdge {
	return domain.GraphEdge{
		SourceType: sourceType,
		SourceID:   strings.TrimSpace(sourceID),
		TargetType: targetType,
		TargetID:   strings.TrimSpace(targetID),
		EdgeType:   edgeType,
		Confidence: 1,
		Rationale:  rationale,
		CreatedBy:  "graph_structure",
		Origin:     "reference",
		Source:     "reference",
		Manual:     false,
	}
}

func fieldString(fields map[string]any, key string) string {
	if fields == nil {
		return ""
	}
	value, _ := fields[key].(string)
	return strings.TrimSpace(value)
}

func ptr[T any](v T) *T { return &v }
