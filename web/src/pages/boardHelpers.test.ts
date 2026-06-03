import { describe, it, expect } from 'vitest'
import type { ProjectSpaceSummary, Task } from '../lib/types'
import {
  isSystemTask,
  effectiveStatus,
  taskMatchesSpaceFilter,
  resolveBoardSpaceQueryParam,
  lookupSpaceForTask,
  getTaskBlockers,
  getOperatorActionBlocker,
  getLatestReview,
  getAcceptanceCriteria,
  taskDuration,
} from './boardHelpers'

function makeTask(overrides: Partial<Task> & { id: string }): Task {
  return { goal: '', status: 'pending', createdAt: new Date().toISOString(), ...overrides }
}

describe('isSystemTask', () => {
  it('hides spawn-worker tasks', () => {
    expect(isSystemTask(makeTask({ id: 'b-1', metadata: { source: 'spawn_worker' } }))).toBe(true)
  })

  it('shows regular delegated tasks with an assignedRole', () => {
    expect(isSystemTask(makeTask({ id: 't-1', assignedRole: 'researcher', metadata: { source: 'task_create' } }))).toBe(false)
  })

  it('shows regular tasks even before a member is assigned', () => {
    expect(isSystemTask(makeTask({ id: 't-2', metadata: { source: 'task_create' } }))).toBe(false)
  })
})

describe('effectiveStatus', () => {
  it('keeps original review_pending status authoritative', () => {
    const t = makeTask({ id: 'task-review', status: 'review_pending', assignedRole: 'reviewer' })
    expect(effectiveStatus(t)).toBe('review_pending')
  })

  it('returns original status when task is NOT in the review set', () => {
    const t = makeTask({ id: 'task-2', status: 'active' })
    expect(effectiveStatus(t)).toBe('active')
  })
})

const sampleSpaces: ProjectSpaceSummary[] = [
  { spaceId: 'space-a', spaceName: 'Alpha' },
  { spaceId: 'space-b', spaceName: 'Beta' },
]

describe('resolveBoardSpaceQueryParam', () => {
  it('accepts canonical space id', () => {
    expect(resolveBoardSpaceQueryParam('space-b', sampleSpaces)).toBe('space-b')
  })

  it('resolves space name case-insensitively', () => {
    expect(resolveBoardSpaceQueryParam('beta', sampleSpaces)).toBe('space-b')
  })

  it('returns null when unknown', () => {
    expect(resolveBoardSpaceQueryParam('nope', sampleSpaces)).toBeNull()
  })
})

describe('taskMatchesSpaceFilter', () => {
  it('matches direct space id', () => {
    const t = makeTask({ id: '1', spaceId: 'space-b', assignedRole: 'r' })
    expect(taskMatchesSpaceFilter(t, 'space-b')).toBe(true)
  })

  it('does not use legacy destination scope id for board filtering', () => {
    const t = makeTask({ id: '2', destinationSpaceId: 'space-b', assignedRole: 'r' })
    expect(taskMatchesSpaceFilter(t, 'space-b')).toBe(false)
  })

  it('matches stored runtime scope id when that is the only available key', () => {
    const t = makeTask({ id: '3', spaceId: 'space-b', assignedRole: 'r' })
    expect(taskMatchesSpaceFilter(t, 'space-b')).toBe(true)
  })
})

describe('lookupSpaceForTask', () => {
  const map = new Map(sampleSpaces.flatMap(row => [[row.spaceId, row], [row.spaceId ?? row.spaceId, row]]))

  it('ignores legacy destination scope id when looking up spaces', () => {
    const t = makeTask({ id: '1', destinationSpaceId: 'space-b', assignedRole: 'r' })
    expect(lookupSpaceForTask(t, map)).toBeNull()
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
          { kind: 'operator_action', id: 'oa-1', reason: 'Need contract review' },
          { kind: 'task', id: 'task-prereq' },
        ],
      },
    })

    expect(getTaskBlockers(t)).toEqual([
      { kind: 'operator_action', id: 'oa-1', reason: 'Need contract review', createdAt: undefined },
      { kind: 'task', id: 'task-prereq', reason: undefined, createdAt: undefined },
    ])
  })

  it('ignores malformed blocker rows', () => {
    const t = makeTask({
      id: 'bad-blockers',
      metadata: { blockedBy: [{ kind: 'operator_action' }, null, 'oa-1'] },
    })

    expect(getTaskBlockers(t)).toEqual([])
  })
})

describe('getOperatorActionBlocker', () => {
  it('returns the operator action blocker when present', () => {
    const t = makeTask({
      id: 'awaiting-oa',
      metadata: {
        blockedBy: [
          { kind: 'task', id: 'task-1' },
          { kind: 'operator_action', id: 'oa-77', reason: 'Waiting on operator' },
        ],
      },
    })

    expect(getOperatorActionBlocker(t)).toEqual({
      kind: 'operator_action',
      id: 'oa-77',
      reason: 'Waiting on operator',
      createdAt: undefined,
    })
  })

  it('returns null for dependency-only blockers', () => {
    const t = makeTask({
      id: 'dependency-only',
      metadata: { blockedBy: [{ kind: 'task', id: 'task-1' }] },
    })

    expect(getOperatorActionBlocker(t)).toBeNull()
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
