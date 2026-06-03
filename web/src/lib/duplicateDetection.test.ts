import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { isDuplicateOA } from './duplicateDetection'

describe('isDuplicateOA', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-03-30T12:00:00.000Z'))
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns true for identical title+taskRef+category within 5-minute window', () => {
    const existing = [
      {
        title: 'Review budget',
        taskRef: 'task-123',
        category: 'financial' as const,
        createdAt: '2026-03-30T11:57:00.000Z', // 3 minutes ago
      },
    ]
    expect(isDuplicateOA('Review budget', 'task-123', 'financial', existing)).toBe(true)
  })

  it('returns false for same content outside 5-minute window', () => {
    const existing = [
      {
        title: 'Review budget',
        taskRef: 'task-123',
        category: 'financial' as const,
        createdAt: '2026-03-30T11:50:00.000Z', // 10 minutes ago
      },
    ]
    expect(isDuplicateOA('Review budget', 'task-123', 'financial', existing)).toBe(false)
  })

  it('returns false for different title within window', () => {
    const existing = [
      {
        title: 'Different title',
        taskRef: 'task-123',
        category: 'financial' as const,
        createdAt: '2026-03-30T11:58:00.000Z',
      },
    ]
    expect(isDuplicateOA('Review budget', 'task-123', 'financial', existing)).toBe(false)
  })

  it('returns false for different taskRef within window', () => {
    const existing = [
      {
        title: 'Review budget',
        taskRef: 'task-999',
        category: 'financial' as const,
        createdAt: '2026-03-30T11:58:00.000Z',
      },
    ]
    expect(isDuplicateOA('Review budget', 'task-123', 'financial', existing)).toBe(false)
  })

  it('returns false for different category within window', () => {
    const existing = [
      {
        title: 'Review budget',
        taskRef: 'task-123',
        category: 'legal' as const,
        createdAt: '2026-03-30T11:58:00.000Z',
      },
    ]
    expect(isDuplicateOA('Review budget', 'task-123', 'financial', existing)).toBe(false)
  })

  it('returns false for empty existing list', () => {
    expect(isDuplicateOA('Review budget', 'task-123', 'financial', [])).toBe(false)
  })

  it('handles exactly 5-minute boundary (exclusive)', () => {
    const existing = [
      {
        title: 'Review budget',
        taskRef: 'task-123',
        category: 'financial' as const,
        createdAt: '2026-03-30T11:55:00.000Z', // exactly 5 minutes ago
      },
    ]
    // At exactly 5 minutes, it should NOT be considered a duplicate (window is < 5 minutes)
    expect(isDuplicateOA('Review budget', 'task-123', 'financial', existing)).toBe(false)
  })

  it('handles undefined taskRef (both undefined = match)', () => {
    const existing = [
      {
        title: 'Review budget',
        taskRef: undefined,
        category: 'financial' as const,
        createdAt: '2026-03-30T11:58:00.000Z',
      },
    ]
    expect(isDuplicateOA('Review budget', undefined, 'financial', existing)).toBe(true)
  })
})
