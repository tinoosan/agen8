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
    return firstNonEmpty(
      typeof data.spaceName === 'string' ? data.spaceName : '',
      node.id,
    )
  }

  if (node.type === 'keyResult') {
    const kr = (data.kr ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof kr.spaceName === 'string' ? kr.spaceName : '',
      typeof kr.spaceId === 'string' ? kr.spaceId : '',
      typeof data.spaceName === 'string' ? data.spaceName : '',
    )
  }

  if (node.type === 'task') {
    const task = (data.task ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof task.spaceName === 'string' ? task.spaceName : '',
      typeof task.spaceId === 'string' ? task.spaceId : '',
      typeof task.sourceSpaceId === 'string' ? task.sourceSpaceId : '',
      typeof task.destinationSpaceId === 'string' ? task.destinationSpaceId : '',
      typeof task.assignedRole === 'string' ? task.assignedRole : '',
    )
  }

  if (node.type === 'decision') {
    const decision = (data.decision ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof decision.spaceName === 'string' ? decision.spaceName : '',
      typeof decision.spaceId === 'string' ? decision.spaceId : '',
      typeof decision.sourceIdentity === 'string' ? decision.sourceIdentity : '',
      typeof decision.sourceMemberLabel === 'string' ? decision.sourceMemberLabel : '',
    )
  }

  if (node.type === 'operatorAction') {
    const oa = (data.oa ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof oa.spaceName === 'string' ? oa.spaceName : '',
      typeof oa.spaceId === 'string' ? oa.spaceId : '',
      typeof oa.sourceMemberLabel === 'string' ? oa.sourceMemberLabel : '',
      typeof oa.source === 'string' ? oa.source : '',
    )
  }

  if (node.type === 'escalation') {
    const escalation = (data.escalation ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof escalation.spaceName === 'string' ? escalation.spaceName : '',
      typeof escalation.spaceId === 'string' ? escalation.spaceId : '',
      typeof escalation.sourceMemberLabel === 'string' ? escalation.sourceMemberLabel : '',
      typeof escalation.source === 'string' ? escalation.source : '',
    )
  }

  if (node.type === 'plan') {
    const plan = (data.plan ?? {}) as Record<string, unknown>
    return firstNonEmpty(
      typeof plan.spaceName === 'string' ? plan.spaceName : '',
      typeof plan.spaceId === 'string' ? plan.spaceId : '',
      typeof plan.createdBy === 'string' ? plan.createdBy : '',
      node.id,
    )
  }

  return firstNonEmpty(node.id)
}
