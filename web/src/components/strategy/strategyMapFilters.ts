import type { Node, Edge } from '@xyflow/react'

export type FilterPreset = 'in_motion' | 'blocked' | 'done' | 'decisions' | 'trace'

export interface FilterResult {
  nodeIds: ReadonlySet<string>
  edgeIds: ReadonlySet<string>
  /** Subset of edgeIds to render as "direct" (bold path). Only the trace lens
   *  sets this — and only with STRUCTURAL edges, never context links. Context
   *  links rendered "direct" each spawn a label portal + an infinite dash
   *  animation; lighting many at once kills the browser render process
   *  (the Safari "a problem repeatedly occurred" crash). Leaving this undefined
   *  means "no direct edges" (set filters highlight via opacity only). */
  directEdgeIds?: ReadonlySet<string>
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

// ── Path trace filter (progressive rings: structural ↔ context) ──────────

/** Trace-path step bounds. Each press of the stepper is one reveal phase, and
 *  the phases ALTERNATE: odd steps grow the structural skeleton by one hop, even
 *  steps layer in the context links off everything shown so far. Step 0 (just
 *  the selected node) is useless on screen, so the floor is 1 — selecting a node
 *  always shows at least its immediate structural neighbours. The ceiling allows
 *  three structural rings interleaved with three context passes. */
export const TRACE_MIN_DEPTH = 1
export const TRACE_MAX_DEPTH = 6
/** Step applied the moment trace is enabled: the first structural ring. */
export const TRACE_INITIAL_DEPTH = 1

/** Progressive trace, alternating structural ↔ context.
 *
 *  Selecting a node and stepping outward reveals its neighbourhood one phase at
 *  a time instead of lighting the whole connected component at once (the old
 *  "select a mission → everything explodes" behaviour):
 *
 *    step 1  structural ring 1  — straight lineage edges to immediate neighbours
 *    step 2  context pass 1     — context links off everything shown so far
 *    step 3  structural ring 2  — the next hop of straight lineage
 *    step 4  context pass 2     — context links off everything shown so far
 *    …                            (odd = structural hop, even = context pass)
 *
 *  Both adjacencies are undirected, so the structural skeleton grows
 *  symmetrically (ancestors AND descendants) and context chains expand from the
 *  whole visible set. The structural frontier advances ONLY on structural
 *  edges — context-added nodes (e.g. decisions) don't pull their own lineage
 *  back into the skeleton, so "structural ring 2" stays literally two structural
 *  hops from the selection.
 *
 *  Edge weighting (returned, applied downstream): only STRUCTURAL edges within
 *  the revealed set are "direct" (bold). Context links stay ambient — a direct
 *  context link spawns a label portal + an infinite dash animation, and lighting
 *  many at once crashes the browser render process. Keeping context ambient is
 *  what makes the trace safe on a high-degree node. */
export function computeTraceFilter(
  selectedNodeId: string,
  allEdges: Edge[],
  depth: number,
): FilterResult {
  // Separate undirected adjacencies so structural and context reveal on
  // different phases.
  const structuralAdj = new Map<string, string[]>()
  const contextAdj = new Map<string, string[]>()
  for (const edge of allEdges) {
    const adj = edge.type === 'statusEdge' ? structuralAdj
      : edge.type === 'contextEdge' ? contextAdj
      : null
    if (!adj) continue
    if (!adj.has(edge.source)) adj.set(edge.source, [])
    if (!adj.has(edge.target)) adj.set(edge.target, [])
    adj.get(edge.source)!.push(edge.target)
    adj.get(edge.target)!.push(edge.source)
  }

  const visited = new Set<string>([selectedNodeId])
  // The structural BFS frontier — advanced only by structural rings, so the
  // skeleton stays pure (context-added nodes are leaves, not new roots).
  let structuralFrontier = [selectedNodeId]

  for (let step = 1; step <= depth; step++) {
    if (step % 2 === 1) {
      // Structural ring: one more hop along the lineage skeleton.
      const next: string[] = []
      for (const id of structuralFrontier) {
        const neighbors = structuralAdj.get(id)
        if (!neighbors) continue
        for (const neighbor of neighbors) {
          if (!visited.has(neighbor)) {
            visited.add(neighbor)
            next.push(neighbor)
          }
        }
      }
      structuralFrontier = next
    } else {
      // Context pass: pull in context links off everything visible so far
      // (including nodes added by earlier context passes, so context chains
      // unfold progressively).
      const snapshot = [...visited]
      for (const id of snapshot) {
        const neighbors = contextAdj.get(id)
        if (!neighbors) continue
        for (const neighbor of neighbors) {
          if (!visited.has(neighbor)) visited.add(neighbor)
        }
      }
    }
  }

  // All edges within the revealed set drive dimming (cluster); only the
  // structural ones are "direct" (bold). Context links stay ambient.
  const edgeIds = new Set<string>()
  const directEdgeIds = new Set<string>()
  for (const edge of allEdges) {
    if (visited.has(edge.source) && visited.has(edge.target)) {
      edgeIds.add(edge.id)
      if (edge.type === 'statusEdge') directEdgeIds.add(edge.id)
    }
  }

  return { nodeIds: visited, edgeIds, directEdgeIds, matchCount: visited.size }
}
