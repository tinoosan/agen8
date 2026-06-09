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

// The second arg feeds the focus cursor (effectiveFocusNodeId) — focus is
// reachable with the panel closed.
function lens(activeFilter: FilterPreset | null, focusNodeId: string | null = null) {
  return renderHook(() =>
    useStrategyMapLenses({
      activeFilter,
      displayNodes: nodes,
      displayEdges: edges,
      effectiveFocusNodeId: focusNodeId,
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

  it('no filter + no focus leaves direct/cluster sets null (full graph, no dimming)', () => {
    const result = lens(null)
    expect(result.directEdgeIds).toBeNull()
    expect(result.clusterNodeIds).toBeNull()
  })

  it('plain node focus marks its structural edge direct but keeps a touching context link ambient (iPad crash regression)', () => {
    // Regression: focusing a node used to flag EVERY touching edge — including
    // context links — as direct. A direct context edge spawns a label portal +
    // an infinite dash animation; a node with many context links floods the
    // render process and crashes iOS WebKit on select. Only the structural edge
    // may be direct; the context link stays ambient (highlighted, not animated).
    const result = lens(null, 'T1')
    expect(result.directEdgeIds?.has('s:K1->T1')).toBe(true)   // structural = direct
    expect(result.directEdgeIds?.has('cl:D->T1')).toBe(false)  // context never direct
    // The context link is still revealed (its node + edge join the neighborhood).
    expect(result.clusterNodeIds?.has('D')).toBe(true)
    expect(result.clusterEdgeIds?.has('cl:D->T1')).toBe(true)
  })
})
