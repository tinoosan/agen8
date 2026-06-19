import { describe, it, expect } from 'vitest'
import { groupRecentlyShipped, MAX_MISSION_GROUPS, MAX_TASKS_PER_GROUP } from './recentlyShipped'
import type { Task, MissionView, KeyResultView } from './types'

const NOW = new Date('2026-06-19T12:00:00.000Z').getTime()
const WINDOW_48H = 48 * 60 * 60 * 1000

// Times relative to NOW
const WITHIN_WINDOW = new Date(NOW - 10 * 60 * 60 * 1000).toISOString()      // 10 h ago
const WITHIN_WINDOW_EARLIER = new Date(NOW - 24 * 60 * 60 * 1000).toISOString() // 24 h ago
const ON_BOUNDARY = new Date(NOW - WINDOW_48H).toISOString()                  // exactly 48 h ago (excluded)
const OUTSIDE_WINDOW = new Date(NOW - 49 * 60 * 60 * 1000).toISOString()     // 49 h ago

function makeTask(overrides: Partial<Task>): Task {
  return {
    id: 't-1',
    description: 'Test task',
    status: 'succeeded',
    completedAt: WITHIN_WINDOW,
    missionRef: 'mission-A',
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

function makeKR(overrides: Partial<KeyResultView>): KeyResultView {
  return {
    id: 'kr-1',
    missionId: 'mission-A',
    title: 'A key result',
    measurementType: 'percentage',
    direction: 'increase',
    targetValue: 100,
    currentValue: 50,
    progressPercent: 50,
    lastMilestoneNotified: 0,
    status: 'on_track',
    createdAt: '2026-01-01T00:00:00.000Z',
    updatedAt: '2026-06-01T00:00:00.000Z',
    ...overrides,
  }
}

function emptyKRMap(): Map<string, KeyResultView> {
  return new Map()
}

describe('groupRecentlyShipped — empty / null cases', () => {
  it('returns null when no tasks exist', () => {
    expect(groupRecentlyShipped([], [makeMission()], emptyKRMap(), NOW)).toBeNull()
  })

  it('returns null when all tasks are not succeeded', () => {
    const tasks = [
      makeTask({ status: 'active' }),
      makeTask({ id: 't-2', status: 'pending' }),
      makeTask({ id: 't-3', status: 'failed' }),
    ]
    expect(groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)).toBeNull()
  })

  it('returns null when all succeeded tasks are outside the window', () => {
    const tasks = [makeTask({ completedAt: OUTSIDE_WINDOW })]
    expect(groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)).toBeNull()
  })

  it('excludes tasks completed exactly on the window boundary (> not >=)', () => {
    const tasks = [makeTask({ completedAt: ON_BOUNDARY })]
    expect(groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)).toBeNull()
  })

  it('returns null when tasks are succeeded but have no completedAt', () => {
    const tasks = [makeTask({ completedAt: undefined })]
    expect(groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)).toBeNull()
  })
})

describe('groupRecentlyShipped — grouping by missionRef', () => {
  it('groups tasks by direct missionRef', () => {
    const tasks = [
      makeTask({ id: 't-1', missionRef: 'mission-A' }),
      makeTask({ id: 't-2', missionRef: 'mission-A' }),
    ]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result).not.toBeNull()
    expect(result!.groups).toHaveLength(1)
    expect(result!.groups[0].missionId).toBe('mission-A')
    expect(result!.groups[0].tasks).toHaveLength(2)
  })

  it('handles mission:<id> prefixed missionRef', () => {
    const tasks = [makeTask({ missionRef: 'mission:mission-A' })]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result).not.toBeNull()
    expect(result!.groups[0].missionId).toBe('mission-A')
  })

  it('puts tasks into separate groups for different missions', () => {
    const tasks = [
      makeTask({ id: 't-1', missionRef: 'mission-A' }),
      makeTask({ id: 't-2', missionRef: 'mission-B' }),
    ]
    const missions = [
      makeMission({ id: 'mission-A', title: 'Mission Alpha' }),
      makeMission({ id: 'mission-B', title: 'Mission Beta' }),
    ]
    const result = groupRecentlyShipped(tasks, missions, emptyKRMap(), NOW)
    expect(result).not.toBeNull()
    expect(result!.groups).toHaveLength(2)
  })
})

