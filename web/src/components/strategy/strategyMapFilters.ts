import type { Node, Edge } from '@xyflow/react'

export type FilterPreset = 'in_motion' | 'blocked' | 'done' | 'decisions' | 'trace'

export interface FilterResult {
  nodeIds: ReadonlySet<string>
  edgeIds: ReadonlySet<string>
  /** Count of directly matched nodes (excludes context parents). */
  matchCount: number
}

// ── Helpers ─────────────────────────────────────────────────────────────

/** Add 1-hop parent nodes so matched leaves don't float disconnected. */
function includeParents(matchedIds: Set<string>, edges: Edge[]): void {
  const snapshot = [...matchedIds]
  for (const edge of edges) {
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
  if (matchCount > 0) includeParents(matchedIds, edges)
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
  if (matchCount > 0) includeParents(matchedIds, edges)
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

// ── Done lens (declutter / hide) ─────────────────────────────────────────

const DONE_STATUS: Record<string, ReadonlySet<string>> = {
  mission: new Set(['completed', 'archived']),
  keyResult: new Set(['completed', 'dropped']),
  task: new Set(['succeeded', 'canceled']),
}

/** Finished work to hide so the live map is uncluttered. Returns the set of
 *  node ids to remove. A done node is kept only if it's an ancestor of some
 *  still-live node, so hiding never orphans unfinished work below it. */
export function computeDoneFilter(nodes: Node[], structuralEdges: Edge[]): FilterResult {
  const doneIds = new Set<string>()
  for (const node of nodes) {
    const status = nodeStatus(node)
    if (node.type && status && DONE_STATUS[node.type]?.has(status)) doneIds.add(node.id)
  }

  // Walk up from every live node, marking ancestors that must stay visible.
  const parentsOf = new Map<string, string[]>()
  for (const edge of structuralEdges) {
    if (!parentsOf.has(edge.target)) parentsOf.set(edge.target, [])
    parentsOf.get(edge.target)!.push(edge.source)
  }
  const keep = new Set<string>()
  const queue: string[] = []
  for (const node of nodes) {
    if (!doneIds.has(node.id)) queue.push(node.id)
  }
  while (queue.length > 0) {
    const current = queue.shift()!
    for (const parent of parentsOf.get(current) ?? []) {
      if (!keep.has(parent)) {
        keep.add(parent)
        queue.push(parent)
      }
    }
  }

  const hiddenIds = new Set<string>()
  for (const id of doneIds) {
    if (!keep.has(id)) hiddenIds.add(id)
  }
  return { nodeIds: hiddenIds, edgeIds: new Set(), matchCount: hiddenIds.size }
}

// ── Path trace filter ───────────────────────────────────────────────────

/** Directed lineage trace: the direct ancestor path UP to the root,
 *  plus any descendants DOWN from the selected node only.
 *  Optionally expands via context links (informed_by) up to `contextDepth` hops. */
export function computeTraceFilter(
  selectedNodeId: string,
  structuralEdges: Edge[],
  allEdges: Edge[],
  contextDepth: number,
): FilterResult {
  const childrenOf = new Map<string, string[]>()
  const parentsOf = new Map<string, string[]>()
  for (const edge of structuralEdges) {
    if (!childrenOf.has(edge.source)) childrenOf.set(edge.source, [])
    childrenOf.get(edge.source)!.push(edge.target)
    if (!parentsOf.has(edge.target)) parentsOf.set(edge.target, [])
    parentsOf.get(edge.target)!.push(edge.source)
  }

  const visited = new Set<string>([selectedNodeId])

  // Walk UP: direct ancestor path
  const upQueue = [selectedNodeId]
  while (upQueue.length > 0) {
    const current = upQueue.shift()!
    const parents = parentsOf.get(current)
    if (!parents) continue
    for (const parent of parents) {
      if (!visited.has(parent)) {
        visited.add(parent)
        upQueue.push(parent)
      }
    }
  }

  // Walk DOWN: descendants of the selected node only
  const downQueue = [selectedNodeId]
  while (downQueue.length > 0) {
    const current = downQueue.shift()!
    const children = childrenOf.get(current)
    if (!children) continue
    for (const child of children) {
      if (!visited.has(child)) {
        visited.add(child)
        downQueue.push(child)
      }
    }
  }

  // Expand via context links (informed_by) up to contextDepth hops
  if (contextDepth > 0) {
    const contextAdj = new Map<string, string[]>()
    for (const edge of allEdges) {
      if (edge.type === 'contextEdge') {
        if (!contextAdj.has(edge.source)) contextAdj.set(edge.source, [])
        if (!contextAdj.has(edge.target)) contextAdj.set(edge.target, [])
        contextAdj.get(edge.source)!.push(edge.target)
        contextAdj.get(edge.target)!.push(edge.source)
      }
    }

    // BFS from the structural lineage nodes, limited to contextDepth hops
    let frontier = [...visited]
    for (let hop = 0; hop < contextDepth; hop++) {
      const nextFrontier: string[] = []
      for (const nodeId of frontier) {
        const neighbors = contextAdj.get(nodeId)
        if (!neighbors) continue
        for (const neighbor of neighbors) {
          if (!visited.has(neighbor)) {
            visited.add(neighbor)
            nextFrontier.push(neighbor)
          }
        }
      }
      frontier = nextFrontier
      if (frontier.length === 0) break
    }
  }

  // Include edges from ALL edges (structural + context) where both endpoints are visited
  return { nodeIds: visited, edgeIds: connectedEdges(visited, allEdges), matchCount: visited.size }
}
