import type { Task, ProjectMember } from './types'
import { pickupLatencyMs, inProgressDurationMs } from './taskTiming'

/**
 * The metrics page is a CLIENT-SIDE PROJECTION, exactly like the activity feed.
 *
 * There is no metrics table, RPC, or migration: every figure here is derived
 * from data already fetched for other surfaces — the task list (timestamps +
 * status) and the project roster. The existing SSE invalidation refetches those
 * queries, so recomputing this projection keeps the page live for free.
 *
 * Two timing measures come straight from taskTiming.ts and are kept distinct on
 * purpose (the span the user cares about most is pickup/queue latency, not raw
 * execution time):
 *
 *   - Pickup latency:   createdAt (queued) → startedAt (first claimed).
 *   - Work time:        startedAt → completedAt (in-progress duration, final).
 *
 * Honest-MVP rule: we never invent data. A figure is null when there is nothing
 * real to average (e.g. no task has been picked up yet), and the UI renders that
 * as "—" rather than a fabricated 0.
 */

/* ── Status vocabulary (mirrors statusLabels.ts) ──────────
 * pending → Queued (the backlog), active → Working, succeeded → Done,
 * failed / canceled → terminal. We treat `pending` as the backlog because it is
 * the queued-awaiting-pickup state. */
const BACKLOG_STATUS = 'pending'
const IN_PROGRESS_STATUS = 'active'
const DONE_STATUS = 'succeeded'
const FAILED_STATUS = 'failed'

export interface ThroughputMetrics {
  /** Tasks queued and awaiting pickup (status `pending`). */
  backlog: number
  /** Tasks currently being worked (status `active`). */
  inProgress: number
  /** Tasks that finished successfully (status `succeeded`). */
  completed: number
  /** Every task in scope, regardless of status. */
  total: number
  /** Mean pickup latency over tasks that have been claimed; null when none have. */
  avgPickupLatencyMs: number | null
  /** Mean work time over completed tasks; null when none have completed. */
  avgWorkTimeMs: number | null
}

/* A leaderboard row aggregates completed/failed work for one harness. Only the
 * harness leaderboard exists today: harness is auto-determined per member
 * (see the Go daemon's HarnessFromJSONRPCBody), whereas model is not, so a model
 * leaderboard would be fabricated. Add it back here only once model is real. */
export interface LeaderboardEntry {
  /** The harness kind this row aggregates (e.g. "claude-code", "codex"). */
  key: string
  /** Successfully completed tasks attributed to this key. */
  done: number
  /** Failed tasks attributed to this key. */
  failed: number
  /** done / (done + failed); null when there are no terminal outcomes yet. */
  successRate: number | null
  /** Mean work time over this key's completed tasks; null when none completed. */
  avgWorkTimeMs: number | null
}

export interface ProjectMetrics {
  throughput: ThroughputMetrics
  /** Per-harness performance, most-productive first. Only counts tasks we can
   *  attribute to a real harness via the member who claimed them. */
  harnessLeaderboard: LeaderboardEntry[]
}

export interface ComputeMetricsInput {
  tasks?: Task[]
  /** Project roster — joins each claimed task to its claimant's harness. */
  members?: ProjectMember[]
  /** Injected for deterministic tests; only affects in-flight durations. */
  now?: number
}

/** Mean of a numeric list, or null when empty. */
function mean(values: number[]): number | null {
  if (values.length === 0) return null
  let sum = 0
  for (const v of values) sum += v
  return sum / values.length
}

/* Mutable accumulator while folding tasks into a leaderboard bucket. */
interface Bucket {
  key: string
  done: number
  failed: number
  workTimes: number[]
}

