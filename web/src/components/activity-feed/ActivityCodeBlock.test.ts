import { describe, expect, it } from 'vitest'
import { normalizeActivityCodeLanguage } from './activityCodeLanguage'

describe('ActivityCodeBlock', () => {
  it('maps common aliases to registered prism languages', () => {
    expect(normalizeActivityCodeLanguage('ts')).toBe('typescript')
    expect(normalizeActivityCodeLanguage('html')).toBe('markup')
    expect(normalizeActivityCodeLanguage('yml')).toBe('yaml')
    expect(normalizeActivityCodeLanguage('text')).toBe('markup')
  })

  it('falls back to markup for unsupported values', () => {
    expect(normalizeActivityCodeLanguage('unknown-lang')).toBe('markup')
  })
})
