import { useMemo } from 'react'
import type { Node, Edge } from '@xyflow/react'
import {
  computeInMotionFilter,
  computeBlockedFilter,
  computeDecisionsFilter,
  computeDoneFilter,
  computeTraceFilter,
  type FilterPreset,
} from './strategyMapFilters'

/**
 * The strategy map's "lenses": the highlight/declutter filter results, what
 * actually reaches the canvas after the declutter lens removes finished nodes,
 * and the 1-hop focus-neighborhood dimming sets. All pure derivations of the
 * current graph + filter + selection state.
 */
export function useStrategyMapLenses({
  activeFilter,
  displayNodes,
  displayEdges,
  selectedNodeId,
  contextDepth,
  effectiveFocusNodeId,
}: {
  activeFilter: FilterPreset | null
  displayNodes: Node[]
  displayEdges: Edge[]
  selectedNodeId: string | null
  contextDepth: number
  effectiveFocusNodeId: string | null
}) {
  // Highlight lenses — compute a match set that dims everything else.
  // 'done' is handled separately below because it hides rather than dims.
  const filterResult = useMemo(() => {
    if (!activeFilter) return null
    if (activeFilter === 'in_motion') return computeInMotionFilter(displayNodes, displayEdges)
    if (activeFilter === 'blocked') return computeBlockedFilter(displayNodes, displayEdges)
    if (activeFilter === 'decisions') return computeDecisionsFilter(displayNodes, displayEdges)
    if (activeFilter === 'trace' && selectedNodeId) {
      const structuralEdges = displayEdges.filter((e) => e.type === 'statusEdge')
      return computeTraceFilter(selectedNodeId, structuralEdges, displayEdges, contextDepth)
    }
    return null
  }, [activeFilter, displayNodes, displayEdges, selectedNodeId, contextDepth])

  // Declutter lens — the set of finished nodes to remove from the map.
  const doneHidden = useMemo(() => {
    if (activeFilter !== 'done') return null
    const structuralEdges = displayEdges.filter((e) => e.type === 'statusEdge')
    return computeDoneFilter(displayNodes, structuralEdges)
  }, [activeFilter, displayNodes, displayEdges])

  // What actually reaches the canvas: with 'done' active, drop hidden nodes
  // and any edge touching them; otherwise render the full graph.
  const renderNodes = useMemo(
    () => (doneHidden ? displayNodes.filter((n) => !doneHidden.nodeIds.has(n.id)) : displayNodes),
    [doneHidden, displayNodes],
  )
  const renderEdges = useMemo(
    () => (doneHidden
      ? displayEdges.filter((e) => !doneHidden.nodeIds.has(e.source) && !doneHidden.nodeIds.has(e.target))
      : displayEdges),
    [doneHidden, displayEdges],
  )

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
    // Smart filter overrides focus-neighborhood dimming
    if (filterResult) {
      return {
        directEdgeIds: filterResult.edgeIds,
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
      if (edge.source === effectiveFocusNodeId) {
        neighborIds.add(edge.target)
        dEdges.add(edge.id)
      } else if (edge.target === effectiveFocusNodeId) {
        neighborIds.add(edge.source)
        dEdges.add(edge.id)
      }
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
    doneHidden,
    renderNodes,
    renderEdges,
    directEdgeIds: neighborhood.directEdgeIds,
    clusterNodeIds: neighborhood.clusterNodeIds,
    clusterEdgeIds: neighborhood.clusterEdgeIds,
  }
}
