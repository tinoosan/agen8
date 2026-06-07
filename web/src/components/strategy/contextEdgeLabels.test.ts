import { describe, expect, it } from 'vitest'
import { contextEdgeLabel, humanizeEdgeType } from './contextEdgeLabels'

describe('contextEdgeLabel', () => {
  // forward = focused on source (or nothing focused); reverse = focused on target.
  const cases: Array<{ edgeType: string; forward: string; reverse: string }> = [
    { edgeType: 'informed_by', forward: 'informed by', reverse: 'informs' },
    { edgeType: 'blocked_by', forward: 'blocked by', reverse: 'blocks' },
    { edgeType: 'resolved_by', forward: 'resolved by', reverse: 'resolves' },
    { edgeType: 'completed_by', forward: 'completed by', reverse: 'completes' },
    { edgeType: 'made_during', forward: 'made during', reverse: 'context for' },
    { edgeType: 'produced', forward: 'produced', reverse: 'produced by' },
    { edgeType: 'spawned', forward: 'spawned', reverse: 'spawned by' },
    { edgeType: 'serves', forward: 'serves', reverse: 'served by' },
    { edgeType: 'child_of', forward: 'child of', reverse: 'parent of' },
    { edgeType: 'relates_to', forward: 'relates to', reverse: 'relates to' },
  ]

  for (const { edgeType, forward, reverse } of cases) {
    it(`${edgeType}: forward phrase from the source side`, () => {
      expect(contextEdgeLabel(edgeType, false)).toBe(forward)
    })
    it(`${edgeType}: reverse phrase from the target side`, () => {
      expect(contextEdgeLabel(edgeType, true)).toBe(reverse)
    })
  }

  it('a blocked_by edge never reads as "informed by" (the original bug)', () => {
    expect(contextEdgeLabel('blocked_by', false)).not.toContain('informed')
    expect(contextEdgeLabel('blocked_by', true)).not.toContain('informed')
  })

  it('falls back to the humanized edge type for an unmapped type, not a wrong label', () => {
    expect(contextEdgeLabel('escalated_by', false)).toBe('escalated by')
    expect(contextEdgeLabel('escalated_by', true)).toBe('escalated by')
  })
})

describe('humanizeEdgeType', () => {
  it('replaces underscores with spaces', () => {
    expect(humanizeEdgeType('blocked_by')).toBe('blocked by')
    expect(humanizeEdgeType('made_during')).toBe('made during')
  })

  it('leaves a single-word type unchanged', () => {
    expect(humanizeEdgeType('produced')).toBe('produced')
  })
})
