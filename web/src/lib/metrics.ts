import type { Task, ProjectMember } from './types'
import { pickupLatencyMs, inProgressDurationMs } from './taskTiming'

/**
 * The metrics page is a CLIENT-SIDE PROJECTION, exactly like the activity feed.
 *
 * There is no metrics table, RPC, or migration: every figure here is derived
 * from data already fetched for other surfaces — the task list (timestamps +
 * status) and the project roster (model / harness per member). The existing SSE
 * invalidation refetches those queries, so recomputing this projection keeps the
 * page live for free.
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
 * as "—" rather than a fabricated 0. Leaderboards only count tasks we can
 * actually attribute to a model / harness via the member who claimed them;
 * unattributable tasks simply don't contribute to a breakdown (they're still
 * counted in throughput totals).
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

export interface LeaderboardEntry {
  /** The model name or harness kind this row aggregates. */
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
  modelLeaderboard: LeaderboardEntry[]
  harnessLeaderboard: LeaderboardEntry[]
}

export interface ComputeMetricsInput {
  tasks?: Task[]
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

/**
 * Project the current task + member snapshots into the metrics shown on the page.
 * Pure and deterministic given its inputs.
 */
export function computeMetrics({
  tasks,
  members,
  now = Date.now(),
}: ComputeMetricsInput): ProjectMetrics {
  const taskList = tasks ?? []
  const memberById = new Map<string, ProjectMember>()
  for (const m of members ?? []) memberById.set(m.id, m)

  let backlog = 0
  let inProgress = 0
  let completed = 0
  const pickupLatencies: number[] = []
  const workTimes: number[] = []

  const modelBuckets = new Map<string, Bucket>()
  const harnessBuckets = new Map<string, Bucket>()

  const bump = (buckets: Map<string, Bucket>, key: string, succeeded: boolean, workMs: number | null) => {
    let b = buckets.get(key)
    if (!b) {
      b = { key, done: 0, failed: 0, workTimes: [] }
      buckets.set(key, b)
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

    // Leaderboards: only succeeded/failed outcomes attributed to a real member.
    if (!isDone && !isFailed) continue
    const member = task.claimedByMemberId ? memberById.get(task.claimedByMemberId) : undefined
    if (!member) continue
    const model = member.model?.trim()
    if (model) bump(modelBuckets, model, isDone, workMs)
    const harness = member.harnessKind?.trim()
    if (harness) bump(harnessBuckets, harness, isDone, workMs)
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
    modelLeaderboard: finalizeBuckets(modelBuckets),
    harnessLeaderboard: finalizeBuckets(harnessBuckets),
  }
}

/** Format a success rate (0..1) as a whole-percent string, or "—" when null. */
export function formatSuccessRate(rate: number | null): string {
  if (rate === null) return '—'
  return `${Math.round(rate * 100)}%`
}
