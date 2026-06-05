import type { Node } from '@xyflow/react'

function firstNonEmpty(...values: Array<string | null | undefined>): string {
  for (const value of values) {
    const trimmed = (value ?? '').trim()
    if (trimmed !== '') return trimmed
  }
  return ''
}

/**
 * Returns a stable identity string used to derive cluster colour for nodes
 * that are not reachable from a mission-rooted structural path.
 */
export function deriveNodeClusterIdentity(node: Node): string {
  const data = (node.data ?? {}) as Record<string, unknown>

  if (node.type === 'mission') {
    return firstNonEmpty(node.id)
  }

  if (node.type === 'keyResult') {
    const kr = (data.kr ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof kr.missionId === 'string' ? kr.missionId : '',
      typeof kr.id === 'string' ? kr.id : '',
      node.id,
    )
  }

  if (node.type === 'task') {
    const task = (data.task ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof task.assignedToLabel === 'string' ? task.assignedToLabel : '',
      typeof task.assignedTo === 'string' ? task.assignedTo : '',
      typeof task.id === 'string' ? task.id : '',
      node.id,
    )
  }

  if (node.type === 'decision') {
    const decision = (data.decision ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof decision.sourceMemberLabel === 'string' ? decision.sourceMemberLabel : '',
      typeof decision.sourceIdentity === 'string' ? decision.sourceIdentity : '',
      typeof decision.id === 'string' ? decision.id : '',
      node.id,
    )
  }

  if (node.type === 'plan') {
    const plan = (data.plan ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof plan.createdBy === 'string' ? plan.createdBy : '',
      node.id,
    )
  }

  return firstNonEmpty(node.id)
}
