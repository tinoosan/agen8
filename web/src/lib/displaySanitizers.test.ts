import { describe, expect, it } from 'vitest'
import { looksLikeOpaqueId, safeReferenceLabel, sanitizeDecisionTitle, sanitizeDisplayTitle } from './displaySanitizers'

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
