import { describe, it, expect } from 'vitest'
import { computeBriefing, briefingIsEmpty, BRIEFING_WINDOW_MS } from './dashboardBriefing'
import type { Task, DecisionView, MissionView } from './types'

const NOW = new Date('2026-06-19T12:00:00.000Z').getTime()

// Times relative to NOW
const WITHIN_WINDOW = new Date(NOW - 10 * 60 * 60 * 1000).toISOString() // 10 h ago
const ON_BOUNDARY = new Date(NOW - BRIEFING_WINDOW_MS).toISOString() // exactly 48 h ago (excluded)
const OUTSIDE_WINDOW = new Date(NOW - 49 * 60 * 60 * 1000).toISOString() // 49 h ago

function makeTask(overrides: Partial<Task>): Task {
  return {
    id: 't-1',
    description: 'Test task',
    status: 'active',
    ...overrides,
  }
}

function makeDecision(overrides: Partial<DecisionView>): DecisionView {
  return {
    id: 'd-1',
    projectId: 'proj-1',
    source: 'agent',
    title: 'A decision',
    rationale: 'because',
    confidence: 0.8,
    createdAt: WITHIN_WINDOW,
    ...overrides,
  }
}

function makeMission(overrides: Partial<MissionView>): MissionView {
  return {
    id: 'mission-A',
    projectId: 'proj-1',
    title: 'Mission Alpha',
    status: 'active',
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-06-01T00:00:00.000Z',
    ...overrides,
  }
}

describe('computeBriefing — board states (point-in-time)', () => {
  it('counts blocked and in_review as "needs you"', () => {
    const tasks = [
      makeTask({ id: 't-1', status: 'blocked' }),
      makeTask({ id: 't-2', status: 'in_review' }),
      makeTask({ id: 't-3', status: 'in_review' }),
    ]
    const b = computeBriefing(tasks, [], [], NOW)
    expect(b.needsYou).toBe(3)
    expect(b.inFlight).toBe(0)
  })

  it('counts active as "in flight"', () => {
    const tasks = [
      makeTask({ id: 't-1', status: 'active' }),
      makeTask({ id: 't-2', status: 'active' }),
    ]
    const b = computeBriefing(tasks, [], [], NOW)
    expect(b.inFlight).toBe(2)
    expect(b.needsYou).toBe(0)
  })

  it('does not count pending / failed / canceled toward needs-you or in-flight', () => {
    const tasks = [
      makeTask({ id: 't-1', status: 'pending' }),
      makeTask({ id: 't-2', status: 'failed' }),
      makeTask({ id: 't-3', status: 'canceled' }),
    ]
    const b = computeBriefing(tasks, [], [], NOW)
    expect(b.needsYou).toBe(0)
    expect(b.inFlight).toBe(0)
    expect(b.completed).toBe(0)
  })

  it('a task contributes to at most one board bucket', () => {
    // status buckets are mutually exclusive — a single in_review task is needs-you, not in-flight
    const b = computeBriefing([makeTask({ status: 'in_review' })], [], [], NOW)
    expect(b.needsYou + b.inFlight).toBe(1)
  })
})

describe('computeBriefing — completed within window', () => {
  it('counts succeeded tasks completed inside the window', () => {
    const tasks = [
      makeTask({ id: 't-1', status: 'succeeded', completedAt: WITHIN_WINDOW }),
      makeTask({ id: 't-2', status: 'succeeded', completedAt: WITHIN_WINDOW }),
    ]
    expect(computeBriefing(tasks, [], [], NOW).completed).toBe(2)
  })

  it('excludes succeeded tasks completed outside the window', () => {
    const tasks = [makeTask({ status: 'succeeded', completedAt: OUTSIDE_WINDOW })]
    expect(computeBriefing(tasks, [], [], NOW).completed).toBe(0)
  })

  it('excludes a task completed exactly on the boundary (> not >=)', () => {
    const tasks = [makeTask({ status: 'succeeded', completedAt: ON_BOUNDARY })]
    expect(computeBriefing(tasks, [], [], NOW).completed).toBe(0)
  })

  it('ignores succeeded tasks with no completedAt', () => {
    const tasks = [makeTask({ status: 'succeeded', completedAt: undefined })]
    expect(computeBriefing(tasks, [], [], NOW).completed).toBe(0)
  })
})

describe('computeBriefing — decisions within window', () => {
  it('counts decisions created inside the window', () => {
    const decisions = [
      makeDecision({ id: 'd-1', createdAt: WITHIN_WINDOW }),
      makeDecision({ id: 'd-2', createdAt: WITHIN_WINDOW }),
    ]
    expect(computeBriefing([], decisions, [], NOW).decisions).toBe(2)
  })

  it('excludes decisions outside the window and on the boundary', () => {
    const decisions = [
      makeDecision({ id: 'd-1', createdAt: OUTSIDE_WINDOW }),
      makeDecision({ id: 'd-2', createdAt: ON_BOUNDARY }),
    ]
    expect(computeBriefing([], decisions, [], NOW).decisions).toBe(0)
  })
})

describe('computeBriefing — active missions', () => {
  it('counts only missions with status active', () => {
    const missions = [
      makeMission({ id: 'm-1', status: 'active' }),
      makeMission({ id: 'm-2', status: 'active' }),
      makeMission({ id: 'm-3', status: 'paused' }),
      makeMission({ id: 'm-4', status: 'completed' }),
    ]
    expect(computeBriefing([], [], missions, NOW).activeMissions).toBe(2)
  })
})

describe('computeBriefing — window override', () => {
  it('honours a custom window', () => {
    // 10 h ago is inside 48 h but outside a 1 h window
    const tasks = [makeTask({ status: 'succeeded', completedAt: WITHIN_WINDOW })]
    expect(computeBriefing(tasks, [], [], NOW, 60 * 60 * 1000).completed).toBe(0)
    expect(computeBriefing(tasks, [], [], NOW).completed).toBe(1)
  })
})

describe('briefingIsEmpty', () => {
  it('is true when every vital is zero', () => {
    expect(briefingIsEmpty(computeBriefing([], [], [], NOW))).toBe(true)
  })

  it('is false when any vital is non-zero', () => {
    const b = computeBriefing([makeTask({ status: 'active' })], [], [], NOW)
    expect(briefingIsEmpty(b)).toBe(false)
  })
})
