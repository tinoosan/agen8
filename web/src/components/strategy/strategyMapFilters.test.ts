import { describe, expect, it } from 'vitest'
import type { Node, Edge } from '@xyflow/react'
import {
  computeInMotionFilter,
  computeBlockedFilter,
  computeDoneFilter,
  computeDecisionsFilter,
  computeTraceFilter,
} from './strategyMapFilters'

/* ── Fixture builders ─────────────────────────────────────────────────────
 * nodeStatus() reads status by type: mission→data.mission.status,
 * keyResult→data.kr.status, task→data.task.status. Decisions carry no status.
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

/** Structural tree edge (mission→KR→task lineage). */
function treeEdge(source: string, target: string): Edge {
  return { id: `s:${source}->${target}`, source, target, type: 'statusEdge' }
}
/** Context link edge carrying its semantic type. */
function contextEdge(source: string, target: string, edgeType: string): Edge {
  return { id: `c:${source}->${target}`, source, target, type: 'contextEdge', data: { edgeType } }
}

/**
 * A small but representative graph:
 *   M (active mission)
 *    └─ K1 (on_track KR)
 *        ├─ T1 (active task)        ← a decision was made during this task
 *        ├─ T2 (succeeded task)     ← finished
 *        └─ T3 (blocked task)
 *   D (decision) --made_during--> T1
 *   T3 --blocked_by--> T1
 */
function buildGraph(): { nodes: Node[]; edges: Edge[] } {
  const nodes = [
    mission('M', 'active'),
    kr('K1', 'on_track'),
    task('T1', 'active'),
    task('T2', 'succeeded'),
    task('T3', 'blocked'),
    decision('D'),
  ]
  const edges = [
    treeEdge('M', 'K1'),
    treeEdge('K1', 'T1'),
    treeEdge('K1', 'T2'),
    treeEdge('K1', 'T3'),
    contextEdge('D', 'T1', 'made_during'),
    contextEdge('T3', 'T1', 'blocked_by'),
  ]
  return { nodes, edges }
}

describe('computeInMotionFilter', () => {
  it('matches active missions, on-track KRs, and active/in-review tasks', () => {
    const { nodes, edges } = buildGraph()
    const result = computeInMotionFilter(nodes, edges)
    expect(result.nodeIds.has('M')).toBe(true)
    expect(result.nodeIds.has('K1')).toBe(true)
    expect(result.nodeIds.has('T1')).toBe(true)
  })

  it('counts only directly matched nodes (excludes structural parents pulled in)', () => {
    const { nodes, edges } = buildGraph()
    const result = computeInMotionFilter(nodes, edges)
    // T1 (active), K1 (on_track), M (active) match directly = 3.
    expect(result.matchCount).toBe(3)
  })

  it('does NOT pull a decision into the match set via its context link (regression)', () => {
    // D is `made_during` T1. Parent inclusion must follow structural edges only,
    // so a decision touching a live task should never light up under In Motion.
    const { nodes, edges } = buildGraph()
    const result = computeInMotionFilter(nodes, edges)
    expect(result.nodeIds.has('D')).toBe(false)
  })

  it('excludes finished tasks', () => {
    const { nodes, edges } = buildGraph()
    const result = computeInMotionFilter(nodes, edges)
    expect(result.nodeIds.has('T2')).toBe(false)
  })

  it('keeps a matched leaf connected by including its structural parent', () => {
    // A lone in-motion task should drag in its KR so it isn't floating.
    const nodes = [kr('K1', 'draft'), task('T1', 'active')]
    const edges = [treeEdge('K1', 'T1')]
    const result = computeInMotionFilter(nodes, edges)
    expect(result.nodeIds.has('T1')).toBe(true)
    expect(result.nodeIds.has('K1')).toBe(true)
    expect(result.matchCount).toBe(1) // only T1 matched directly
  })
})

describe('computeDoneFilter', () => {
  it('highlights finished work rather than hiding it (returns a match set)', () => {
    const { nodes, edges } = buildGraph()
    const result = computeDoneFilter(nodes, edges)
    // T2 is succeeded → it must be IN the highlight set, not removed.
    expect(result.nodeIds.has('T2')).toBe(true)
  })

  it('matches completed/archived missions, completed/dropped KRs, succeeded/canceled tasks', () => {
    const nodes = [
      mission('Mc', 'completed'),
      mission('Ma', 'archived'),
      kr('Kc', 'completed'),
      kr('Kd', 'dropped'),
      task('Ts', 'succeeded'),
      task('Tx', 'canceled'),
    ]
    const result = computeDoneFilter(nodes, [])
    for (const id of ['Mc', 'Ma', 'Kc', 'Kd', 'Ts', 'Tx']) {
      expect(result.nodeIds.has(id)).toBe(true)
    }
    expect(result.matchCount).toBe(6)
  })

  it('does not highlight live work', () => {
    const { nodes, edges } = buildGraph()
    const result = computeDoneFilter(nodes, edges)
    expect(result.nodeIds.has('T1')).toBe(false) // active
    expect(result.nodeIds.has('T3')).toBe(false) // blocked
  })

  it('includes the structural parent of a finished leaf so it stays connected', () => {
    const { nodes, edges } = buildGraph()
    const result = computeDoneFilter(nodes, edges)
    expect(result.nodeIds.has('K1')).toBe(true) // parent of succeeded T2
  })

  it('does not pull a decision in via context links', () => {
    const { nodes, edges } = buildGraph()
    const result = computeDoneFilter(nodes, edges)
    expect(result.nodeIds.has('D')).toBe(false)
  })
})

