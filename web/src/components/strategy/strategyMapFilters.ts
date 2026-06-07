import type { Node, Edge } from '@xyflow/react'

export type FilterPreset = 'in_motion' | 'blocked' | 'done' | 'decisions' | 'trace'

export interface FilterResult {
  nodeIds: ReadonlySet<string>
  edgeIds: ReadonlySet<string>
  /** Count of directly matched nodes (excludes context parents). */
  matchCount: number
}

// ── Helpers ─────────────────────────────────────────────────────────────

/** Structural tree edges only (mission→KR→task), excluding context links.
 *  Parent inclusion must follow the tree, never context relationships —
 *  otherwise a decision linked `made_during`/`informed_by` a matched task gets
 *  pulled into the match set and lights up under filters like "In Motion". */
function structuralEdges(edges: Edge[]): Edge[] {
  return edges.filter((e) => e.type === 'statusEdge')
}

/** Add 1-hop structural parent nodes so matched leaves don't float
 *  disconnected. Pass structural edges only — context links are not lineage. */
function includeParents(matchedIds: Set<string>, structural: Edge[]): void {
  const snapshot = [...matchedIds]
  for (const edge of structural) {
    if (snapshot.some((id) => id === edge.target)) {
      matchedIds.add(edge.source)
    }
  }
}

/** Build edge set from edges where both endpoints are in nodeIds. */
function connectedEdges(nodeIds: Set<string>, edges: Edge[]): Set<string> {
  const edgeIds = new Set<string>()
  for (const edge of edges) {
    if (nodeIds.has(edge.source) && nodeIds.has(edge.target)) {
      edgeIds.add(edge.id)
    }
  }
  return edgeIds
}

/** Read a node's status string regardless of entity type. */
function nodeStatus(node: Node): string | undefined {
  const d = node.data as Record<string, unknown>
  switch (node.type) {
    case 'mission': return (d.mission as { status?: string } | undefined)?.status
    case 'keyResult': return (d.kr as { status?: string } | undefined)?.status
    case 'task': return (d.task as { status?: string } | undefined)?.status
    default: return undefined
  }
}

/** Endpoints of context edges whose semantic type matches `edgeType`. */
function contextEdgeEndpoints(edges: Edge[], edgeType: string): Set<string> {
  const ids = new Set<string>()
  for (const edge of edges) {
    if (edge.type !== 'contextEdge') continue
    if ((edge.data as { edgeType?: string } | undefined)?.edgeType !== edgeType) continue
    ids.add(edge.source)
    ids.add(edge.target)
  }
  return ids
}

// ── In Motion lens ───────────────────────────────────────────────────────

/** Live work: active missions, on-track KRs, and tasks being worked or
 *  reviewed. Highlights the slice of the map that's actually moving. */
export function computeInMotionFilter(nodes: Node[], edges: Edge[]): FilterResult {
  const matchedIds = new Set<string>()
  for (const node of nodes) {
    const status = nodeStatus(node)
    if (
      (node.type === 'mission' && status === 'active')
      || (node.type === 'keyResult' && status === 'on_track')
      || (node.type === 'task' && (status === 'active' || status === 'in_review'))
    ) {
      matchedIds.add(node.id)
    }
  }
  const matchCount = matchedIds.size
  if (matchCount > 0) includeParents(matchedIds, structuralEdges(edges))
  return { nodeIds: matchedIds, edgeIds: connectedEdges(matchedIds, edges), matchCount }
}

// ── Blocked lens ─────────────────────────────────────────────────────────

/** Stuck work: blocked tasks, at-risk KRs, paused missions, plus anything
 *  on either end of a `blocked_by` context link — the full snag surface. */
export function computeBlockedFilter(nodes: Node[], edges: Edge[]): FilterResult {
  const matchedIds = new Set<string>()
  for (const node of nodes) {
    const status = nodeStatus(node)
    if (
      (node.type === 'task' && status === 'blocked')
      || (node.type === 'keyResult' && status === 'at_risk')
      || (node.type === 'mission' && status === 'paused')
    ) {
      matchedIds.add(node.id)
    }
  }
  for (const id of contextEdgeEndpoints(edges, 'blocked_by')) matchedIds.add(id)

  const matchCount = matchedIds.size
  if (matchCount > 0) includeParents(matchedIds, structuralEdges(edges))
  return { nodeIds: matchedIds, edgeIds: connectedEdges(matchedIds, edges), matchCount }
}

// ── Decisions lens ───────────────────────────────────────────────────────

/** Reasoning layer: every decision node plus the nodes its context links
 *  touch (what informed it, what it produced). Only the context edges that
 *  actually touch a decision are lit — structural tree edges stay dim, so the
 *  lens surfaces the "why" rather than re-lighting the whole map. */
