import type { Task } from './types'

/**
 * Task timing measures, mirrored from the Go domain helpers
 * (internal/services/task/domain/task_timing.go). The two measures are kept
 * deliberately DISTINCT — the user was explicit that the span that matters most
 * is pickup/queue latency ("how long until an agent is aware of a task"), NOT
 * execution time:
 *
 *   - Pickup latency:   createdAt (queued) → startedAt (first claimed).
 *   - In-progress time: startedAt → completedAt (or now, while in flight).
 *
 * Both derive from timestamps that already exist on the task, so no new field /
 * migration is required. In-progress duration is wall-clock and does not
 * subtract blocked/in_review intervals; an "active-only" measure would need a
 * status-transition history, intentionally deferred for the MVP.
 *
 * This is the data foundation the metrics page and over-threshold notifications
 * build on. Functions take an explicit `now` (defaulting to Date.now()) so
 * callers can render deterministically and tests stay stable.
 */

/** Parse an ISO timestamp to epoch ms, or null when absent/unparseable. */
function epochMs(iso?: string): number | null {
  if (!iso) return null
  const t = new Date(iso).getTime()
  return Number.isNaN(t) ? null : t
}

/** Terminal statuses — work is done, so they can never be "over threshold". */
const TERMINAL_STATUSES = new Set(['succeeded', 'failed', 'canceled'])

/**
 * Pickup latency in ms: startedAt - createdAt. null when either timestamp is
 * missing (never claimed, or unknown creation time). Clock skew is clamped to 0.
 */
export function pickupLatencyMs(task: Task): number | null {
  const created = epochMs(task.createdAt)
  const started = epochMs(task.startedAt)
  if (created === null || started === null) return null
  return Math.max(0, started - created)
}

/**
 * In-progress duration in ms: (completedAt ?? now) - startedAt. null when the
 * task was never started. Clock skew is clamped to 0.
 */
export function inProgressDurationMs(task: Task, now: number = Date.now()): number | null {
  const started = epochMs(task.startedAt)
  if (started === null) return null
  const completed = epochMs(task.completedAt)
  const end = completed ?? now
  return Math.max(0, end - started)
}

/**
 * Whether a still-in-flight task has been in progress past `thresholdMs`.
 * Terminal tasks never breach, an unstarted task never breaches, and a
 * non-positive threshold disables the check.
 */
export function isOverThreshold(
  task: Task,
  thresholdMs: number,
  now: number = Date.now(),
): boolean {
  if (thresholdMs <= 0 || TERMINAL_STATUSES.has(task.status)) return false
  const d = inProgressDurationMs(task, now)
  return d !== null && d > thresholdMs
}

/**
 * Coarse, human-readable duration for timing figures ("<1m", "5m", "2h 15m",
 * "3d 4h"). Distinct from format.ts `formatDuration`, which keeps sub-minute
 * precision for short execution spans — pickup latency and in-progress duration
 * read better at minute granularity. Returns null when given null.
 */
export function formatCoarseDuration(milliseconds: number | null): string | null {
  if (milliseconds === null) return null
  const mins = Math.floor(milliseconds / 60_000)
  if (mins < 1) return '<1m'
  if (mins < 60) return `${mins}m`
  const hrs = Math.floor(mins / 60)
  const remMins = mins % 60
  if (hrs < 24) return remMins > 0 ? `${hrs}h ${remMins}m` : `${hrs}h`
  const days = Math.floor(hrs / 24)
  const remHrs = hrs % 24
  return remHrs > 0 ? `${days}d ${remHrs}h` : `${days}d`
}
