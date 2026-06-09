import { useMemo } from 'react'
import type { Node, Edge } from '@xyflow/react'
import {
  computeInMotionFilter,
  computeBlockedFilter,
  computeDecisionsFilter,
  computeDoneFilter,
  type FilterPreset,
} from './strategyMapFilters'

// Stable empty set so the "no direct edges" case keeps a constant reference
// across renders (avoids spurious re-renders of edge subscribers).
const NO_DIRECT_EDGES: ReadonlySet<string> = new Set()

/**
 * The strategy map's "lenses": the highlight filter results and the 1-hop
 * focus-neighborhood dimming sets. All pure derivations of the current graph +
 * filter + selection state. Every filter — including 'done' — uses the same
 * "show these, dim the rest" model: it returns a match set, and node/edge
 * components dim anything not in it.
 */
export function useStrategyMapLenses({
  activeFilter,
  displayNodes,
  displayEdges,
  effectiveFocusNodeId,
}: {
  activeFilter: FilterPreset | null
  displayNodes: Node[]
  displayEdges: Edge[]
  effectiveFocusNodeId: string | null
}) {
  // Highlight lenses — compute a match set that dims everything else.
  const filterResult = useMemo(() => {
    if (!activeFilter) return null
    if (activeFilter === 'in_motion') return computeInMotionFilter(displayNodes, displayEdges)
    if (activeFilter === 'blocked') return computeBlockedFilter(displayNodes, displayEdges)
    if (activeFilter === 'decisions') return computeDecisionsFilter(displayNodes, displayEdges)
    if (activeFilter === 'done') return computeDoneFilter(displayNodes, displayEdges)
    return null
  }, [activeFilter, displayNodes, displayEdges])

  // ── Highlighting logic (1-hop neighborhood focus) ─────────────────────
  // When a node is focused, we compute a visual hierarchy:
  //   - neighborhood nodes  = focused node + every 1-hop neighbor (both
  //                           structural and context-link edges)
  //   - direct edges        = edges touching the focused node
  //   - neighborhood edges  = edges where BOTH endpoints are in the
  //                           neighborhood (so parent↔child within the
  //                           focused subgraph stay visible)
  // Everything outside the neighborhood dims (non-neighborhood nodes fade,
  // non-neighborhood edges drop to 0.3 opacity) so the user gets an
  // uncluttered view of just the relationships of the node they clicked.
  //
  // Naming note: `clusterNodeIds` / `clusterEdgeIds` kept the old names for
  // backwards compatibility with leaf-node components, but the SEMANTICS
  // changed from "same cluster colour" to "focus neighborhood".
  const neighborhood = useMemo(() => {
    // Smart filter overrides focus-neighborhood dimming.
    if (filterResult) {
      // "Direct" edges get the heavy per-edge treatment: a label portal plus,
      // for context links, an infinite dash-flow animation. That's fine for a
      // node focus (a handful of edges). But a SET-highlight filter (in_motion /
      // blocked / done / decisions) can match the entire decision web at once —
      // flagging every matched edge as direct sprays hundreds of label portals
      // and infinite animations and crashes the browser's render process
      // (Safari shows "a problem repeatedly occurred"). So set filters highlight
      // via opacity contrast only: matched edges stay in clusterEdgeIds
      // (ambient), nothing is "direct".
      return {
        directEdgeIds: NO_DIRECT_EDGES,
        clusterNodeIds: filterResult.nodeIds,
        clusterEdgeIds: filterResult.edgeIds,
      }
    }

    if (!effectiveFocusNodeId) {
      return { directEdgeIds: null, clusterNodeIds: null, clusterEdgeIds: null }
    }

    const neighborIds = new Set<string>([effectiveFocusNodeId])
    const dEdges = new Set<string>()

    for (const edge of displayEdges) {
      const touchesFocus =
        edge.source === effectiveFocusNodeId || edge.target === effectiveFocusNodeId
      if (!touchesFocus) continue
      neighborIds.add(edge.source === effectiveFocusNodeId ? edge.target : edge.source)
      // Only STRUCTURAL edges become "direct" (bold + label + dash-flow). A
      // direct CONTEXT edge spawns a label portal AND an infinite
      // `context-dash-flow` animation (see ContextEdge.tsx). Focusing a node
      // with many context links would then mount dozens-to-hundreds of
      // perpetual animations + portals at once — which iOS WebKit's tight
      // render-memory budget can't survive, so the iPad crashes on select.
      // Context links touching the focus stay AMBIENT: they're still revealed
      // via clusterEdgeIds below (both endpoints land in the neighborhood),
      // just never animated.
      if (edge.type === 'statusEdge') dEdges.add(edge.id)
    }

    const cEdges = new Set<string>(dEdges)
    for (const edge of displayEdges) {
      if (neighborIds.has(edge.source) && neighborIds.has(edge.target)) {
        cEdges.add(edge.id)
      }
    }

    return { directEdgeIds: dEdges, clusterNodeIds: neighborIds, clusterEdgeIds: cEdges }
  }, [effectiveFocusNodeId, displayEdges, filterResult])

  return {
    filterResult,
    directEdgeIds: neighborhood.directEdgeIds,
    clusterNodeIds: neighborhood.clusterNodeIds,
    clusterEdgeIds: neighborhood.clusterEdgeIds,
  }
}
