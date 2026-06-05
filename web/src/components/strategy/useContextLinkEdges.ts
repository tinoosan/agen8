import { useMemo } from 'react'
import type { Edge } from '@xyflow/react'
import { useContextLinks, type ContextLinkEntity } from '../../hooks/useContextLinks'

/**
 * Maps a context link entity type + raw ID to the React Flow node ID used in
 * the strategy graph. Node IDs are prefixed for leaf types to avoid collisions
 * with KR/mission IDs (which use their raw IDs directly).
 */
function toNodeId(entityType: string, entityId: string): string {
  switch (entityType) {
    case 'mission':         return entityId
    case 'key_result':      return entityId
    case 'task':            return `task:${entityId}`
    case 'decision':        return `decision:${entityId}`
    default:                return entityId
  }
}

/**
 * Extracts the raw entity type + ID from a React Flow node so we can query
 * context links by source for leaf nodes.
 */
function graphNodeIdToEntity(nodeId: string): ContextLinkEntity | null {
  if (nodeId.startsWith('task:')) {
    return { entityType: 'task', entityId: nodeId.slice('task:'.length) }
  }
  if (nodeId.startsWith('decision:')) {
    return { entityType: 'decision', entityId: nodeId.slice('decision:'.length) }
  }
  switch (nodeId) {
    default:               return null
  }
}

/**
 * Builds context link edges for the strategy graph.
 *
 * Queries:
 *   - listByTarget for each KR/mission  → catches inbound links to the strategy backbone
 *   - listBySource for each leaf node   → catches outbound links (e.g. decision→task made_during)
 *
 * Only edges where BOTH source and target nodes are present in the graph are emitted.
 */
export function useContextLinkEdges(
  krIds: string[],
  missionIds: string[],
  graphNodeIds: string[],
): { edges: Edge[]; isLoading: boolean } {
  const leafSources = useMemo(() => {
    const result: ContextLinkEntity[] = []
    for (const nodeId of graphNodeIds) {
      const entity = graphNodeIdToEntity(nodeId)
      if (entity) result.push(entity)
    }
    return result
  }, [graphNodeIds])

  const { contextLinks, isLoading } = useContextLinks(krIds, missionIds, leafSources)

  const nodeIdSet = useMemo(() => {
    const s = new Set<string>()
    for (const nodeId of graphNodeIds) s.add(nodeId)
    return s
  }, [graphNodeIds])

  const edges = useMemo(() => {
    const result: Edge[] = []
    for (const link of contextLinks) {
      const sourceNodeId = toNodeId(link.sourceType, link.sourceId)
      const targetNodeId = toNodeId(link.targetType, link.targetId)
      // Only draw the edge if both endpoints are present in the graph
      if (!nodeIdSet.has(sourceNodeId) || !nodeIdSet.has(targetNodeId)) continue
      result.push({
        id: `cl:${link.id}`,
        source: sourceNodeId,
        target: targetNodeId,
        type: 'contextEdge',
        animated: false,
        data: { edgeType: link.edgeType, confidence: link.confidence },
      })
    }
    return result
  }, [contextLinks, nodeIdSet])

  return { edges, isLoading }
}
