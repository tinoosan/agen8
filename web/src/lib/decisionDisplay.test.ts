import { describe, expect, it } from 'vitest'
import { decisionActorDisplay } from './decisionDisplay'

describe('decisionActorDisplay', () => {
  it('prefers the immutable stamped member name over raw ids', () => {
    expect(decisionActorDisplay({
      source: 'agent',
      memberId: 'member-95fed2e1ebce6ce6',
      memberName: 'Codex backend engineer',
      sourceIdentity: 'member-95fed2e1ebce6ce6',
    })).toEqual({
      label: 'Codex backend engineer',
      clusterKey: 'member-95fed2e1ebce6ce6',
    })
  })

  it('uses a readable source identity when no stamped member name exists', () => {
    expect(decisionActorDisplay({
      source: 'agent',
      memberId: 'member-abc123',
      sourceIdentity: 'backend engineer',
    }).label).toBe('backend engineer')
  })

  it('falls back to the raw member id only when no readable label exists', () => {
    expect(decisionActorDisplay({
      source: 'agent',
      memberId: 'member-abc123',
      sourceIdentity: 'session-abc123',
    }).label).toBe('member-abc123')
  })
})
