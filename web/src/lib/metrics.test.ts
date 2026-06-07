import { describe, it, expect } from 'vitest'
import { computeMetrics, computeMemberPerformance, formatSuccessRate } from './metrics'
import type { Task, ProjectMember } from './types'

/* Fixed anchor so in-flight durations (which read `now`) stay deterministic. */
const T0 = Date.parse('2026-05-10T12:00:00.000Z')
const at = (offsetMs: number) => new Date(T0 + offsetMs).toISOString()
const MIN = 60_000

function makeTask(over: Partial<Task>): Task {
  return { id: 't', description: 'desc', status: 'pending', ...over }
}
function makeMember(over: Partial<ProjectMember>): ProjectMember {
  return {
    id: 'm',
    projectId: 'p',
    memberType: 'agent',
    lifecycleState: 'active',
    ...over,
  }
}

describe('computeMetrics — throughput', () => {
  it('counts backlog, in-progress, completed and total by status', () => {
    const { throughput } = computeMetrics({
      tasks: [
        makeTask({ id: 'a', status: 'pending' }),
        makeTask({ id: 'b', status: 'pending' }),
        makeTask({ id: 'c', status: 'active' }),
        makeTask({ id: 'd', status: 'succeeded' }),
        makeTask({ id: 'e', status: 'failed' }),
        makeTask({ id: 'f', status: 'canceled' }),
      ],
    })
    expect(throughput.backlog).toBe(2)
    expect(throughput.inProgress).toBe(1)
    expect(throughput.completed).toBe(1)
    expect(throughput.total).toBe(6)
  })

  it('averages pickup latency over every claimed task', () => {
    const { throughput } = computeMetrics({
      tasks: [
        // claimed 2m after create
        makeTask({ id: 'a', status: 'active', createdAt: at(0), startedAt: at(2 * MIN) }),
        // claimed 4m after create
        makeTask({ id: 'b', status: 'succeeded', createdAt: at(0), startedAt: at(4 * MIN), completedAt: at(10 * MIN) }),
        // never claimed → excluded
        makeTask({ id: 'c', status: 'pending', createdAt: at(0) }),
      ],
    })
    expect(throughput.avgPickupLatencyMs).toBe(3 * MIN)
  })

  it('averages work time over completed tasks only', () => {
    const { throughput } = computeMetrics({
      tasks: [
        // 6m of work, completed
        makeTask({ id: 'a', status: 'succeeded', startedAt: at(0), completedAt: at(6 * MIN) }),
        // 10m of work, completed
        makeTask({ id: 'b', status: 'succeeded', startedAt: at(0), completedAt: at(10 * MIN) }),
        // active (no completedAt) → not counted toward work-time average
        makeTask({ id: 'c', status: 'active', startedAt: at(0) }),
      ],
    })
    expect(throughput.avgWorkTimeMs).toBe(8 * MIN)
  })

  it('returns null averages when there is nothing real to average', () => {
    const { throughput } = computeMetrics({
      tasks: [makeTask({ id: 'a', status: 'pending', createdAt: at(0) })],
    })
    expect(throughput.avgPickupLatencyMs).toBeNull()
    expect(throughput.avgWorkTimeMs).toBeNull()
  })

  it('handles an empty input', () => {
    const { throughput } = computeMetrics({})
    expect(throughput.total).toBe(0)
    expect(throughput.backlog).toBe(0)
    expect(throughput.avgPickupLatencyMs).toBeNull()
  })
})

describe('computeMetrics — harness leaderboard', () => {
  const members = [
    makeMember({ id: 'm-cc', harnessKind: 'claude-code' }),
    makeMember({ id: 'm-cx', harnessKind: 'codex' }),
    makeMember({ id: 'm-none' }), // no harness signal → contributes nothing
  ]

  it('buckets terminal tasks by the claimant member harness', () => {
    const { harnessLeaderboard } = computeMetrics({
      members,
      now: T0,
      tasks: [
        // claude-code: 2 done (10m, 20m), 1 failed.
        makeTask({ id: 'a', status: 'succeeded', claimedByMemberId: 'm-cc', startedAt: at(-30 * MIN), completedAt: at(-20 * MIN) }),
        makeTask({ id: 'b', status: 'succeeded', claimedByMemberId: 'm-cc', startedAt: at(-40 * MIN), completedAt: at(-20 * MIN) }),
        makeTask({ id: 'c', status: 'failed', claimedByMemberId: 'm-cc' }),
        // codex: 1 done.
        makeTask({ id: 'd', status: 'succeeded', claimedByMemberId: 'm-cx', startedAt: at(-15 * MIN), completedAt: at(-5 * MIN) }),
        // active task does not count toward a terminal leaderboard.
        makeTask({ id: 'e', status: 'active', claimedByMemberId: 'm-cx' }),
      ],
    })
    expect(harnessLeaderboard.map((e) => e.key)).toEqual(['claude-code', 'codex'])
    const cc = harnessLeaderboard.find((e) => e.key === 'claude-code')!
    expect(cc.done).toBe(2)
    expect(cc.failed).toBe(1)
    expect(cc.successRate).toBeCloseTo(2 / 3, 5)
    expect(cc.avgWorkTimeMs).toBe(15 * MIN) // mean of 10m and 20m
    const cx = harnessLeaderboard.find((e) => e.key === 'codex')!
    expect(cx).toMatchObject({ done: 1, failed: 0, successRate: 1 })
  })

  it('skips tasks that cannot be attributed to a real harness', () => {
    const { harnessLeaderboard } = computeMetrics({
      members,
      now: T0,
      tasks: [
        // no claiming member.
        makeTask({ id: 'a', status: 'succeeded' }),
        // claimed by a member with no harness signal.
        makeTask({ id: 'b', status: 'succeeded', claimedByMemberId: 'm-none' }),
        // claimed by an unknown member id.
        makeTask({ id: 'c', status: 'failed', claimedByMemberId: 'ghost' }),
      ],
    })
    expect(harnessLeaderboard).toEqual([])
  })

  it('returns an empty leaderboard when no roster is supplied', () => {
    const { harnessLeaderboard } = computeMetrics({
      tasks: [makeTask({ id: 'a', status: 'succeeded', claimedByMemberId: 'm-cc' })],
    })
    expect(harnessLeaderboard).toEqual([])
  })

  it('orders by most done, then faster average, then key', () => {
    const { harnessLeaderboard } = computeMetrics({
      members: [
        makeMember({ id: 'm1', harnessKind: 'codex' }),
        makeMember({ id: 'm2', harnessKind: 'claude-code' }),
      ],
      now: T0,
      tasks: [
        // claude-code gets 2 done, codex gets 1 → claude-code ranks first.
        makeTask({ id: 'a', status: 'succeeded', claimedByMemberId: 'm2', startedAt: at(-20 * MIN), completedAt: at(-10 * MIN) }),
        makeTask({ id: 'b', status: 'succeeded', claimedByMemberId: 'm2', startedAt: at(-30 * MIN), completedAt: at(-20 * MIN) }),
        makeTask({ id: 'c', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(-15 * MIN), completedAt: at(-5 * MIN) }),
      ],
    })
    expect(harnessLeaderboard.map((e) => e.key)).toEqual(['claude-code', 'codex'])
  })
})

