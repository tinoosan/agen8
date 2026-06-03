import type { AgentEvent, SpaceMember } from './types'

function normalized(value: unknown): string {
  return String(value ?? '').trim().toLowerCase()
}

export function isRemovedSpaceMember(member: SpaceMember): boolean {
  return normalized(member.lifecycleState) === 'removed'
}

export function removedMemberRefs(members: SpaceMember[] | null | undefined): Set<string> {
  const refs = new Set<string>()
  for (const member of members ?? []) {
    if (!isRemovedSpaceMember(member)) continue
    for (const value of [
      member.id,
      member.channelId,
      member.currentRunId,
    ]) {
      const ref = normalized(value)
      if (ref) refs.add(ref)
    }
  }
  return refs
}

function eventRefs(event: AgentEvent): string[] {
  const data = event.data ?? {}
  const refs = [
    data.memberId,
    data.memberID,
    data.member_id,
    data.member,
    data.memberLabel,
    data.destinationMemberId,
    data.destinationMemberID,
    data.role,
    data.runId,
    data.runID,
    data.sessionId,
    data.channelId,
    data.channelID,
    event.from,
    event.to,
  ]
  const id = normalized(event.id)
  const colonIdx = id.indexOf(':')
  if (colonIdx > 0) refs.push(id.slice(0, colonIdx))
  const pipeIdx = id.indexOf('|')
  if (pipeIdx > 0) refs.push(id.slice(0, pipeIdx))
  return refs.map(normalized).filter(Boolean)
}

export function eventBelongsToRemovedMember(event: AgentEvent, removedRefs: Set<string>): boolean {
  if (removedRefs.size === 0) return false
  return eventRefs(event).some(ref => removedRefs.has(ref))
}

export function filterRemovedMemberEvents(
  events: AgentEvent[],
  members: SpaceMember[] | null | undefined,
  includeRemoved: boolean,
): AgentEvent[] {
  if (includeRemoved) return events
  const refs = removedMemberRefs(members)
  if (refs.size === 0) return events
  return events.filter(event => !eventBelongsToRemovedMember(event, refs))
}
