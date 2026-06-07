/**
 * Human-readable wording for a context-link edge, written from the perspective
 * of the node currently in focus.
 *
 * A context link is stored as `source --edgeType--> target` and read as
 * "source <edgeType> target". For example a `blocked_by` link from task A to
 * task B means "A is blocked by B". The label therefore has two forms:
 *
 *   - forward: read from the SOURCE's side (or when nothing is focused) — the
 *     stored relationship, e.g. "blocked by".
 *   - reverse: read from the TARGET's side — the inverse, e.g. "blocks".
 *
 * Before this existed, ContextEdge hardcoded "informs"/"informed by" for every
 * context link, so a `blocked_by` edge wrongly read "informed by". The label
 * must follow the real edge type.
 */
interface ContextEdgePhrases {
  /** Read from the source node's perspective: "source <forward> target". */
  forward: string
  /** Read from the target node's perspective: "target <reverse> source". */
  reverse: string
}

/**
 * Phrasing for each context edge type agen8's graph defines. Kept in sync with
 * the graph edge vocabulary (blocked_by, resolved_by, completed_by, serves,
 * informed_by, produced, made_during, spawned, child_of, relates_to).
 */
const CONTEXT_EDGE_PHRASES: Record<string, ContextEdgePhrases> = {
  informed_by: { forward: 'informed by', reverse: 'informs' },
  blocked_by: { forward: 'blocked by', reverse: 'blocks' },
  resolved_by: { forward: 'resolved by', reverse: 'resolves' },
  completed_by: { forward: 'completed by', reverse: 'completes' },
  made_during: { forward: 'made during', reverse: 'context for' },
  produced: { forward: 'produced', reverse: 'produced by' },
  spawned: { forward: 'spawned', reverse: 'spawned by' },
  serves: { forward: 'serves', reverse: 'served by' },
  child_of: { forward: 'child of', reverse: 'parent of' },
  relates_to: { forward: 'relates to', reverse: 'relates to' },
}

/** Turn a raw edge type like "blocked_by" into "blocked by". */
export function humanizeEdgeType(edgeType: string): string {
  return edgeType.replace(/_/g, ' ').trim()
}

/**
 * The verb phrase for a context edge, given its type and whether the focused
 * node is the edge's target. Pure: it returns only the phrase, with no arrow
 * and no React — the caller composes the directional arrow.
 *
 * An unmapped edge type falls back to its humanized name (e.g. "escalated by")
 * in both directions. That is truthful — it shows the real relationship name —
 * rather than a hardcoded wrong label. There is no silent substitution.
 */
export function contextEdgeLabel(edgeType: string, focusedAtTarget: boolean): string {
  const phrases = CONTEXT_EDGE_PHRASES[edgeType]
  if (!phrases) {
    return humanizeEdgeType(edgeType)
  }
  return focusedAtTarget ? phrases.reverse : phrases.forward
}