describe('groupRecentlyShipped — keyResultRef fallback', () => {
  it('resolves mission via keyResultRef when missionRef is absent', () => {
    const krMap = new Map<string, KeyResultView>()
    const kr = makeKR({ id: 'kr-1', missionId: 'mission-A' })
    krMap.set('kr-1', kr)

    const tasks = [makeTask({ missionRef: undefined, keyResultRef: 'kr-1' })]
    const result = groupRecentlyShipped(tasks, [makeMission()], krMap, NOW)
    expect(result).not.toBeNull()
    expect(result!.groups[0].missionId).toBe('mission-A')
  })

  it('resolves mission via keyResultRef with key_result: prefix in map', () => {
    const krMap = new Map<string, KeyResultView>()
    const kr = makeKR({ id: 'kr-1', missionId: 'mission-A' })
    krMap.set('key_result:kr-1', kr)

    const tasks = [makeTask({ missionRef: undefined, keyResultRef: 'key_result:kr-1' })]
    const result = groupRecentlyShipped(tasks, [makeMission()], krMap, NOW)
    expect(result).not.toBeNull()
    expect(result!.groups[0].missionId).toBe('mission-A')
  })

  it('prefers missionRef over keyResultRef when both are present', () => {
    const krMap = new Map<string, KeyResultView>()
    const kr = makeKR({ id: 'kr-1', missionId: 'mission-B' })
    krMap.set('kr-1', kr)

    const missions = [
      makeMission({ id: 'mission-A', title: 'Mission Alpha' }),
      makeMission({ id: 'mission-B', title: 'Mission Beta' }),
    ]
    const tasks = [makeTask({ missionRef: 'mission-A', keyResultRef: 'kr-1' })]
    const result = groupRecentlyShipped(tasks, missions, krMap, NOW)
    expect(result).not.toBeNull()
    // missionRef wins → mission-A
    expect(result!.groups[0].missionId).toBe('mission-A')
  })
})

describe('groupRecentlyShipped — "No mission" fallback group', () => {
  it('places tasks with unresolvable mission in a null group', () => {
    const tasks = [makeTask({ missionRef: 'mission-UNKNOWN', keyResultRef: undefined })]
    const result = groupRecentlyShipped(tasks, [], emptyKRMap(), NOW)
    expect(result).not.toBeNull()
    expect(result!.groups[0].missionId).toBeNull()
    expect(result!.groups[0].missionTitle).toBe('No mission')
  })

  it('omits "No mission" group when all tasks resolve to a mission', () => {
    const tasks = [makeTask({ missionRef: 'mission-A' })]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result!.groups.every((g) => g.missionId !== null)).toBe(true)
  })
})

describe('groupRecentlyShipped — sorting', () => {
  it('sorts groups by most-recent completedAt descending', () => {
    const tasks = [
      makeTask({ id: 't-1', missionRef: 'mission-A', completedAt: WITHIN_WINDOW_EARLIER }),
      makeTask({ id: 't-2', missionRef: 'mission-B', completedAt: WITHIN_WINDOW }),
    ]
    const missions = [
      makeMission({ id: 'mission-A', title: 'Mission Alpha' }),
      makeMission({ id: 'mission-B', title: 'Mission Beta' }),
    ]
    const result = groupRecentlyShipped(tasks, missions, emptyKRMap(), NOW)
    expect(result).not.toBeNull()
    // mission-B has more recent task → should be first
    expect(result!.groups[0].missionId).toBe('mission-B')
    expect(result!.groups[1].missionId).toBe('mission-A')
  })

  it('sorts tasks within a group by completedAt descending', () => {
    const tasks = [
      makeTask({ id: 't-1', missionRef: 'mission-A', completedAt: WITHIN_WINDOW_EARLIER }),
      makeTask({ id: 't-2', missionRef: 'mission-A', completedAt: WITHIN_WINDOW }),
    ]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result!.groups[0].tasks[0].task.id).toBe('t-2') // most recent first
    expect(result!.groups[0].tasks[1].task.id).toBe('t-1')
  })
})

