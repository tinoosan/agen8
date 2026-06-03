import { useMemo } from 'react'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useAllEscalations } from '../../hooks/useEscalations'
import { useMissions, useProjectKRs } from '../../hooks/useMissions'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useProjectSpaces } from '../../hooks/useProjectSpaces'
import { useStrategyMapOpActions } from '../../hooks/useOpActions'

interface EntityResult {
  type: string
  data: unknown
  title: string
}

/**
 * Parses a nodeId into entity type and raw ID.
 *   "decision:abc123" → { type: 'decision', id: 'abc123' }
 *   "abc123"          → { type: 'mission', id: 'abc123' } (raw UUIDs = mission or KR)
 */
function parseNodeId(nodeId: string): { prefix: string; id: string } {
  const colonIdx = nodeId.indexOf(':')
  if (colonIdx > 0) {
    return { prefix: nodeId.slice(0, colonIdx), id: nodeId.slice(colonIdx + 1) }
  }
  return { prefix: '', id: nodeId }
}

/**
 * Looks up entity data for a given nodeId from cached query results.
 * Returns the entity type, data (in the shape the panel expects),
 * and a display title. Returns null if the entity isn't found yet
 * (queries still loading).
 */
export function useEntityLookup(
  nodeId: string | null,
  projectId: string | null,
  _projectRoot: string | null,
): EntityResult | null {
  const { prefix, id } = nodeId ? parseNodeId(nodeId) : { prefix: '', id: '' }

  const spacesQuery = useProjectSpaces(projectId, { refetchInterval: false })
  const spaces = spacesQuery.data ?? []
  const missionsQuery = useMissions(projectId)
  const krsQuery = useProjectKRs(projectId)
  const tasksQuery = useProjectTasks(spaces)
  const decisionsQuery = useRecentDecisions(projectId)
  const oasQuery = useStrategyMapOpActions(projectId)
  const escalationsQuery = useAllEscalations(projectId)

  return useMemo(() => {
    if (!nodeId || !id) return null

    if (prefix === 'task') {
      const task = (tasksQuery.data ?? []).find(t => t.id === id)
      if (!task) return null
      return { type: 'task', data: { task }, title: task.title ?? task.description ?? 'Task' }
    }

    if (prefix === 'decision') {
      const decision = (decisionsQuery.data ?? []).find(d => d.id === id)
      if (!decision) return null
      return { type: 'decision', data: { decision }, title: decision.title }
    }

    if (prefix === 'oa') {
      const oa = (oasQuery.data ?? []).find(o => o.id === id)
      if (!oa) return null
      return { type: 'operatorAction', data: { oa }, title: oa.title }
    }

    if (prefix === 'escalation') {
      const escalation = (escalationsQuery.data ?? []).find(e => e.id === id)
      if (!escalation) return null
      return { type: 'escalation', data: { escalation }, title: escalation.title }
    }

    // No prefix — could be a KR or mission (both use raw UUIDs)
    const kr = krsQuery.data?.get(id)
    if (kr) {
      return { type: 'keyResult', data: { kr }, title: kr.title }
    }

    const mission = (missionsQuery.data ?? []).find(m => m.id === id)
    if (mission) {
      return { type: 'mission', data: { mission, avgProgress: 0, krCount: 0 }, title: mission.title }
    }

    return null
  }, [nodeId, id, prefix, tasksQuery.data, decisionsQuery.data, oasQuery.data, escalationsQuery.data, krsQuery.data, missionsQuery.data])
}
