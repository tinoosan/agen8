import { describe, expect, it } from 'vitest'
import { filterRemovedMemberEvents, removedMemberRefs } from './removedMemberLogs'
import type { AgentEvent, SpaceMember } from './types'

function member(overrides: Partial<SpaceMember>): SpaceMember {
  return {
    id: 'member-active',
    spaceId: 'space-1',
    channelId: 'channel:space-1:member:member-active',
    displayName: 'Active',
    memberType: 'worker',
    lifecycleState: 'active',
    harnessKind: 'codex',
    model: 'gpt-5',
    effort: 'low',
    ...overrides,
  }
}

function event(id: string, data: Record<string, string>): AgentEvent {
  return {
    id,
    kind: 'agent.tool.call',
    title: 'Tool call',
    status: 'ok',
    startedAt: '2026-05-26T10:00:00Z',
    data,
  }
}

describe('removed member log filtering', () => {
  it('indexes removed members by durable message and runtime identifiers', () => {
    const refs = removedMemberRefs([
      member({
        id: 'member-removed',
        channelId: 'channel:space-1:member:member-removed',
        currentRunId: 'run-removed',
        displayName: 'Sarah',
        lifecycleState: 'removed',
      }),
    ])

    expect(refs.has('member-removed')).toBe(true)
    expect(refs.has('channel:space-1:member:member-removed')).toBe(true)
    expect(refs.has('run-removed')).toBe(true)
    expect(refs.has('sarah')).toBe(false)
  })

  it('hides removed member events unless explicitly included', () => {
    const members = [
      member({ id: 'member-active', lifecycleState: 'active' }),
      member({
        id: 'member-removed',
        channelId: 'channel:space-1:member:member-removed',
        currentRunId: 'run-removed',
        displayName: 'Sarah',
        lifecycleState: 'removed',
      }),
    ]
    const active = event('event-active', { memberId: 'member-active' })
    const removedByMember = event('event-removed-member', { memberId: 'member-removed' })
    const removedByRun = event('event-removed-run', { runId: 'run-removed' })
    const removedByChannel = event('event-removed-channel', { channelId: 'channel:space-1:member:member-removed' })

    expect(filterRemovedMemberEvents([active, removedByMember, removedByRun, removedByChannel], members, false))
      .toEqual([active])
    expect(filterRemovedMemberEvents([active, removedByMember], members, true))
      .toEqual([active, removedByMember])
  })

  it('does not hide active member message sends because an old removed member reused the same display name', () => {
    const members = [
      member({
        id: 'member-active-sarah',
        channelId: 'channel:space-1:member:member-active-sarah',
        displayName: 'Sarah',
        lifecycleState: 'active',
      }),
      member({
        id: 'member-removed-sarah',
        channelId: 'channel:space-1:member:member-removed-sarah',
        displayName: 'Sarah',
        lifecycleState: 'removed',
      }),
    ]
    const outboundToActiveSarah = event('event-send-active-sarah', {
      channelId: 'channel:space-1:member:member-fred',
      destinationMemberId: 'member-active-sarah',
      destinationMemberLabel: 'Sarah',
    })

    expect(filterRemovedMemberEvents([outboundToActiveSarah], members, false))
      .toEqual([outboundToActiveSarah])
  })
})
