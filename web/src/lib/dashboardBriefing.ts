/**
 * dashboardBriefing — pure compute for the hero "vitals" line.
 *
 * The dashboard opens with a one-line synthesis of the durable work state so
 * the user feels informed at a glance, before drilling into the detail cards
 * below. This module derives those numbers from data the dashboard already
 * loads (tasks, decisions, active missions) — it triggers no new fetches.
 *
 * Factored out of the component so it can be unit-tested without a React tree,
 * mirroring recentlyShipped.ts and sinceYouWereAway.ts.
 *
 * Vocabulary is deliberately domain-neutral: agen8 is a work-context layer for
 * any kind of work, not a coding tool, so we count "completed" and "in flight",
 * never "shipped" / "merged" / "deployed".
 */

import type { Task, DecisionView, MissionView } from './types'

export interface DashboardBriefing {
  /** Tasks the board is holding for a person: blocked + in_review. */
  needsYou: number
  /** Subset of needsYou awaiting review (in_review). Drives the chip's link target. */
  inReview: number
  /** Subset of needsYou blocked (blocked). Link fallback when nothing is in review. */
  blocked: number
  /** Tasks queued but not yet started: pending. */
  queued: number
  /** Tasks an agent is actively working right now: active. */
  inFlight: number
  /** Tasks that succeeded within the look-back window. */
  completed: number
  /** Decisions logged within the look-back window. */
  decisions: number
  /** Missions currently active. */
  activeMissions: number
}

/** Default look-back window for "recent" counts — matches Recently completed. */
export const BRIEFING_WINDOW_MS = 48 * 60 * 60 * 1000

/**
 * computeBriefing derives the hero vitals from live query data.
 *
 * @param tasks          Full task list from useProjectTasks.
 * @param decisions      Recent decisions from useRecentDecisions.
 * @param activeMissions Missions from useMissions(projectId, 'active'). Filtered
 *                       again here to status === 'active' so a looser query
 *                       (e.g. one that also returns paused) can't inflate it.
 * @param nowMs          Current time in ms (Date.now()). Injected for testability.
 * @param windowMs       Look-back window in ms. Defaults to 48 h.
 *
 * "needs you" and "in flight" are point-in-time board states (no window).
 * "completed" and "decisions" are deltas within the window. Status buckets are
 * mutually exclusive, so a task contributes to at most one of needsYou / inFlight.
 */
export function computeBriefing(
  tasks: Task[],
  decisions: DecisionView[],
  activeMissions: MissionView[],
  nowMs: number,
  windowMs: number = BRIEFING_WINDOW_MS,
): DashboardBriefing {
  const since = nowMs - windowMs

  let inReview = 0
  let blocked = 0
  let queued = 0
  let inFlight = 0
  let completed = 0

  for (const t of tasks) {
    const status = t.status ?? ''
    if (status === 'in_review') {
      inReview += 1
    } else if (status === 'blocked') {
      blocked += 1
    } else if (status === 'pending') {
      queued += 1
    } else if (status === 'active') {
      inFlight += 1
    }
    if (status === 'succeeded' && t.completedAt && new Date(t.completedAt).getTime() > since) {
      completed += 1
    }
  }

  const needsYou = inReview + blocked

  const decisionsInWindow = decisions.reduce(
    (n, d) => (new Date(d.createdAt).getTime() > since ? n + 1 : n),
    0,
  )

  const activeMissionCount = activeMissions.reduce(
    (n, m) => (m.status === 'active' ? n + 1 : n),
    0,
  )

  return {
    needsYou,
    inReview,
    blocked,
    queued,
    inFlight,
    completed,
    decisions: decisionsInWindow,
    activeMissions: activeMissionCount,
  }
}

/**
 * briefingIsEmpty — true when every vital is zero. Drives whether the line is
 * worth rendering: a project with tasks but a fully quiet board still shows the
 * line (the zeros are reassuring), so visibility is gated on having tasks at all
 * in the component, not on this. Exposed for tests and future callers.
 */
export function briefingIsEmpty(b: DashboardBriefing): boolean {
  return (
    b.needsYou === 0 &&
    b.queued === 0 &&
    b.inFlight === 0 &&
    b.completed === 0 &&
    b.decisions === 0 &&
    b.activeMissions === 0
  )
}