export function computeDecisionsFilter(nodes: Node[], edges: Edge[]): FilterResult {
  const decisionIds = new Set<string>()
  for (const node of nodes) {
    if (node.type === 'decision') decisionIds.add(node.id)
  }

  // Light each decision plus the far end of its context links, and mark only
  // those context edges as direct. We deliberately skip structural edges and
  // context edges between two non-decision nodes.
  const matchedIds = new Set<string>(decisionIds)
  const edgeIds = new Set<string>()
  for (const edge of edges) {
    if (edge.type !== 'contextEdge') continue
    if (!decisionIds.has(edge.source) && !decisionIds.has(edge.target)) continue
    edgeIds.add(edge.id)
    matchedIds.add(edge.source)
    matchedIds.add(edge.target)
  }

  return { nodeIds: matchedIds, edgeIds, matchCount: decisionIds.size }
}

// ── Done lens (highlight finished work) ──────────────────────────────────

const DONE_STATUS: Record<string, ReadonlySet<string>> = {
  mission: new Set(['completed', 'archived']),
  keyResult: new Set(['completed', 'dropped']),
  task: new Set(['succeeded', 'canceled']),
}

/** Finished work: completed/archived missions, completed/dropped KRs, and
 *  succeeded/canceled tasks. Highlights the done slice (everything else dims),
 *  the same "show these" model as the Decisions and In Motion lenses. Adds
 *  structural parents so a finished leaf stays connected to its tree. */
export function computeDoneFilter(nodes: Node[], edges: Edge[]): FilterResult {
  const matchedIds = new Set<string>()
  for (const node of nodes) {
    const status = nodeStatus(node)
    if (node.type && status && DONE_STATUS[node.type]?.has(status)) matchedIds.add(node.id)
  }
  const matchCount = matchedIds.size
  if (matchCount > 0) includeParents(matchedIds, structuralEdges(edges))
  return { nodeIds: matchedIds, edgeIds: connectedEdges(matchedIds, edges), matchCount }
}

// ── Path trace filter (progressive rings) ────────────────────────────────

/** Trace-path depth bounds. The trace lens reveals the graph one BFS ring at a
 *  time, so the stepper drives a hop radius rather than a context-only expansion.
 *  Depth 0 (just the selected node) is useless on screen, so the floor is 1 —
 *  selecting a node always shows at least its immediate neighbours. */
export const TRACE_MIN_DEPTH = 1
export const TRACE_MAX_DEPTH = 5
/** Depth applied the moment trace is enabled: light the first ring. */
export const TRACE_INITIAL_DEPTH = 1

/** Progressive ring trace: an undirected BFS outward from the selected node over
 *  the COMBINED graph (structural lineage `statusEdge` + context links
 *  `contextEdge`), revealing every node within `depth` hops. Each +/- step grows
 *  or shrinks the ring by one hop, so a selection lights its immediate
 *  neighbours first and the user walks outward — instead of the entire connected
 *  component lighting up at once (the old "select a mission → whole tree
 *  explodes" behaviour).
 *
 *  Traversal is symmetric and edge-type-agnostic: ancestors, descendants, and
 *  context-linked siblings all appear by hop distance. The visual weight split
 *  (direct/bold vs ambient) is applied downstream in useStrategyMapLenses, not
 *  here. Because the ring is bounded by `depth`, the "direct edges" set stays
 *  small — preserving the Safari label-portal/animation budget. */
export function computeTraceFilter(
  selectedNodeId: string,
  allEdges: Edge[],
  depth: number,
): FilterResult {
  // Undirected adjacency over every edge type.
  const adj = new Map<string, string[]>()
  const link = (a: string, b: string) => {
    if (!adj.has(a)) adj.set(a, [])
    adj.get(a)!.push(b)
  }
  for (const edge of allEdges) {
    link(edge.source, edge.target)
    link(edge.target, edge.source)
  }

  // BFS rings outward, bounded by `depth` hops.
  const visited = new Set<string>([selectedNodeId])
  let frontier = [selectedNodeId]
  for (let hop = 0; hop < depth; hop++) {
    const next: string[] = []
    for (const id of frontier) {
      const neighbors = adj.get(id)
      if (!neighbors) continue
      for (const neighbor of neighbors) {
        if (!visited.has(neighbor)) {
          visited.add(neighbor)
          next.push(neighbor)
        }
      }
    }
    frontier = next
    if (frontier.length === 0) break
  }

  // Include edges (structural + context) where both endpoints are within the ring.
  return { nodeIds: visited, edgeIds: connectedEdges(visited, allEdges), matchCount: visited.size }
}
