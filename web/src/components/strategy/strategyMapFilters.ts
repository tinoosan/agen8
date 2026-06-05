import type { Node, Edge } from '@xyflow/react'

export type FilterPreset = 'attention' | 'failed' | 'trace'

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

// ── Attention filter ────────────────────────────────────────────────────

/** Nodes needing human action: blocked/review tasks and input-needed decisions. */
export function computeAttentionFilter(
  nodes: Node[],
  edges: Edge[],
): FilterResult | null {
  const matchedIds = new Set<string>()

  for (const node of nodes) {
    const d = node.data as Record<string, unknown>

    switch (node.type) {
      case 'task': {
        const task = d.task as { status?: string } | undefined
        if (task?.status === 'blocked' || task?.status === 'in_review') {
          matchedIds.add(node.id)
        }
        break
      }
      case 'decision': {
        const decision = d.decision as {
          cancelled?: boolean
          questions?: { id: string; blocking?: boolean }[]
          answers?: { questionId: string; selectedOption?: string; selectedOptions?: string[]; freeFormText?: string }[]
        } | undefined
        if (!decision || decision.cancelled) break
        const hasBlockingUnanswered = (decision.questions ?? []).some((q) => {
          if (!q.blocking) return false
          return !(decision.answers ?? []).some(
            (a) =>
              a.questionId === q.id &&
              Boolean(
                a.selectedOption?.trim() ||
                  a.freeFormText?.trim() ||
                  (a.selectedOptions ?? []).some((opt) => opt.trim() !== ''),
              ),
          )
        })
        if (hasBlockingUnanswered) matchedIds.add(node.id)
        break
      }
      case 'plan': {
        const plan = d.plan as { status?: string } | undefined
        if (plan?.status === 'pending_approval') matchedIds.add(node.id)
        break
      }
    }
  }

  const matchCount = matchedIds.size
  if (matchCount > 0) includeParents(matchedIds, edges)
  return { nodeIds: matchedIds, edgeIds: connectedEdges(matchedIds, edges), matchCount }
}

// ── Failed & Cancelled filter ───────────────────────────────────────────

/** Dead/failed work: failed tasks, cancelled decisions, and abandoned plans. */
export function computeFailedFilter(
  nodes: Node[],
  edges: Edge[],
): FilterResult | null {
  const matchedIds = new Set<string>()

  for (const node of nodes) {
    const d = node.data as Record<string, unknown>

    switch (node.type) {
      case 'task': {
        const task = d.task as { status?: string } | undefined
        if (task?.status === 'failed' || task?.status === 'canceled') {
          matchedIds.add(node.id)
        }
        break
      }
      case 'decision': {
        const decision = d.decision as { cancelled?: boolean } | undefined
        if (decision?.cancelled) matchedIds.add(node.id)
        break
      }
      case 'plan': {
        const plan = d.plan as { status?: string } | undefined
        if (plan?.status === 'abandoned') matchedIds.add(node.id)
        break
      }
    }
  }

  const matchCount = matchedIds.size
  if (matchCount > 0) includeParents(matchedIds, edges)
  return { nodeIds: matchedIds, edgeIds: connectedEdges(matchedIds, edges), matchCount }
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
