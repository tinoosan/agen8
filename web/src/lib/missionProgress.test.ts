import { describe, it, expect } from 'vitest'
import { keyResultProgressSummary, missionProgressColor } from './missionProgress'
import type { KeyResultView } from './types'

function makeKR(overrides: Partial<KeyResultView>): KeyResultView {
  return {
    id: 'kr-1',
    missionId: 'mission-A',
    title: 'A key result',
    measurementType: 'percentage',
    direction: 'increase',
    targetValue: 100,
    currentValue: 0,
    progressPercent: 0,
    lastMilestoneNotified: 0,
    status: 'on_track',
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
    ...overrides,
  }
}

describe('keyResultProgressSummary', () => {
  it('returns null for no KRs', () => {
    expect(keyResultProgressSummary(undefined)).toBeNull()
    expect(keyResultProgressSummary([])).toBeNull()
  })

  it('returns null when every KR is dropped', () => {
    const krs = [makeKR({ status: 'dropped', progressPercent: 80 })]
    expect(keyResultProgressSummary(krs)).toBeNull()
  })

  it('averages live KR progress and rounds', () => {
    const krs = [
      makeKR({ id: 'a', progressPercent: 100, status: 'completed' }),
      makeKR({ id: 'b', progressPercent: 0 }),
      makeKR({ id: 'c', progressPercent: 50 }),
    ]
    const s = keyResultProgressSummary(krs)
    expect(s).toEqual({ pct: 50, completed: 1, total: 3 })
  })

  it('excludes dropped KRs from the average and totals', () => {
    const krs = [
      makeKR({ id: 'a', progressPercent: 100, status: 'completed' }),
      makeKR({ id: 'b', progressPercent: 0, status: 'dropped' }),
    ]
    // only the completed KR counts: 100% over 1 live KR
    expect(keyResultProgressSummary(krs)).toEqual({ pct: 100, completed: 1, total: 1 })
  })

  it('clamps out-of-range progress into 0..100', () => {
    const krs = [
      makeKR({ id: 'a', progressPercent: 150 }),
      makeKR({ id: 'b', progressPercent: -20 }),
    ]
    // clamped to 100 and 0 → average 50
    expect(keyResultProgressSummary(krs)?.pct).toBe(50)
  })

  it('treats non-finite progress as 0', () => {
    const krs = [makeKR({ id: 'a', progressPercent: Number.NaN })]
    expect(keyResultProgressSummary(krs)?.pct).toBe(0)
  })
})

describe('missionProgressColor', () => {
  it('maps each status to its accent', () => {
    expect(missionProgressColor('active')).toContain('--accent')
    expect(missionProgressColor('completed')).toContain('--green')
    expect(missionProgressColor('paused')).toContain('--amber')
    expect(missionProgressColor('draft')).toContain('--text-3')
    expect(missionProgressColor('archived')).toContain('--text-3')
  })
})
