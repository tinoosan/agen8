import { describe, it, expect } from 'vitest'
import { computeMetrics, formatSuccessRate } from './metrics'
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
    const { throughput, modelLeaderboard, harnessLeaderboard } = computeMetrics({})
    expect(throughput.total).toBe(0)
    expect(throughput.backlog).toBe(0)
    expect(throughput.avgPickupLatencyMs).toBeNull()
    expect(modelLeaderboard).toEqual([])
    expect(harnessLeaderboard).toEqual([])
  })
})

describe('computeMetrics — leaderboards', () => {
  const members = [
    makeMember({ id: 'm1', model: 'claude-opus-4.7', harnessKind: 'claude-code' }),
    makeMember({ id: 'm2', model: 'gpt-5.1-codex', harnessKind: 'codex-cli' }),
  ]

  it('attributes completed/failed tasks to the claiming member’s model', () => {
    const { modelLeaderboard } = computeMetrics({
      members,
      tasks: [
        makeTask({ id: 'a', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(0), completedAt: at(4 * MIN) }),
        makeTask({ id: 'b', status: 'succeeded', claimedByMemberId: 'm1', startedAt: at(0), completedAt: at(6 * MIN) }),
        makeTask({ id: 'c', status: 'failed', claimedByMemberId: 'm1' }),
        makeTask({ id: 'd', status: 'succeeded', claimedByMemberId: 'm2', startedAt: at(0), completedAt: at(20 * MIN) }),
      ],
    })
    // Opus first (2 done > 1 done).
    expect(modelLeaderboard.map((e) => e.key)).toEqual(['claude-opus-4.7', 'gpt-5.1-codex'])

    const opus = modelLeaderboard[0]
    expect(opus.done).toBe(2)
    expect(opus.failed).toBe(1)
    expect(opus.successRate).toBeCloseTo(2 / 3)
    expect(opus.avgWorkTimeMs).toBe(5 * MIN) // (4 + 6) / 2
  })

  it('builds a separate harness leaderboard from the same tasks', () => {
    const { harnessLeaderboard } = computeMetrics({
      members,
      tasks: [
        makeTask({ id: 'a', status: 'succeeded', claimedByMemberId: 'm1' }),
        makeTask({ id: 'b', status: 'succeeded', claimedByMemberId: 'm2' }),
        makeTask({ id: 'c', status: 'failed', claimedByMemberId: 'm2' }),
      ],
    })
    const byKey = Object.fromEntries(harnessLeaderboard.map((e) => [e.key, e]))
    expect(byKey['claude-code'].done).toBe(1)
    expect(byKey['claude-code'].successRate).toBe(1)
    expect(byKey['codex-cli'].done).toBe(1)
    expect(byKey['codex-cli'].failed).toBe(1)
    expect(byKey['codex-cli'].successRate).toBeCloseTo(0.5)
  })

  it('skips tasks that cannot be attributed to a member or model', () => {
    const { modelLeaderboard } = computeMetrics({
      members: [makeMember({ id: 'm1', harnessKind: 'claude-code' /* no model */ })],
      tasks: [
        makeTask({ id: 'a', status: 'succeeded' /* no claimedByMemberId */ }),
        makeTask({ id: 'b', status: 'succeeded', claimedByMemberId: 'missing' }),
        makeTask({ id: 'c', status: 'succeeded', claimedByMemberId: 'm1' /* member has no model */ }),
      ],
    })
    expect(modelLeaderboard).toEqual([])
  })

  it('only counts terminal outcomes — pending/active tasks never enter a board', () => {
    const { modelLeaderboard } = computeMetrics({
      members,
      tasks: [
        makeTask({ id: 'a', status: 'pending', claimedByMemberId: 'm1' }),
        makeTask({ id: 'b', status: 'active', claimedByMemberId: 'm1' }),
      ],
    })
    expect(modelLeaderboard).toEqual([])
  })

  it('reports a null success rate when a key has no terminal outcomes', () => {
    // A model with only canceled work: canceled is neither success nor failure,
    // so it should not appear at all (no done, no failed).
    const { modelLeaderboard } = computeMetrics({
      members,
      tasks: [makeTask({ id: 'a', status: 'canceled', claimedByMemberId: 'm1' })],
    })
    expect(modelLeaderboard).toEqual([])
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
