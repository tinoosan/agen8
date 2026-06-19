import { describe, it, expect } from 'vitest'
import { computeDiff, diffIsEmpty } from './sinceYouWereAway'
import type { Task, DecisionView, MissionView, KeyResultView } from './types'

const BASE_TIME = '2026-06-19T10:00:00.000Z'
const BEFORE = '2026-06-19T09:00:00.000Z' // before BASE_TIME
const AFTER = '2026-06-19T11:00:00.000Z'  // after BASE_TIME

function makeTask(overrides: Partial<Task>): Task {
  return {
    id: 't-1',
    description: 'Test task',
    status: 'succeeded',
    completedAt: AFTER,
    ...overrides,
  }
}

function makeDecision(overrides: Partial<DecisionView>): DecisionView {
  return {
    id: 'd-1',
    projectId: 'proj-1',
    source: 'agent',
    title: 'A decision',
    rationale: 'Because',
    confidence: 0.8,
    createdAt: AFTER,
    ...overrides,
  }
}

function makeMission(overrides: Partial<MissionView>): MissionView {
  return {
    id: 'm-1',
    projectId: 'proj-1',
    title: 'A mission',
    status: 'active',
    createdAt: BEFORE,
    updatedAt: AFTER,
    ...overrides,
  }
}

function makeKR(overrides: Partial<KeyResultView>): KeyResultView {
  return {
    id: 'kr-1',
    missionId: 'm-1',
    title: 'A key result',
    measurementType: 'percentage',
    direction: 'increase',
    targetValue: 100,
    currentValue: 50,
    progressPercent: 50,
    lastMilestoneNotified: 0,
    status: 'on_track',
    createdAt: BEFORE,
    updatedAt: AFTER,
    ...overrides,
  }
}

describe('computeDiff', () => {
  it('returns empty diff when lastSeenAt is null', () => {
    const diff = computeDiff(null, [makeTask()], [makeDecision()], [makeMission()], [makeKR()])
    expect(diffIsEmpty(diff)).toBe(true)
  })

  it('returns empty diff when lastSeenAt is empty string', () => {
    const diff = computeDiff('', [makeTask()], [makeDecision()], [makeMission()], [makeKR()])
    expect(diffIsEmpty(diff)).toBe(true)
  })

  it('returns empty diff when lastSeenAt is invalid', () => {
    const diff = computeDiff('not-a-date', [makeTask()], [makeDecision()], [makeMission()], [makeKR()])
    expect(diffIsEmpty(diff)).toBe(true)
  })

  it('includes completed tasks after lastSeen', () => {
    const diff = computeDiff(BASE_TIME, [makeTask({ completedAt: AFTER })], [], [], [])
    expect(diff.completedTasks).toHaveLength(1)
  })

  it('excludes completed tasks at or before lastSeen', () => {
    const diff = computeDiff(BASE_TIME, [makeTask({ completedAt: BEFORE })], [], [], [])
    expect(diff.completedTasks).toHaveLength(0)
  })

  it('excludes tasks that are not succeeded', () => {
    const diff = computeDiff(
      BASE_TIME,
      [makeTask({ status: 'active', completedAt: AFTER })],
      [],
      [],
      [],
    )
    expect(diff.completedTasks).toHaveLength(0)
  })

  it('excludes tasks missing completedAt', () => {
    const diff = computeDiff(
      BASE_TIME,
      [makeTask({ status: 'completed', completedAt: undefined })],
      [],
      [],
      [],
    )
    expect(diff.completedTasks).toHaveLength(0)
  })

  it('includes new decisions after lastSeen', () => {
    const diff = computeDiff(BASE_TIME, [], [makeDecision({ createdAt: AFTER })], [], [])
    expect(diff.newDecisions).toHaveLength(1)
  })

  it('excludes decisions at or before lastSeen', () => {
    const diff = computeDiff(BASE_TIME, [], [makeDecision({ createdAt: BEFORE })], [], [])
    expect(diff.newDecisions).toHaveLength(0)
  })

  it('includes missions updated after lastSeen', () => {
    const diff = computeDiff(BASE_TIME, [], [], [makeMission({ updatedAt: AFTER })], [])
    expect(diff.changedMissions).toHaveLength(1)
  })

  it('excludes missions updated at or before lastSeen', () => {
    const diff = computeDiff(BASE_TIME, [], [], [makeMission({ updatedAt: BEFORE })], [])
    expect(diff.changedMissions).toHaveLength(0)
  })

  it('includes key results updated after lastSeen', () => {
    const diff = computeDiff(BASE_TIME, [], [], [], [makeKR({ updatedAt: AFTER })])
    expect(diff.changedKeyResults).toHaveLength(1)
  })

  it('excludes key results updated at or before lastSeen', () => {
    const diff = computeDiff(BASE_TIME, [], [], [], [makeKR({ updatedAt: BEFORE })])
    expect(diff.changedKeyResults).toHaveLength(0)
  })

  it('handles all categories simultaneously', () => {
    const diff = computeDiff(
      BASE_TIME,
      [makeTask({ completedAt: AFTER }), makeTask({ id: 't-2', completedAt: BEFORE })],
      [makeDecision({ createdAt: AFTER }), makeDecision({ id: 'd-2', createdAt: BEFORE })],
      [makeMission({ updatedAt: AFTER })],
      [makeKR({ updatedAt: AFTER }), makeKR({ id: 'kr-2', updatedAt: BEFORE })],
    )
    expect(diff.completedTasks).toHaveLength(1)
    expect(diff.newDecisions).toHaveLength(1)
    expect(diff.changedMissions).toHaveLength(1)
    expect(diff.changedKeyResults).toHaveLength(1)
  })
})

describe('diffIsEmpty', () => {
  it('returns true for an all-empty diff', () => {
    const diff = computeDiff(BASE_TIME, [], [], [], [])
    expect(diffIsEmpty(diff)).toBe(true)
  })

  it('returns false when any category has items', () => {
    const diff = computeDiff(BASE_TIME, [makeTask({ completedAt: AFTER })], [], [], [])
    expect(diffIsEmpty(diff)).toBe(false)
  })
})
