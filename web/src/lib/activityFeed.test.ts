import { describe, it, expect } from 'vitest'
import {
  buildActivityEvents,
  getActivityBucket,
  groupActivityByBucket,
  ACTIVITY_BUCKET_ORDER,
  type ActivityEvent,
} from './activityFeed'
import type { Task, DecisionView, MissionView } from './types'

/* Fixed anchor so relative-time math is deterministic across machines/TZs. */
const T0 = Date.parse('2026-05-10T12:00:00.000Z')
const iso = (offsetMs: number) => new Date(T0 + offsetMs).toISOString()

const MIN = 60_000
const HOUR = 3_600_000
const DAY = 86_400_000

function makeTask(over: Partial<Task> = {}): Task {
  return {
    id: 't1',
    description: 'A task',
    status: 'pending',
    ...over,
  }
}

function makeDecision(over: Partial<DecisionView> = {}): DecisionView {
  return {
    id: 'd1',
    projectId: 'p1',
    source: 'agent' as DecisionView['source'],
    title: 'A decision',
    rationale: 'because',
    confidence: 0.8,
    createdAt: iso(0),
    ...over,
  }
}

function makeMission(over: Partial<MissionView> = {}): MissionView {
  return {
    id: 'm1',
    projectId: 'p1',
    title: 'A mission',
    status: 'active' as MissionView['status'],
    createdAt: iso(0),
    updatedAt: iso(0),
    ...over,
  }
}

describe('buildActivityEvents — task milestones', () => {
  it('emits created when only createdAt is present', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      tasks: [makeTask({ id: 'ta', createdAt: iso(0) })],
    })
    expect(events.map(e => e.type)).toEqual(['task.created'])
    expect(events[0].id).toBe('task.created:ta')
  })

  it('emits created + started + completed for a finished task, each with a distinct id', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      tasks: [
        makeTask({
          id: 'tb',
          status: 'succeeded',
          createdAt: iso(0),
          startedAt: iso(5 * MIN),
          completedAt: iso(20 * MIN),
        }),
      ],
    })
    expect(events.map(e => e.type).sort()).toEqual(
      ['task.completed', 'task.created', 'task.started'].sort(),
    )
    // ids are unique even though they all share the same entity id
    expect(new Set(events.map(e => e.id)).size).toBe(3)
  })

  it('maps failed/canceled terminal statuses to their own types', () => {
    const failed = buildActivityEvents({
      projectId: 'p1',
      tasks: [makeTask({ id: 'tf', status: 'failed', completedAt: iso(0) })],
    })
    expect(failed.map(e => e.type)).toEqual(['task.failed'])

    const canceled = buildActivityEvents({
      projectId: 'p1',
      tasks: [makeTask({ id: 'tc', status: 'canceled', completedAt: iso(0) })],
    })
    expect(canceled.map(e => e.type)).toEqual(['task.canceled'])
  })

  it('omits the terminal event when status is terminal but completedAt is missing (no invented time)', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      tasks: [makeTask({ id: 'tn', status: 'succeeded', createdAt: iso(0) })],
    })
    expect(events.map(e => e.type)).toEqual(['task.created'])
  })

  it('does NOT emit a terminal event for a non-terminal status (e.g. in_review)', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      tasks: [
        makeTask({
          id: 'tr',
          status: 'in_review',
          createdAt: iso(0),
          startedAt: iso(MIN),
          completedAt: iso(2 * MIN), // present, but status is non-terminal
        }),
      ],
    })
    expect(events.map(e => e.type).sort()).toEqual(['task.created', 'task.started'])
  })

  it('falls back to description, then a literal, for the subject', () => {
    const titled = buildActivityEvents({
      projectId: 'p1',
      tasks: [makeTask({ id: 'tt', title: 'Ship it', createdAt: iso(0) })],
    })
    expect(titled[0].subject).toBe('Ship it')

    const described = buildActivityEvents({
      projectId: 'p1',
      tasks: [makeTask({ id: 'td', title: '   ', description: 'Just a desc', createdAt: iso(0) })],
    })
    expect(described[0].subject).toBe('Just a desc')
  })

  it('builds a task detail link for the project', () => {
    const events = buildActivityEvents({
      projectId: 'proj-x',
      tasks: [makeTask({ id: 'tl', createdAt: iso(0) })],
    })
    expect(events[0].link).toBe('/project/proj-x/tasks/tl')
  })
})

