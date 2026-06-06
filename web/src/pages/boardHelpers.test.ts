import { describe, it, expect } from 'vitest'
import type { Task } from '../lib/types'
import {
  isSystemTask,
  effectiveStatus,
  getTaskBlockers,
  getLatestReview,
  getAcceptanceCriteria,
  taskDuration,
} from './boardHelpers'

function makeTask(overrides: Partial<Task> & { id: string }): Task {
  return { goal: '', status: 'pending', createdAt: new Date().toISOString(), ...overrides }
}

describe('isSystemTask', () => {
  it('hides internal maintenance tasks', () => {
    expect(isSystemTask(makeTask({ id: 'b-1', metadata: { source: 'system.maintenance' } }))).toBe(true)
  })

  it('shows regular assigned tasks', () => {
    expect(isSystemTask(makeTask({ id: 't-1', assignedToLabel: 'researcher', metadata: { source: 'task_create' } }))).toBe(false)
  })

  it('shows regular tasks even before a member is assigned', () => {
    expect(isSystemTask(makeTask({ id: 't-2', metadata: { source: 'task_create' } }))).toBe(false)
  })
})

describe('effectiveStatus', () => {
  it('keeps original review_pending status authoritative', () => {
    const t = makeTask({ id: 'task-review', status: 'review_pending', assignedToLabel: 'reviewer' })
    expect(effectiveStatus(t)).toBe('review_pending')
  })

  it('returns original status when task is NOT in the review set', () => {
    const t = makeTask({ id: 'task-2', status: 'active' })
    expect(effectiveStatus(t)).toBe('active')
  })
})

describe('getAcceptanceCriteria', () => {
  it('preserves structured satisfied state from task acceptance criteria', () => {
    const t = makeTask({
      id: 'task-with-criteria',
      acceptanceCriteria: [
        { id: 'criterion-1', text: 'mentions async delivery', satisfied: true },
        { id: 'criterion-2', text: 'mentions correlation IDs', satisfied: false },
      ],
    })

    expect(getAcceptanceCriteria(t)).toEqual([
      { id: 'criterion-1', text: 'mentions async delivery', satisfied: true },
      { id: 'criterion-2', text: 'mentions correlation IDs', satisfied: false },
    ])
  })

  it('parses legacy markdown checkbox criteria from metadata', () => {
    const t = makeTask({
      id: 'legacy-criteria',
      metadata: {
        acceptanceCriteria: [
          '- [x] mentions async delivery',
          '- [ ] mentions correlation IDs',
          'plain criterion',
        ],
      },
    })

    expect(getAcceptanceCriteria(t)).toEqual([
      { id: 'criterion-1', text: 'mentions async delivery', satisfied: true },
      { id: 'criterion-2', text: 'mentions correlation IDs', satisfied: false },
      { id: 'criterion-3', text: 'plain criterion', satisfied: false },
    ])
  })
})

describe('getTaskBlockers', () => {
  it('parses structured blocker metadata', () => {
    const t = makeTask({
      id: 'blocked-task',
      status: 'blocked',
      metadata: {
        blockedBy: [
          { kind: 'task', id: 'task-contract-review', reason: 'Need contract review' },
          { kind: 'task', id: 'task-prereq' },
        ],
      },
    })

    expect(getTaskBlockers(t)).toEqual([
      { kind: 'task', id: 'task-contract-review', reason: 'Need contract review', createdAt: undefined },
      { kind: 'task', id: 'task-prereq', reason: undefined, createdAt: undefined },
    ])
  })

  it('ignores malformed blocker rows', () => {
    const t = makeTask({
      id: 'bad-blockers',
      metadata: { blockedBy: [{ kind: 'task' }, null, 'task-prereq'] },
    })

    expect(getTaskBlockers(t)).toEqual([])
  })
})

describe('getLatestReview', () => {
  it('falls back to metadata review fields when attempts are absent', () => {
    const review = getLatestReview(makeTask({
      id: 'reviewed-task',
      metadata: {
        reviewDecision: 'approve',
        reviewFeedback: 'Looks good. Ship it.',
        reviewedBy: 'cto',
        reviewedAt: '2026-04-03T20:00:00Z',
        reviewerRole: 'cto',
      },
    }))

    expect(review).toEqual({
      decision: 'approved',
      feedback: 'Looks good. Ship it.',
      reviewedBy: 'cto',
      reviewedAt: '2026-04-03T20:00:00Z',
      reviewerRole: 'cto',
    })
  })
})

describe('taskDuration', () => {
  it('returns null for active tasks with no completedAt', () => {
    const t = makeTask({ id: 't-active', status: 'active', createdAt: '2026-04-05T10:00:00Z' })
    expect(taskDuration(t)).toBeNull()
  })

  it('returns minutes for short tasks', () => {
    const t = makeTask({
      id: 't-quick',
      status: 'succeeded',
      createdAt: '2026-04-05T10:00:00Z',
      completedAt: '2026-04-05T10:05:00Z',
    })
    expect(taskDuration(t)).toBe('5m')
  })

  it('returns hours and minutes for medium tasks', () => {
    const t = makeTask({
      id: 't-medium',
      status: 'succeeded',
      createdAt: '2026-04-05T10:00:00Z',
      completedAt: '2026-04-05T12:30:00Z',
    })
    expect(taskDuration(t)).toBe('2h 30m')
  })

  it('returns days for long tasks', () => {
    const t = makeTask({
      id: 't-long',
      status: 'succeeded',
      createdAt: '2026-04-03T10:00:00Z',
      completedAt: '2026-04-05T10:00:00Z',
    })
    expect(taskDuration(t)).toBe('2d')
  })

  it('returns null when completedAt is before createdAt (bad data)', () => {
    const t = makeTask({
      id: 't-bad',
      status: 'succeeded',
      createdAt: '2026-04-05T10:05:00Z',
      completedAt: '2026-04-05T10:00:00Z',
    })
    expect(taskDuration(t)).toBeNull()
  })
})
