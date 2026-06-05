import { describe, expect, it } from 'vitest'
import { memberDisplayName } from './memberDisplay'

describe('memberDisplayName', () => {
  it('prefers stamped labels', () => {
    expect(memberDisplayName('Codex backend engineer', 'member-95fed2e1ebce6ce6')).toBe('Codex backend engineer')
  })

  it('does not present raw member ids as names', () => {
    expect(memberDisplayName(undefined, 'member-95fed2e1ebce6ce6')).toBeUndefined()
    expect(memberDisplayName('member-95fed2e1ebce6ce6', undefined)).toBeUndefined()
  })

  it('keeps readable non-id identities', () => {
    expect(memberDisplayName(undefined, 'backend_engineer')).toBe('backend engineer')
  })
})
