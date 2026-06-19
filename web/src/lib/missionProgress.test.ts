import { describe, it, expect } from 'vitest'
import {
  keyResultProgressSummary,
  missionProgressColor,
  groupKRsByMission,
  summarizeMissions,
} from './missionProgress'
import type { KeyResultView, MissionView } from './types'

function makeMission(overrides: Partial<MissionView>): MissionView {
  return {
    id: 'mission-A',
    projectId: 'proj-1',
    title: 'Mission',
    status: 'active',
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-01-01T00:00:00.000Z',
    ...overrides,
  }
}

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

describe('groupKRsByMission', () => {
  it('groups by missionId and dedupes by KR id (map has duplicate keys)', () => {
    const a = makeKR({ id: 'a', missionId: 'm1' })
    const b = makeKR({ id: 'b', missionId: 'm1' })
    const c = makeKR({ id: 'c', missionId: 'm2' })
    // simulate useProjectKRs: same KR present under multiple keys
    const grouped = groupKRsByMission([a, a, b, c, c])
    expect(grouped.get('m1')?.map((k) => k.id)).toEqual(['a', 'b'])
    expect(grouped.get('m2')?.map((k) => k.id)).toEqual(['c'])
  })
})

describe('summarizeMissions', () => {
  it('counts active/completed and averages only active missions with live KRs', () => {
    const missions = [
      makeMission({ id: 'm1', status: 'active' }),
      makeMission({ id: 'm2', status: 'active' }),
      makeMission({ id: 'm3', status: 'completed' }),
      makeMission({ id: 'm4', status: 'draft' }), // no KRs → ignored in avg
    ]
    const krByMission = new Map<string, KeyResultView[]>([
      ['m1', [makeKR({ id: 'a', missionId: 'm1', progressPercent: 80 })]],
      ['m2', [makeKR({ id: 'b', missionId: 'm2', progressPercent: 20 })]],
    ])
    const o = summarizeMissions(missions, krByMission)
    expect(o.total).toBe(4)
    expect(o.active).toBe(2)
    expect(o.completed).toBe(1)
    expect(o.avgActiveProgress).toBe(50) // (80 + 20) / 2
  })

  it('flags active missions with at-risk KRs as needing attention', () => {
    const missions = [
      makeMission({ id: 'm1', status: 'active' }),
      makeMission({ id: 'm2', status: 'active' }),
    ]
    const krByMission = new Map<string, KeyResultView[]>([
      ['m1', [makeKR({ id: 'a', missionId: 'm1', status: 'at_risk' }), makeKR({ id: 'b', missionId: 'm1', status: 'on_track' })]],
      ['m2', [makeKR({ id: 'c', missionId: 'm2', status: 'on_track' })]],
    ])
    const o = summarizeMissions(missions, krByMission)
    expect(o.atRiskKRs).toBe(1)
    expect(o.attentionCount).toBe(1)
  })

  it('does not count at-risk KRs on non-active missions', () => {
    const missions = [makeMission({ id: 'm1', status: 'paused' })]
    const krByMission = new Map<string, KeyResultView[]>([
      ['m1', [makeKR({ id: 'a', missionId: 'm1', status: 'at_risk' })]],
    ])
    const o = summarizeMissions(missions, krByMission)
    expect(o.attentionCount).toBe(0)
    expect(o.atRiskKRs).toBe(0)
    expect(o.avgActiveProgress).toBeNull()
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