function finalizeBuckets(buckets: Map<string, Bucket>): LeaderboardEntry[] {
  const entries: LeaderboardEntry[] = []
  for (const b of buckets.values()) {
    const terminal = b.done + b.failed
    entries.push({
      key: b.key,
      done: b.done,
      failed: b.failed,
      successRate: terminal === 0 ? null : b.done / terminal,
      avgWorkTimeMs: mean(b.workTimes),
    })
  }
  // Most-productive first; tie-break on faster average, then key for stability.
  entries.sort(
    (a, b) =>
      b.done - a.done ||
      (a.avgWorkTimeMs ?? Infinity) - (b.avgWorkTimeMs ?? Infinity) ||
      (a.key < b.key ? -1 : a.key > b.key ? 1 : 0),
  )
  return entries
}

/** Local-time YYYY-MM-DD key for a date — the bucket unit for the daily series.
 * We bucket in LOCAL time (not UTC) so the sparkline lines up with the day the
 * viewer experienced, and an unparseable date yields a key no window contains. */
function localDateKey(d: Date): string {
  const y = d.getFullYear()
  if (Number.isNaN(y)) return 'invalid'
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${y}-${m}-${day}`
}

/**
 * Project the current task snapshot into the throughput metrics shown on the page.
 * Pure and deterministic given its inputs.
 */
export function computeMetrics({
  tasks,
  members,
  now = Date.now(),
}: ComputeMetricsInput): ProjectMetrics {
  const taskList = tasks ?? []

  // Roster lookup: joins each claimed task back to the harness of the member who
  // claimed it. A task we can't attribute to a real harness is simply skipped —
  // we never bucket it under a placeholder (honest-MVP rule).
  const harnessById = new Map<string, string>()
  for (const m of members ?? []) {
    const harness = m.harnessKind?.trim()
    if (harness) harnessById.set(m.id, harness)
  }

  let backlog = 0
  let inProgress = 0
  let completed = 0
  const pickupLatencies: number[] = []
  const workTimes: number[] = []
  const harnessBuckets = new Map<string, Bucket>()

  const bumpHarness = (key: string, succeeded: boolean, workMs: number | null) => {
    let b = harnessBuckets.get(key)
    if (!b) {
      b = { key, done: 0, failed: 0, workTimes: [] }
      harnessBuckets.set(key, b)
    }
    if (succeeded) {
      b.done += 1
      if (workMs !== null) b.workTimes.push(workMs)
    } else {
      b.failed += 1
    }
  }

  for (const task of taskList) {
    switch (task.status) {
      case BACKLOG_STATUS:
        backlog += 1
        break
      case IN_PROGRESS_STATUS:
        inProgress += 1
        break
      case DONE_STATUS:
        completed += 1
        break
    }

    // Pickup latency: any task that has been claimed contributes.
    const latency = pickupLatencyMs(task)
    if (latency !== null) pickupLatencies.push(latency)

    // Work time: only completed tasks have a final, meaningful duration.
    const isDone = task.status === DONE_STATUS
    const isFailed = task.status === FAILED_STATUS
    let workMs: number | null = null
    if (isDone) {
      workMs = inProgressDurationMs(task, now)
      if (workMs !== null) workTimes.push(workMs)
    }

    // Leaderboard: only terminal tasks we can attribute to a real harness count.
    if (isDone || isFailed) {
      const harness = task.claimedByMemberId
        ? harnessById.get(task.claimedByMemberId)
        : undefined
      if (harness) bumpHarness(harness, isDone, workMs)
    }
  }

  return {
    throughput: {
      backlog,
      inProgress,
      completed,
      total: taskList.length,
      avgPickupLatencyMs: mean(pickupLatencies),
      avgWorkTimeMs: mean(workTimes),
    },
    harnessLeaderboard: finalizeBuckets(harnessBuckets),
  }
}

/** Format a success rate (0..1) as a whole-percent string, or "—" when null. */
export function formatSuccessRate(rate: number | null): string {
  if (rate === null) return '—'
  return `${Math.round(rate * 100)}%`
}

/* ── Per-member performance (powers the Members roster) ───────────────────────
 *
 * Same honest-MVP projection as the throughput metrics, but bucketed by the
 * member who claimed each task. This lives next to the roster's existing
 * attributes so each agent's recent output reads inline. The `daily` series is a
 * trailing-window count of COMPLETED tasks per day — the input for the roster's
 * mini sparkline.
 */

export interface MemberDailyPoint {
  /** Local-time YYYY-MM-DD for this bucket. */
  date: string
  /** Tasks the member completed (succeeded) on this day. */
  done: number
}

export interface MemberPerformance {
  memberId: string
  /** Successfully completed tasks attributed to this member. */
  done: number
  /** Failed tasks attributed to this member. */
  failed: number
  /** Tasks the member is currently working (status `active`). */
  inProgress: number
  /** done / (done + failed); null when there are no terminal outcomes yet. */
  successRate: number | null
  /** Mean work time over this member's completed tasks; null when none. */
  avgWorkTimeMs: number | null
  /** Trailing-window completed-count series, oldest → newest, for the sparkline. */
  daily: MemberDailyPoint[]
}

export interface ComputeMemberPerformanceInput {
  tasks?: Task[]
  members?: ProjectMember[]
  /** Injected for deterministic tests; affects in-flight durations + the window. */
  now?: number
  /** Trailing window length (days) for the daily series. Defaults to 14. */
  windowDays?: number
}

/**
 * Project per-member performance keyed by member id. Pure and deterministic.
 * Every roster member gets an entry (zeroed when idle) so the UI can render a
 * consistent row; tasks claimed by an unknown member id also get one, which is
 * harmless because the roster only ever looks up ids it already knows.
 */
export function computeMemberPerformance({
  tasks,
  members,
  now = Date.now(),
  windowDays = 14,
}: ComputeMemberPerformanceInput): Map<string, MemberPerformance> {
  const taskList = tasks ?? []
  const memberList = members ?? []
  const days = Math.max(1, Math.floor(windowDays))

  // Trailing day window, oldest → newest. Stepped with setDate (not ms math) so
  // it stays correct across DST transitions.
  const start = new Date(now)
  start.setHours(0, 0, 0, 0)
  start.setDate(start.getDate() - (days - 1))
  const windowKeys: string[] = []
  for (let i = 0; i < days; i++) {
    const d = new Date(start)
    d.setDate(start.getDate() + i)
    windowKeys.push(localDateKey(d))
  }
  const windowSet = new Set(windowKeys)

  interface Acc {
    done: number
    failed: number
    inProgress: number
    workTimes: number[]
    byDay: Map<string, number>
  }
  const accById = new Map<string, Acc>()
  const ensure = (id: string): Acc => {
    let a = accById.get(id)
    if (!a) {
      a = { done: 0, failed: 0, inProgress: 0, workTimes: [], byDay: new Map() }
      accById.set(id, a)
    }
    return a
  }
  // Seed every roster member so idle agents still get a (zeroed) row.
  for (const m of memberList) ensure(m.id)

  for (const task of taskList) {
    const id = task.claimedByMemberId
    if (!id) continue
    const a = ensure(id)
    if (task.status === DONE_STATUS) {
      a.done += 1
      const workMs = inProgressDurationMs(task, now)
      if (workMs !== null) a.workTimes.push(workMs)
      if (task.completedAt) {
        const key = localDateKey(new Date(task.completedAt))
        if (windowSet.has(key)) a.byDay.set(key, (a.byDay.get(key) ?? 0) + 1)
      }
    } else if (task.status === FAILED_STATUS) {
      a.failed += 1
    } else if (task.status === IN_PROGRESS_STATUS) {
      a.inProgress += 1
    }
  }

  const result = new Map<string, MemberPerformance>()
  for (const [id, a] of accById) {
    const terminal = a.done + a.failed
    result.set(id, {
      memberId: id,
      done: a.done,
      failed: a.failed,
      inProgress: a.inProgress,
      successRate: terminal === 0 ? null : a.done / terminal,
      avgWorkTimeMs: mean(a.workTimes),
      daily: windowKeys.map((date) => ({ date, done: a.byDay.get(date) ?? 0 })),
    })
  }
  return result
}
