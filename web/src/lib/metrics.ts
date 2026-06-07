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

export interface ProjectMetrics {
  throughput: ThroughputMetrics
}

export interface ComputeMetricsInput {
  tasks?: Task[]
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
  now = Date.now(),
}: ComputeMetricsInput): ProjectMetrics {
  const taskList = tasks ?? []

  let backlog = 0
  let inProgress = 0
  let completed = 0
  const pickupLatencies: number[] = []
  const workTimes: number[] = []

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
    if (task.status === DONE_STATUS) {
      const workMs = inProgressDurationMs(task, now)
      if (workMs !== null) workTimes.push(workMs)
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