describe('groupRecentlyShipped — caps', () => {
  it(`caps mission groups at ${MAX_MISSION_GROUPS} and reports moreGroupsCount`, () => {
    const missions: MissionView[] = []
    const tasks: Task[] = []
    for (let i = 0; i < MAX_MISSION_GROUPS + 3; i++) {
      missions.push(makeMission({ id: `mission-${i}`, title: `Mission ${i}` }))
      tasks.push(makeTask({ id: `t-${i}`, missionRef: `mission-${i}` }))
    }
    const result = groupRecentlyShipped(tasks, missions, emptyKRMap(), NOW)
    expect(result).not.toBeNull()
    expect(result!.groups).toHaveLength(MAX_MISSION_GROUPS)
    expect(result!.moreGroupsCount).toBe(3)
  })

  it(`caps tasks per group at ${MAX_TASKS_PER_GROUP}`, () => {
    const tasks: Task[] = []
    for (let i = 0; i < MAX_TASKS_PER_GROUP + 2; i++) {
      tasks.push(
        makeTask({
          id: `t-${i}`,
          missionRef: 'mission-A',
          completedAt: new Date(NOW - i * 1000).toISOString(),
        }),
      )
    }
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result!.groups[0].tasks).toHaveLength(MAX_TASKS_PER_GROUP)
    // Trimmed tasks must be reported, not silently dropped (no-silent-caps).
    expect(result!.groups[0].moreTasksCount).toBe(2)
    // The raw window total counts every succeeded task, before any cap.
    expect(result!.totalShipped).toBe(MAX_TASKS_PER_GROUP + 2)
  })

  it('reports moreGroupsCount of 0 when at or below the cap', () => {
    const tasks = [makeTask({ missionRef: 'mission-A' })]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result!.moreGroupsCount).toBe(0)
  })
})

describe('groupRecentlyShipped — agent label', () => {
  it('uses claimedByMemberLabel when available', () => {
    const tasks = [makeTask({ claimedByMemberLabel: 'Atlas (Backend)' })]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result!.groups[0].tasks[0].agentLabel).toBe('Atlas (Backend)')
  })

  it('falls back to claimedByMemberId when label is absent', () => {
    const tasks = [makeTask({ claimedByMemberLabel: undefined, claimedByMemberId: 'member-abc' })]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result!.groups[0].tasks[0].agentLabel).toBe('member-abc')
  })

  it('falls back to assignedToLabel then assignedTo', () => {
    const tasks = [
      makeTask({
        claimedByMemberLabel: undefined,
        claimedByMemberId: undefined,
        assignedToLabel: 'Wren (Frontend)',
      }),
    ]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result!.groups[0].tasks[0].agentLabel).toBe('Wren (Frontend)')
  })

  it('falls back to "Unknown" when all member fields are absent', () => {
    const tasks = [
      makeTask({
        claimedByMemberLabel: undefined,
        claimedByMemberId: undefined,
        assignedToLabel: undefined,
        assignedTo: undefined,
      }),
    ]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result!.groups[0].tasks[0].agentLabel).toBe('Unknown')
  })
})

describe('groupRecentlyShipped — window boundary', () => {
  it('includes tasks completed just inside the window', () => {
    const justInside = new Date(NOW - WINDOW_48H + 1000).toISOString()
    const tasks = [makeTask({ completedAt: justInside })]
    const result = groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW)
    expect(result).not.toBeNull()
  })

  it('respects a custom windowMs parameter', () => {
    const tasks = [makeTask({ completedAt: WITHIN_WINDOW })] // 10 h ago
    const twoHourWindowMs = 2 * 60 * 60 * 1000
    expect(groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW, twoHourWindowMs)).toBeNull()
    const twelveHourWindowMs = 12 * 60 * 60 * 1000
    expect(groupRecentlyShipped(tasks, [makeMission()], emptyKRMap(), NOW, twelveHourWindowMs)).not.toBeNull()
  })
})