describe('computeBlockedFilter', () => {
  it('matches blocked tasks and both endpoints of a blocked_by link', () => {
    const { nodes, edges } = buildGraph()
    const result = computeBlockedFilter(nodes, edges)
    expect(result.nodeIds.has('T3')).toBe(true) // blocked task + blocked_by source
    expect(result.nodeIds.has('T1')).toBe(true) // blocked_by target
  })

  it('does not pull a decision in via its made_during link', () => {
    const { nodes, edges } = buildGraph()
    const result = computeBlockedFilter(nodes, edges)
    expect(result.nodeIds.has('D')).toBe(false)
  })
})

describe('computeDecisionsFilter (reference model — show these)', () => {
  it('highlights decisions and the far end of their context links', () => {
    const { nodes, edges } = buildGraph()
    const result = computeDecisionsFilter(nodes, edges)
    expect(result.nodeIds.has('D')).toBe(true)
    expect(result.nodeIds.has('T1')).toBe(true) // made_during target
    expect(result.matchCount).toBe(1) // one decision
  })
})

describe('computeTraceFilter (alternating structural ↔ context)', () => {
  /* buildGraph() adjacency:
   *   structural:  M ── K1 ── {T1, T2, T3}
   *   context:     D ── T1,  T3 ── T1
   * From T1 the stepper alternates phases:
   *   step 1 (structural ring 1): K1
   *   step 2 (context pass 1):    D, T3
   *   step 3 (structural ring 2): M, T2
   */

  it('step 1 reveals the structural ring only — no context links yet', () => {
    const { edges } = buildGraph()
    const result = computeTraceFilter('T1', edges, 1)
    expect(result.nodeIds.has('T1')).toBe(true) // the selected node
    expect(result.nodeIds.has('K1')).toBe(true) // straight edge to immediate neighbour
    // Context-linked nodes wait for the next (even) step.
    expect(result.nodeIds.has('D')).toBe(false)
    expect(result.nodeIds.has('T3')).toBe(false)
    // 2nd structural hop also waits.
    expect(result.nodeIds.has('M')).toBe(false)
    expect(result.nodeIds.has('T2')).toBe(false)
  })

  it('step 2 layers in context links off everything shown so far', () => {
    const { edges } = buildGraph()
    const result = computeTraceFilter('T1', edges, 2)
    expect(result.nodeIds.has('D')).toBe(true)  // T1 —made_during→ D
    expect(result.nodeIds.has('T3')).toBe(true) // T1 —blocked_by→ T3
    // The 2nd structural hop is still not revealed (that's step 3).
    expect(result.nodeIds.has('M')).toBe(false)
    expect(result.nodeIds.has('T2')).toBe(false)
  })

  it('step 3 grows the structural skeleton by another hop', () => {
    const { edges } = buildGraph()
    const result = computeTraceFilter('T1', edges, 3)
    expect(result.nodeIds.has('M')).toBe(true)  // T1→K1→M
    expect(result.nodeIds.has('T2')).toBe(true) // T1→K1→T2
  })

  it('step 0 lights only the selected node (no edges)', () => {
    const { edges } = buildGraph()
    const result = computeTraceFilter('T1', edges, 0)
    expect(result.nodeIds.size).toBe(1)
    expect(result.nodeIds.has('T1')).toBe(true)
    expect(result.edgeIds.size).toBe(0)
  })

  it('marks only STRUCTURAL edges as direct — never context links (crash regression)', () => {
    // A direct context edge spawns a label portal + infinite animation; lighting
    // many at once crashes the render process. Trace must keep context ambient.
    const { edges } = buildGraph()
    for (const depth of [1, 2, 3, 4, 5, 6]) {
      const result = computeTraceFilter('T1', edges, depth)
      for (const id of result.directEdgeIds ?? []) {
        expect(id.startsWith('c:')).toBe(false)
      }
    }
    // And the structural path edge IS direct from step 1.
    expect(computeTraceFilter('T1', edges, 1).directEdgeIds?.has('s:K1->T1')).toBe(true)
  })

  it('cluster edgeIds include revealed context links even though they are ambient', () => {
    const { edges } = buildGraph()
    const result = computeTraceFilter('T1', edges, 2)
    // Context links are in edgeIds (so they render + dim correctly)...
    expect(result.edgeIds.has('c:D->T1')).toBe(true)
    expect(result.edgeIds.has('c:T3->T1')).toBe(true)
    // ...but NOT in directEdgeIds (ambient, no animation).
    expect(result.directEdgeIds?.has('c:D->T1')).toBe(false)
    expect(result.directEdgeIds?.has('c:T3->T1')).toBe(false)
  })

  it('matchCount equals the number of revealed nodes', () => {
    const { edges } = buildGraph()
    expect(computeTraceFilter('T1', edges, 1).matchCount).toBe(2) // T1,K1
    expect(computeTraceFilter('T1', edges, 2).matchCount).toBe(4) // + D,T3
    expect(computeTraceFilter('T1', edges, 3).matchCount).toBe(6) // + M,T2
  })
})