describe('buildActivityEvents — decisions & missions', () => {
  it('emits a single decision.logged event with an actor', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      decisions: [makeDecision({ id: 'dx', memberName: 'Nova', createdAt: iso(0) })],
    })
    expect(events).toHaveLength(1)
    expect(events[0].type).toBe('decision.logged')
    expect(events[0].id).toBe('decision.logged:dx')
    expect(events[0].actor).toBeTruthy()
  })

  it('emits a mission.created event with no actor (MissionView carries no creator)', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      missions: [makeMission({ id: 'mx', createdAt: iso(0) })],
    })
    expect(events).toHaveLength(1)
    expect(events[0].type).toBe('mission.created')
    expect(events[0].actor).toBeUndefined()
  })

  it('skips entities whose timestamp is missing or unparseable', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      decisions: [makeDecision({ id: 'bad', createdAt: 'not-a-date' })],
      missions: [makeMission({ id: 'bad2', createdAt: '' })],
    })
    expect(events).toHaveLength(0)
  })
})

describe('buildActivityEvents — ordering', () => {
  it('sorts newest-first across all kinds', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      tasks: [makeTask({ id: 'old', createdAt: iso(-2 * HOUR) })],
      decisions: [makeDecision({ id: 'mid', createdAt: iso(-1 * HOUR) })],
      missions: [makeMission({ id: 'new', createdAt: iso(0) })],
    })
    expect(events.map(e => e.kind)).toEqual(['mission', 'decision', 'task'])
  })

  it('breaks ties on id for a stable order at equal timestamps', () => {
    const events = buildActivityEvents({
      projectId: 'p1',
      decisions: [
        makeDecision({ id: 'bbb', createdAt: iso(0) }),
        makeDecision({ id: 'aaa', createdAt: iso(0) }),
      ],
    })
    expect(events.map(e => e.id)).toEqual(['decision.logged:aaa', 'decision.logged:bbb'])
  })
})

describe('getActivityBucket', () => {
  const now = T0
  it('classifies by recency', () => {
    expect(getActivityBucket(now - 2 * MIN, now)).toBe('Just now')
    expect(getActivityBucket(now - 30 * MIN, now)).toBe('Last hour')
    expect(getActivityBucket(now - 5 * HOUR, now)).toBe('Today')
    expect(getActivityBucket(now - (DAY + HOUR), now)).toBe('Yesterday')
    expect(getActivityBucket(now - 5 * DAY, now)).toBe('Older')
  })

  it('uses half-open boundaries (exactly 5m is no longer "Just now")', () => {
    expect(getActivityBucket(now - 5 * MIN, now)).toBe('Last hour')
    expect(getActivityBucket(now - HOUR, now)).toBe('Today')
    expect(getActivityBucket(now - DAY, now)).toBe('Yesterday')
    expect(getActivityBucket(now - 2 * DAY, now)).toBe('Older')
  })
})

describe('groupActivityByBucket', () => {
  it('returns only non-empty buckets, in canonical order', () => {
    const mk = (id: string, atMs: number): ActivityEvent => ({
      id,
      kind: 'task',
      type: 'task.created',
      at: new Date(atMs).toISOString(),
      atMs,
      subject: id,
      link: '#',
    })
    const now = T0
    const events = [
      mk('a', now - 1 * MIN), // Just now
      mk('b', now - 6 * HOUR), // Today
      mk('c', now - 7 * HOUR), // Today
    ]
    const groups = groupActivityByBucket(events, now)
    expect(groups.map(g => g.bucket)).toEqual(['Just now', 'Today'])
    expect(groups[1].items.map(i => i.id)).toEqual(['b', 'c'])
  })

  it('exposes the canonical bucket order constant', () => {
    expect(ACTIVITY_BUCKET_ORDER).toEqual(['Just now', 'Last hour', 'Today', 'Yesterday', 'Older'])
  })
})
