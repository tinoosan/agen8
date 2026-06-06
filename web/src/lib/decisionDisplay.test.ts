import { describe, expect, it } from 'vitest'
import {
  confidenceBadgeClass,
  confidenceColor,
  confidenceTone,
  decisionActorDisplay,
} from './decisionDisplay'

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

describe('confidenceTone', () => {
  it('returns high at and above 0.8', () => {
    expect(confidenceTone(0.8)).toBe('high')
    expect(confidenceTone(1)).toBe('high')
  })

  it('returns medium from 0.6 up to (not including) 0.8', () => {
    expect(confidenceTone(0.6)).toBe('medium')
    expect(confidenceTone(0.79)).toBe('medium')
  })

  it('returns low below 0.6 — including the old 0.5 amber band', () => {
    expect(confidenceTone(0.59)).toBe('low')
    expect(confidenceTone(0.5)).toBe('low')
    expect(confidenceTone(0)).toBe('low')
  })
})

describe('confidenceColor', () => {
  it('maps tone to the matching CSS colour var', () => {
    expect(confidenceColor(0.9)).toBe('var(--green)')
    expect(confidenceColor(0.6)).toBe('var(--amber)')
    expect(confidenceColor(0.4)).toBe('var(--red)')
  })
})

describe('confidenceBadgeClass', () => {
  it('maps tone to the dim-background badge class pair', () => {
    expect(confidenceBadgeClass(0.9)).toBe('bg-[var(--green-dim)] text-[var(--green)]')
    expect(confidenceBadgeClass(0.6)).toBe('bg-[var(--amber-dim)] text-[var(--amber)]')
    expect(confidenceBadgeClass(0.4)).toBe('bg-[var(--red-dim)] text-[var(--red)]')
  })
})