describe('computeMemberPerformance', () => {
  const members = [
    makeMember({ id: 'm1', displayName: 'Alpha' }),
    makeMember({ id: 'm2', displayName: 'Bravo' }),
  ]

  it('seeds a zeroed entry for every roster member, even idle ones', () => {
    const perf = computeMemberPerformance({ members, tasks: [], now: T0 })
    expect(perf.size).toBe(2)
    const m2 = perf.get('m2')!
    expect(m2).toMatchObject({ memberId: 'm2', done: 0, failed: 0, inProgress: 0, successRate: null, avgWorkTimeMs: null })
    // The daily series is the full window, all zeros.
    expect(m2.daily.length).toBe(14)
    expect(m2.daily.every((d) => d.done === 0)).toBe(true)
  })

  it('attributes done / failed / in-progress to the claiming member', () => {
    const perf = computeMemberPerformance({
      members,
      now: T0,
      tasks: [
        makeTask({ id: 'a', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(-30 * MIN), completedAt: at(-20 * MIN) }),
        makeTask({ id: 'b', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(-50 * MIN), completedAt: at(-40 * MIN) }),
        makeTask({ id: 'c', status: 'failed', claimedByMemberId: 'm1' }),
        makeTask({ id: 'd', status: 'active', claimedByMemberId: 'm1' }),
        makeTask({ id: 'e', status: 'succeeded', claimedByMemberId: 'm2', startedAt: at(-15 * MIN), completedAt: at(-5 * MIN) }),
      ],
    })
    const m1 = perf.get('m1')!
    expect(m1.done).toBe(2)
    expect(m1.failed).toBe(1)
    expect(m1.inProgress).toBe(1)
    // 2 done of 3 terminal = 2/3.
    expect(m1.successRate).toBeCloseTo(2 / 3, 5)
    // Each completed task took 10m of work → 10m average.
    expect(m1.avgWorkTimeMs).toBe(10 * MIN)
    expect(perf.get('m2')!.done).toBe(1)
  })

  it('ignores tasks with no claiming member (unattributable work)', () => {
    const perf = computeMemberPerformance({
      members,
      now: T0,
      tasks: [makeTask({ id: 'a', status: 'succeeded' })],
    })
    expect(perf.get('m1')!.done).toBe(0)
    expect(perf.get('m2')!.done).toBe(0)
  })

  it('buckets completed tasks into the daily window by completion day', () => {
    const DAY = 24 * 60 * MIN
    const perf = computeMemberPerformance({
      members,
      now: T0,
      windowDays: 7,
      tasks: [
        // two completed "today", one "two days ago", one outside the window.
        makeTask({ id: 'a', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(-30 * MIN), completedAt: at(-10 * MIN) }),
        makeTask({ id: 'b', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(-40 * MIN), completedAt: at(-20 * MIN) }),
        makeTask({ id: 'c', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(-2 * DAY - 30 * MIN), completedAt: at(-2 * DAY) }),
        makeTask({ id: 'd', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(-30 * DAY), completedAt: at(-30 * DAY + MIN) }),
      ],
    })
    const m1 = perf.get('m1')!
    expect(m1.daily.length).toBe(7)
    // The window total counts only the 3 completions inside it (the 30-days-ago one is excluded).
    const windowTotal = m1.daily.reduce((s, d) => s + d.done, 0)
    expect(windowTotal).toBe(3)
    // Newest bucket (last) holds the two completed today.
    expect(m1.daily[m1.daily.length - 1].done).toBe(2)
  })
})

describe('formatSuccessRate', () => {
  it('renders whole percents and an em dash for null', () => {
    expect(formatSuccessRate(0.94)).toBe('94%')
    expect(formatSuccessRate(1)).toBe('100%')
    expect(formatSuccessRate(0)).toBe('0%')
    expect(formatSuccessRate(null)).toBe('—')
  })
})
