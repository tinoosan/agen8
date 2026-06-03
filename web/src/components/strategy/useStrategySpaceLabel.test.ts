import { describe, expect, it } from 'vitest'
import { resolveSpaceLabelValue } from './useStrategySpaceLabel'

describe('resolveSpaceLabelValue', () => {
  it('prefers explicit spaceLabel over map lookup', () => {
    const byID = new Map<string, string>([['space-1', 'ops']])
    expect(resolveSpaceLabelValue('research', 'space-1', byID)).toBe('research')
  })

  it('resolves spaceLabel from spaceId map when explicit ref is empty', () => {
    const byID = new Map<string, string>([['space-1', 'market-research']])
    expect(resolveSpaceLabelValue('', 'space-1', byID)).toBe('market-research')
  })

  it('returns empty string when neither explicit nor mapped ref exists', () => {
    const byID = new Map<string, string>()
    expect(resolveSpaceLabelValue('', 'space-missing', byID)).toBe('')
  })
})
