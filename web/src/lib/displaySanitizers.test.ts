import { describe, expect, it } from 'vitest'
import { isPrefixedId, isUuid, looksLikeOpaqueId, safeReferenceLabel, sanitizeDecisionTitle, sanitizeDisplayTitle } from './displaySanitizers'

describe('displaySanitizers', () => {
  it('drops opaque runtime identifiers from display titles', () => {
    expect(sanitizeDisplayTitle('space-07f5f6ee-06a7-4399-97b9-6ced1c165a78')).toBeNull()
  })

  it('keeps human-facing display titles, including slug titles', () => {
    expect(sanitizeDisplayTitle('research-tnxp')).toBe('research-tnxp')
    expect(sanitizeDisplayTitle('Market Research')).toBe('Market Research')
  })

  it('strips raw refs from decision titles', () => {
    expect(
      sanitizeDecisionTitle('Mission: mission-smoke KR: kr-smoke Task: task-smoke Usability smoke: decision tool'),
    ).toBe('Usability smoke: decision tool')
    expect(
      sanitizeDecisionTitle('Task: task-2026042 Coordinator tool usability friction findings'),
    ).toBe('Coordinator tool usability friction findings')
  })

  it('hides opaque references', () => {
    expect(looksLikeOpaqueId('task-123456')).toBe(true)
    expect(safeReferenceLabel('task-123456')).toBeNull()
    expect(safeReferenceLabel('Quarterly Planning')).toBe('Quarterly Planning')
  })
})

describe('isUuid', () => {
  it('matches a loose 8-4-4-4-12 hex uuid (case-insensitive)', () => {
    expect(isUuid('07f5f6ee-06a7-4399-97b9-6ced1c165a78')).toBe(true)
    expect(isUuid('ABCDEF01-1234-5678-9ABC-DEF012345678')).toBe(true)
    // Not RFC-strict: version nibble 0 + variant nibble 7 still match.
    expect(isUuid('07f5f6ee-06a7-0399-77b9-6ced1c165a78')).toBe(true)
  })

  it('rejects non-uuid strings', () => {
    expect(isUuid('member-9f3a')).toBe(false)
    expect(isUuid('not-a-uuid')).toBe(false)
    expect(isUuid('07f5f6ee06a7439997b96ced1c165a78')).toBe(false)
  })
})

describe('isPrefixedId', () => {
  it('matches raw entity identifiers across the broad prefix set', () => {
    expect(isPrefixedId('member-95fed2e1ebce6ce6')).toBe(true)
    expect(isPrefixedId('dec-38381e01')).toBe(true)
    expect(isPrefixedId('session-abcd')).toBe(true)
  })

  it('does not match bare uuids or human strings', () => {
    expect(isPrefixedId('07f5f6ee-06a7-4399-97b9-6ced1c165a78')).toBe(false)
    expect(isPrefixedId('Market Research')).toBe(false)
    expect(isPrefixedId('research-tnxp')).toBe(false)
  })
})

describe('looksLikeOpaqueId loose-uuid regression', () => {
  it('now catches a non-RFC-strict uuid that the old strict regex missed', () => {
    expect(looksLikeOpaqueId('07f5f6ee-06a7-0399-77b9-6ced1c165a78')).toBe(true)
  })
})
