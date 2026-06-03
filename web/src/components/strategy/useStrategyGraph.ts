import { useMemo } from 'react'
import { useMissionKRNodes } from './useMissionKRNodes'
import { useLeafNodes } from './useLeafNodes'
import { useContextLinkEdges } from './useContextLinkEdges'
import { assignClusterMeta, runForceLayout } from './strategyMapLayout'
import { deriveNodeClusterIdentity } from './clusterIdentity'
import { sourceToClusterColor } from '../../lib/clusterColors'

// ── Main hook ─────────────────────────────────────────────────────────────────
// The force-directed layout and the BFS cluster-meta walk run synchronously
// on the main thread inside a topology-keyed useMemo. Previous commits tried
// to move this work to a Web Worker, but that introduced a real bug where
// the layout sometimes returned empty positions and every node collapsed
// onto (0, 0). The freeze cost of sync computation (20-100ms on dense maps)
// is vastly preferable to a broken map, and the useMemo key keyed on sorted
// node/edge IDs means the simulation only re-runs on actual topology
// changes — data-only updates (status, progress) reuse the cached layout.

export function useStrategyGraph(projectId: string | null, projectRoot: string | null, options: { showArchived?: boolean } = {}) {
  // ── Data sources (add new sources here to extend the knowledge graph) ───
  const missionKR = useMissionKRNodes(projectId, projectRoot, { showArchived: options.showArchived })
  const leafNodes = useLeafNodes(projectId, projectRoot)

  const allNodes = useMemo(
    () => [...missionKR.nodes, ...leafNodes.nodes],
    [missionKR.nodes, leafNodes.nodes],
  )

  const structuralEdges = useMemo(
    () => [...missionKR.edges, ...leafNodes.edges],
    [missionKR.edges, leafNodes.edges],
  )

  // ── Layout stability ──────────────────────────────────────────────────────
  // Stable keys derived from sorted IDs — only change when topology changes.
  // Data-only updates (status, progress) do NOT retrigger the force simulation,
  // preventing the map from jumping on every 10-second poll cycle.
  const nodeSetKey = useMemo(
    () => allNodes.map(n => n.id).sort().join(','),
    [allNodes],
  )
  const edgeSetKey = useMemo(
    () => structuralEdges.map(e => `${e.source}>${e.target}`).sort().join('|'),
    [structuralEdges],
  )

  // ── Cluster meta mapping (color & depth tier) ─────────────────────────
  const clusterMeta = useMemo(
    () => assignClusterMeta(allNodes, structuralEdges),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [nodeSetKey, edgeSetKey],
  )

  const positionCache = useMemo((): Map<string, { x: number; y: number }> => {
    return runForceLayout(allNodes, structuralEdges, clusterMeta)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodeSetKey, edgeSetKey, clusterMeta])

  // Count decision edges targeting each KR so KR nodes can display the count.
  const decisionCountByKR = useMemo(() => {
    const counts = new Map<string, number>()
    for (const edge of structuralEdges) {
      // Decision edges target a KR node (source=krId, target=decision:xxx)
      // But actually edges are source=parent, target=child. Decisions point
      // FROM the KR. So the source is the KR id for decision edges.
      if (edge.target.startsWith('decision:')) {
        counts.set(edge.source, (counts.get(edge.source) ?? 0) + 1)
      }
    }
    return counts
  }, [structuralEdges])

  const layoutedNodes = useMemo(() => {
    return allNodes.map(node => {
      const pos = positionCache.get(node.id) ?? node.position
      const m = clusterMeta.get(node.id)
      const structuredClusterColor = m?.color ?? 'var(--border-strong)'
      const fallbackIdentity = deriveNodeClusterIdentity(node)
      const fallbackColor = fallbackIdentity
        ? sourceToClusterColor(fallbackIdentity)
        : 'var(--border-strong)'
      const clusterColor = structuredClusterColor === 'var(--border-strong)'
        ? fallbackColor
        : structuredClusterColor
      const extra: Record<string, unknown> = { clusterColor }
      if (node.type === 'keyResult') {
        extra.linkedDecisionCount = decisionCountByKR.get(node.id) ?? 0
      }
      return {
        ...node,
        position: pos,
        data: { ...node.data, ...extra },
      }
    })
  }, [allNodes, positionCache, clusterMeta, decisionCountByKR])

  // ── Context link edges (added post-layout) ───────────────────────────────
  const krIds = useMemo(
    () => missionKR.nodes.filter(n => n.type === 'keyResult').map(n => n.id),
    [missionKR.nodes],
  )
  const missionIds = useMemo(
    () => missionKR.nodes.filter(n => n.type === 'mission').map(n => n.id),
    [missionKR.nodes],
  )

  const graphNodeIds = useMemo(
    () => layoutedNodes.map(node => node.id),
    [layoutedNodes],
  )

  const contextLinks = useContextLinkEdges(krIds, missionIds, graphNodeIds)

  // Build a nodeId → clusterColor lookup from the laid-out nodes so we can
  // enrich every edge with its source/target cluster colours. This is what
  // lets edges render in their cluster's identity instead of a hardcoded
  // blue when they're the focused/direct edges.
  const allEdges = useMemo(() => {
    const clusterByNodeId = new Map<string, string>()
    for (const node of layoutedNodes) {
      const color = (node.data as { clusterColor?: string } | undefined)?.clusterColor
      if (color) clusterByNodeId.set(node.id, color)
    }

    const structuralPairs = new Set(
      structuralEdges.flatMap(e => [`${e.source}|${e.target}`, `${e.target}|${e.source}`]),
    )
    const dedupedContextLinks = contextLinks.edges.filter(
      e => !structuralPairs.has(`${e.source}|${e.target}`),
    )
    return [...structuralEdges, ...dedupedContextLinks].map(edge => ({
      ...edge,
      data: {
        ...edge.data,
        sourceColor: clusterByNodeId.get(edge.source),
        targetColor: clusterByNodeId.get(edge.target),
      },
    }))
  }, [layoutedNodes, structuralEdges, contextLinks.edges])

  const isLoading = missionKR.isLoading || leafNodes.isLoading

  return { nodes: layoutedNodes, edges: allEdges, isLoading }
}
