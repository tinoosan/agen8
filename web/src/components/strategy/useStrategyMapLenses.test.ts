import { describe, expect, it } from 'vitest'
import { renderHook } from '@testing-library/react'
import type { Node, Edge } from '@xyflow/react'
import { useStrategyMapLenses } from './useStrategyMapLenses'
import type { FilterPreset } from './strategyMapFilters'

/* ── Fixtures ──────────────────────────────────────────────────────────────
 * A mission with a KR and a finished task, plus a decision wired to the task
 * by a context link. Enough to exercise highlight filters and their edges.
 */
function mission(id: string, status: string): Node {
  return { id, type: 'mission', position: { x: 0, y: 0 }, data: { mission: { status } } }
}
function kr(id: string, status: string): Node {
  return { id, type: 'keyResult', position: { x: 0, y: 0 }, data: { kr: { status } } }
}
function task(id: string, status: string): Node {
  return { id, type: 'task', position: { x: 0, y: 0 }, data: { task: { status } } }
}
function decision(id: string): Node {
  return { id, type: 'decision', position: { x: 0, y: 0 }, data: {} }
}
function treeEdge(source: string, target: string): Edge {
  return { id: `s:${source}->${target}`, source, target, type: 'statusEdge' }
}
function contextEdge(source: string, target: string, edgeType: string): Edge {
  return { id: `cl:${source}->${target}`, source, target, type: 'contextEdge', data: { edgeType } }
}

const nodes: Node[] = [
  mission('M', 'completed'),
  kr('K1', 'completed'),
  task('T1', 'succeeded'),
  decision('D'),
]
const edges: Edge[] = [
  treeEdge('M', 'K1'),
  treeEdge('K1', 'T1'),
  contextEdge('D', 'T1', 'made_during'),
]

function lens(activeFilter: FilterPreset | null, selectedNodeId: string | null = null, contextDepth = 0) {
  return renderHook(() =>
    useStrategyMapLenses({
      activeFilter,
      displayNodes: nodes,
      displayEdges: edges,
      selectedNodeId,
      contextDepth,
      effectiveFocusNodeId: null,
    }),
  ).result.current
}

describe('useStrategyMapLenses — set-highlight filters do not flood "direct" edges', () => {
  // Regression: flagging every matched edge as "direct" makes each edge render
  // a label portal + (for context links) an infinite dash animation. With a
  // large decision web that crashes the browser render process. Highlight
  // filters must highlight via opacity only — nothing should be "direct".
  for (const preset of ['decisions', 'done', 'in_motion', 'blocked'] as const) {
    it(`'${preset}' marks no edges as direct but still highlights via clusterEdgeIds`, () => {
      const result = lens(preset)
      expect(result.directEdgeIds?.size ?? 0).toBe(0)
      // The match set is still surfaced for dimming the rest.
      expect(result.clusterNodeIds).not.toBeNull()
    })
  }

  it("'decisions' still highlights the decision's context edge as ambient (cluster), not direct", () => {
    const result = lens('decisions')
    // The made_during link to the decision is in the cluster set (highlighted),
    // but not in the direct set (no label/animation flood).
    expect(result.clusterEdgeIds?.has('cl:D->T1')).toBe(true)
    expect(result.directEdgeIds?.has('cl:D->T1')).toBe(false)
  })

  it("trace keeps its direct edges so the ring path shows labels + flow", () => {
    // Trace from the task at depth 1 reveals its immediate ring (K1 up, D via
    // context link); those edges should be direct (bold + flow), not ambient.
    const result = lens('trace', 'T1', 1)
    expect((result.directEdgeIds?.size ?? 0)).toBeGreaterThan(0)
  })

  it('trace reveals only the first ring at depth 1, the full path at depth 2', () => {
    // Graph: M → K1 → T1, plus D —made_during→ T1. From T1, K1 and D are 1 hop;
    // M is 2 hops. Depth 1 must NOT yet include M (the old code lit the whole
    // lineage at once — this pins the progressive reveal).
    const ring1 = lens('trace', 'T1', 1)
    expect(ring1.clusterNodeIds?.has('K1')).toBe(true)
    expect(ring1.clusterNodeIds?.has('D')).toBe(true)
    expect(ring1.clusterNodeIds?.has('M')).toBe(false)

    const ring2 = lens('trace', 'T1', 2)
    expect(ring2.clusterNodeIds?.has('M')).toBe(true)
  })

  it('no filter + no focus leaves direct/cluster sets null (full graph, no dimming)', () => {
    const result = lens(null)
    expect(result.directEdgeIds).toBeNull()
    expect(result.clusterNodeIds).toBeNull()
  })
})
